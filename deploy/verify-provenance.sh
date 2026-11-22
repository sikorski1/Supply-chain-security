#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat >&2 <<'EOF'
Usage: verify-provenance.sh IMAGE_DIGEST_REF OUTPUT [PUBLIC_KEY]

Verify a Cosign key-based SLSA Provenance v1.0 attestation for a digest-pinned
image in the local registry and write the verified DSSE envelopes as a JSON array.
Set COSIGN to use a specific Cosign binary (default: cosign on PATH).
EOF
  exit 2
}

[[ $# -eq 2 || $# -eq 3 ]] || usage
image_ref=$1
output=$2

[[ "$image_ref" == *@sha256:* ]] || {
  echo "Refusing to verify non-digest reference: $image_ref" >&2
  exit 2
}

repo_dir=$(CDPATH= cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)
public_key=${3:-"$repo_dir/security/cosign.pub"}
[[ -f "$public_key" ]] || {
  echo "Cosign public key not found: $public_key" >&2
  exit 1
}

cosign=${COSIGN:-cosign}
raw_output=$(mktemp -- "$output.raw.XXXXXX")
temporary_output=$(mktemp -- "$output.tmp.XXXXXX")
cleanup() {
  rm -f -- "$raw_output" "$temporary_output"
}
trap cleanup EXIT

"$cosign" verify-attestation \
  --key "$public_key" \
  --type slsaprovenance1 \
  --insecure-ignore-tlog \
  --allow-http-registry \
  --allow-insecure-registry \
  --output json \
  "$image_ref" > "$raw_output"

# Cosign emits one JSON DSSE envelope per line (NDJSON); normalize to a valid JSON array.
python3 - "$raw_output" "$temporary_output" <<'PY'
import base64
import json
import sys
from pathlib import Path

raw_text = Path(sys.argv[1]).read_text()
envelopes = [json.loads(line) for line in raw_text.splitlines() if line.strip()]
if not envelopes:
    sys.exit('No verified SLSA provenance attestations returned.')

for envelope in envelopes:
    stmt = json.loads(base64.b64decode(envelope['payload']).decode('utf-8'))
    if stmt.get('predicateType') != 'https://slsa.dev/provenance/v1':
        sys.exit(f"Unexpected predicateType: {stmt.get('predicateType')}")

Path(sys.argv[2]).write_text(json.dumps(envelopes, indent=2) + '\n')
PY

mv -T -- "$temporary_output" "$output"

#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat >&2 <<'EOF'
Usage: verify-image.sh IMAGE_DIGEST_REF OUTPUT [PUBLIC_KEY]

Verify a Cosign key-based signature for a digest-pinned image in the local registry
and write the verification JSON payload to OUTPUT.
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
temporary_output=$(mktemp -- "$output.tmp.XXXXXX")
cleanup() {
  rm -f -- "$temporary_output"
}
trap cleanup EXIT

"$cosign" verify \
  --key "$public_key" \
  --insecure-ignore-tlog \
  --allow-http-registry \
  --allow-insecure-registry \
  --output json \
  "$image_ref" > "$temporary_output"

mv -T -- "$temporary_output" "$output"

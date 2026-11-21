#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat >&2 <<'EOF'
Usage: sign-image.sh IMAGE_DIGEST_REF [SIGNING_CONFIG]

Sign a pushed container image digest with Cosign using key-based offline signing.
IMAGE_DIGEST_REF must be pinned by digest (e.g. localhost:5001/orders@sha256:...).
Requires COSIGN_PRIVATE_KEY in the environment (optional COSIGN_PASSWORD).
Set COSIGN to use a specific Cosign binary (default: cosign on PATH).
EOF
  exit 2
}

[[ $# -eq 1 || $# -eq 2 ]] || usage
image_ref=$1

[[ "$image_ref" == *@sha256:* ]] || {
  echo "Refusing to sign non-digest reference: $image_ref" >&2
  exit 2
}
[[ -n "${COSIGN_PRIVATE_KEY:-}" ]] || {
  echo "COSIGN_PRIVATE_KEY environment variable is required." >&2
  exit 1
}

repo_dir=$(CDPATH= cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)
signing_config=${2:-"$repo_dir/security/cosign-signing-config.json"}
[[ -f "$signing_config" ]] || {
  echo "Cosign signing config not found: $signing_config" >&2
  exit 1
}

cosign=${COSIGN:-cosign}
COSIGN_PASSWORD="${COSIGN_PASSWORD:-}" \
  "$cosign" sign \
    --yes \
    --key env://COSIGN_PRIVATE_KEY \
    --signing-config "$signing_config" \
    --allow-http-registry \
    --allow-insecure-registry \
    "$image_ref"

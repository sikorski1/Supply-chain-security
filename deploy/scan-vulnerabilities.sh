#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat >&2 <<'EOF'
Usage: scan-vulnerabilities.sh SBOM_INPUT OUTPUT

Scan a CycloneDX SBOM with Trivy using the pre-downloaded local vulnerability DB.
Set TRIVY to use a specific Trivy binary (default: trivy on PATH).
Set TRIVY_CACHE_DIR to override Trivy's cache directory.
EOF
  exit 2
}

[[ $# -eq 2 ]] || usage
sbom_input=$1
output=$2

[[ -f "$sbom_input" ]] || { echo "SBOM input not found: $sbom_input" >&2; exit 1; }

trivy=${TRIVY:-trivy}
temporary_output=$(mktemp -- "$output.tmp.XXXXXX")
cleanup() {
  rm -f -- "$temporary_output"
}
trap cleanup EXIT

cache_args=()
if [[ -n "${TRIVY_CACHE_DIR:-}" ]]; then
  cache_args=(--cache-dir "$TRIVY_CACHE_DIR")
fi

"$trivy" sbom \
  "${cache_args[@]}" \
  --skip-db-update \
  --skip-java-db-update \
  --skip-version-check \
  --disable-telemetry \
  --offline-scan \
  --exit-code 0 \
  --format json \
  --output "$temporary_output" \
  "$sbom_input"

mv -T -- "$temporary_output" "$output"

#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat >&2 <<'EOF'
Usage: generate-sbom.sh source COMPONENT COMPONENT_DIR OUTPUT
       generate-sbom.sh image IMAGE OUTPUT

COMPONENT_DIR is relative to the repository root. IMAGE must be in local Docker.
Set SYFT to use a specific Syft binary. The default is syft on PATH.
EOF
  exit 2
}

case "${1:-}" in
  source)
    [[ $# -eq 4 ]] || usage
    output=$4
    ;;
  image)
    [[ $# -eq 3 ]] || usage
    output=$3
    ;;
  *) usage ;;
esac

syft=${SYFT:-syft}
export SYFT_CHECK_FOR_APP_UPDATE=false
temporary_output=$(mktemp -- "$output.tmp.XXXXXX")
snapshot=
cleanup() {
  rm -f -- "$temporary_output"
  if [[ -n "$snapshot" ]]; then
    rm -rf -- "$snapshot"
  fi
}
trap cleanup EXIT

case "$1" in
  source)
    component=$2
    component_dir=$3
    repo_dir=$(CDPATH= cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)
    revision=$(git -C "$repo_dir" rev-parse HEAD)
    snapshot=$(mktemp -d -- "$(dirname -- "$output")/.sbom-source.XXXXXX")
    git -C "$repo_dir" archive "$revision:$component_dir" | tar -x -C "$snapshot"
    GOPROXY=off GONOPROXY=none SYFT_JAVASCRIPT_INCLUDE_DEV_DEPENDENCIES=true \
      "$syft" scan "dir:$snapshot" --source-name "$component" --source-version "$revision" \
        -o "cyclonedx-json=$temporary_output"
    ;;
  image)
    "$syft" scan "docker:$2" -o "cyclonedx-json=$temporary_output"
    ;;
esac

mv -T -- "$temporary_output" "$output"

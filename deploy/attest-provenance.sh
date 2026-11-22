#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat >&2 <<'EOF'
Usage: attest-provenance.sh IMAGE_DIGEST_REF PREDICATE_OUTPUT [SIGNING_CONFIG]

Generate a SLSA Provenance v1.0 predicate and attach it to a pushed image digest
as a signed in-toto DSSE attestation using Cosign offline key-based signing.
IMAGE_DIGEST_REF must be pinned by digest (e.g. localhost:5001/orders@sha256:...).
Requires COSIGN_PRIVATE_KEY in the environment (optional COSIGN_PASSWORD).
Set COSIGN to use a specific Cosign binary (default: cosign on PATH).
EOF
  exit 2
}

[[ $# -eq 2 || $# -eq 3 ]] || usage
image_ref=$1
predicate_output=$2

[[ "$image_ref" == *@sha256:* ]] || {
  echo "Refusing to attest non-digest reference: $image_ref" >&2
  exit 2
}
[[ -n "${COSIGN_PRIVATE_KEY:-}" ]] || {
  echo "COSIGN_PRIVATE_KEY environment variable is required." >&2
  exit 1
}

repo_dir=$(CDPATH= cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)
signing_config=${3:-"$repo_dir/security/cosign-signing-config.json"}
[[ -f "$signing_config" ]] || {
  echo "Cosign signing config not found: $signing_config" >&2
  exit 1
}

cosign=${COSIGN:-cosign}
temporary_predicate=$(mktemp -- "$predicate_output.tmp.XXXXXX")
cleanup() {
  rm -f -- "$temporary_predicate"
}
trap cleanup EXIT

git_commit=${GITHUB_SHA:-$(git -C "$repo_dir" rev-parse HEAD)}
git_ref=${GITHUB_REF:-$(git -C "$repo_dir" symbolic-ref -q HEAD || echo 'refs/heads/main')}
server_url=${GITHUB_SERVER_URL:-https://github.com}
repository=${GITHUB_REPOSITORY:-sikorski1/Supply-chain-security}
workflow_ref=${GITHUB_WORKFLOW_REF:-"$repository/.github/workflows/benchmark-l4.yaml@$git_ref"}
workflow_path=${workflow_ref#"$repository/"}
workflow_path=${workflow_path%@*}
run_id=${GITHUB_RUN_ID:-local}
run_attempt=${GITHUB_RUN_ATTEMPT:-1}
runner_name=${RUNNER_NAME:-local}
component=${COMPONENT:-unknown}
component_dir=${COMPONENT_DIR:-.}
benchmark_level=${BENCHMARK_LEVEL:-l4}

started_on=''
if [[ -n "${RESULT_DIR:-}" && -f "$RESULT_DIR/started-at" ]]; then
  started_on=$(tr -d '\r\n' < "$RESULT_DIR/started-at")
fi

python3 - "$temporary_predicate" <<'PY' \
  "$server_url" "$repository" "$git_ref" "$git_commit" "$workflow_path" \
  "$component" "$component_dir" "$benchmark_level" "$run_id" "$run_attempt" \
  "$runner_name" "$started_on"
import json
import sys
from datetime import datetime, timezone
from pathlib import Path

(
    output_path,
    server_url,
    repository,
    git_ref,
    git_commit,
    workflow_path,
    component,
    component_dir,
    benchmark_level,
    run_id,
    run_attempt,
    runner_name,
    started_on,
) = sys.argv[1:14]

now_utc = datetime.now(timezone.utc).strftime('%Y-%m-%dT%H:%M:%S.%fZ')
repo_url = f'{server_url.rstrip("/")}/{repository}'
predicate = {
    'buildDefinition': {
        'buildType': 'https://slsa-framework.github.io/github-actions-buildtypes/workflow/v1',
        'externalParameters': {
            'workflow': {
                'ref': git_ref,
                'repository': repo_url,
                'path': workflow_path,
            },
            'component': component,
            'context': component_dir,
            'dockerfile': f'{component_dir.rstrip("/")}/Dockerfile',
        },
        'internalParameters': {
            'benchmark_level': benchmark_level,
            'github_run_id': run_id,
            'github_run_attempt': run_attempt,
            'runner_name': runner_name,
        },
        'resolvedDependencies': [
            {
                'uri': f'git+{repo_url}@{git_ref}',
                'digest': {
                    'gitCommit': git_commit,
                },
            }
        ],
    },
    'runDetails': {
        'builder': {
            'id': f'{repo_url}/runners/{runner_name}',
        },
        'metadata': {
            'invocationId': f'{repo_url}/actions/runs/{run_id}/attempts/{run_attempt}',
            'startedOn': started_on or now_utc,
            'finishedOn': now_utc,
        },
    },
}
Path(output_path).write_text(json.dumps(predicate, indent=2) + '\n')
PY

mv -T -- "$temporary_predicate" "$predicate_output"

COSIGN_PASSWORD="${COSIGN_PASSWORD:-}" \
  "$cosign" attest \
    --yes \
    --key env://COSIGN_PRIVATE_KEY \
    --signing-config "$signing_config" \
    --type slsaprovenance1 \
    --predicate "$predicate_output" \
    --allow-http-registry \
    --allow-insecure-registry \
    "$image_ref"

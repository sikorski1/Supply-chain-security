#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat >&2 <<'EOF'
Usage: evaluate-policy.sh RESULT_DIR COMPONENT DEPLOYMENT_YAML IMAGE_TAG_DIGEST OUTPUT

Assemble all L1–L4 security artifacts and the rendered Kubernetes Deployment
manifest into an OPA input document, evaluate security/policies/ via `opa eval`,
write the structured JSON decision to OUTPUT, and exit non-zero if denied.
Set OPA to use a specific OPA binary (default: opa on PATH).
EOF
  exit 2
}

[[ $# -eq 5 ]] || usage
result_dir=$1
component=$2
deployment_yaml=$3
image_tag_digest=$4
output=$5

repo_dir=$(CDPATH= cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)
policy_dir="$repo_dir/security/policies"
opa=${OPA:-opa}

[[ -f "$deployment_yaml" ]] || { echo "Deployment manifest not found: $deployment_yaml" >&2; exit 1; }

input_file=$(mktemp -- "$output.input.XXXXXX")
rendered_manifest=$(mktemp -- "$output.k8s.XXXXXX")
temporary_output=$(mktemp -- "$output.tmp.XXXXXX")
cleanup() {
  rm -f -- "$input_file" "$rendered_manifest" "$temporary_output"
}
trap cleanup EXIT

kubectl set image --local -f "$deployment_yaml" "$component=$image_tag_digest" -o json > "$rendered_manifest"

git_commit=${GITHUB_SHA:-$(git -C "$repo_dir" rev-parse HEAD)}
server_url=${GITHUB_SERVER_URL:-https://github.com}
repository=${GITHUB_REPOSITORY:-sikorski1/Supply-chain-security}
trusted_repo="${server_url%/}/${repository}"
allowed_registry_prefix=${ALLOWED_REGISTRY_PREFIX:-localhost:5001/}

python3 - "$result_dir" "$rendered_manifest" "$input_file" \
  "$component" "$git_commit" "$trusted_repo" "$allowed_registry_prefix" <<'PY'
import base64
import json
import sys
from pathlib import Path

result_dir = Path(sys.argv[1])
manifest_path = Path(sys.argv[2])
input_path = Path(sys.argv[3])
component, git_commit, trusted_repo, allowed_prefix = sys.argv[4:8]


def load_json_if_exists(path: Path):
    return json.loads(path.read_text()) if path.exists() else None


digest_path = result_dir / 'image-digest'
image_digest = digest_path.read_text().strip() if digest_path.exists() else ''

envelopes = load_json_if_exists(result_dir / 'provenance-verify.json') or []
provenance_statements = []
for env in envelopes:
    payload = env.get('payload')
    if payload:
        provenance_statements.append(json.loads(base64.b64decode(payload).decode('utf-8')))

doc = {
    'context': {
        'component': component,
        'commit_sha': git_commit,
        'image_digest': image_digest,
        'trusted_repository': trusted_repo,
        'allowed_registry_prefix': allowed_prefix,
    },
    'sbom_source': load_json_if_exists(result_dir / 'sbom-source.cdx.json'),
    'sbom_image': load_json_if_exists(result_dir / 'sbom-image.cdx.json'),
    'trivy_source': load_json_if_exists(result_dir / 'trivy-source.json'),
    'trivy_image': load_json_if_exists(result_dir / 'trivy-image.json'),
    'cosign_verify': load_json_if_exists(result_dir / 'cosign-verify.json') or [],
    'provenance_statements': provenance_statements,
    'deployment': json.loads(manifest_path.read_text()),
}
input_path.write_text(json.dumps(doc))
PY

"$opa" eval \
  --data "$policy_dir/sbom.rego" \
  --data "$policy_dir/vulnerabilities.rego" \
  --data "$policy_dir/provenance.rego" \
  --data "$policy_dir/k8s.rego" \
  --data "$policy_dir/decision.rego" \
  --input "$input_file" \
  --format raw \
  'data.supplychain.policy.result' > "$temporary_output"

mv -T -- "$temporary_output" "$output"

python3 - "$output" <<'PY'
import json
import sys
from pathlib import Path

res = json.loads(Path(sys.argv[1]).read_text())
summary = res.get('summary', {})
print(
    f"OPA policy decision: allow={res.get('allow')} "
    f"denies={res.get('deny_count')} warnings={res.get('warn_count')} "
    f"sig={summary.get('signature_verified')} prov={summary.get('provenance_verified')}"
)
for msg in res.get('deny', []):
    print(f"DENY: {msg}", file=sys.stderr)
if not res.get('allow'):
    sys.exit(1)
PY

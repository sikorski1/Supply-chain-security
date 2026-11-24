#!/usr/bin/env bash
set -euo pipefail

deploy_dir=$(CDPATH= cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
repo_dir=$(CDPATH= cd -- "$deploy_dir/.." && pwd)
kube=(kubectl --context "${KUBE_CONTEXT:-kind-benchmark}")
namespace=${KUBE_NAMESPACE:-benchmark}
enable_gatekeeper=${ENABLE_GATEKEEPER:-false}

curl --fail --silent --show-error --max-time 10 http://localhost:5001/v2/

# Clear stale Cosign OCI referrers-tag indices (sha256-*) before each run so
# repeated runs on identical digests always verify 1 signature and 1 attestation.
python3 - <<'PY'
import json
import sys
import urllib.error
import urllib.request

BASE = 'http://localhost:5001'
COMPONENTS = ('inventory', 'payments', 'orders', 'api-gateway', 'notifications', 'analytics', 'frontend')
ACCEPT = ', '.join([
    'application/vnd.oci.image.index.v1+json',
    'application/vnd.docker.distribution.manifest.list.v2+json',
    'application/vnd.oci.image.manifest.v1+json',
    'application/vnd.docker.distribution.manifest.v2+json',
])

deleted = 0
for comp in COMPONENTS:
    try:
        with urllib.request.urlopen(f'{BASE}/v2/{comp}/tags/list', timeout=5) as resp:
            tags = json.loads(resp.read().decode()).get('tags') or []
    except urllib.error.HTTPError as exc:
        if exc.code == 404:
            continue
        raise

    for tag in tags:
        if not tag.startswith('sha256-'):
            continue
        head_req = urllib.request.Request(
            f'{BASE}/v2/{comp}/manifests/{tag}',
            headers={'Accept': ACCEPT},
            method='HEAD',
        )
        try:
            with urllib.request.urlopen(head_req, timeout=5) as resp:
                digest = resp.headers.get('Docker-Content-Digest')
        except urllib.error.HTTPError as exc:
            if exc.code == 404:
                continue
            raise
        if not digest:
            continue
        del_req = urllib.request.Request(f'{BASE}/v2/{comp}/manifests/{digest}', method='DELETE')
        try:
            with urllib.request.urlopen(del_req, timeout=5):
                deleted += 1
        except urllib.error.HTTPError as exc:
            if exc.code == 405:
                print(
                    'Registry rejected DELETE (405). Recreate registry with: bash deploy/setup-kind.sh',
                    file=sys.stderr,
                )
                sys.exit(1)
            if exc.code != 404:
                raise

if deleted:
    print(f'Cleared {deleted} stale Cosign referrer index manifest(s) from localhost:5001.')
PY

"${kube[@]}" wait --for=condition=Ready nodes --all --timeout=120s
"${kube[@]}" apply -f "$deploy_dir/k8s/namespace.yaml"

if [[ "$enable_gatekeeper" == 'true' ]]; then
  if ! "${kube[@]}" -n gatekeeper-system get deployment gatekeeper-controller-manager >/dev/null 2>&1; then
    echo "OPA Gatekeeper is not installed in ${KUBE_CONTEXT:-kind-benchmark}. Run: bash deploy/setup-gatekeeper.sh" >&2
    exit 1
  fi
  "${kube[@]}" -n gatekeeper-system rollout status deployment/gatekeeper-controller-manager --timeout=180s
  "${kube[@]}" -n gatekeeper-system wait --for=condition=Ready pod -l control-plane=controller-manager --timeout=120s
  "${kube[@]}" apply -f "$repo_dir/security/gatekeeper/template-benchmark-workload.yaml"
  "${kube[@]}" wait --for=jsonpath='{.status.created}'=true \
    constrainttemplate/k8sbenchmarksupplychainpolicy --timeout=60s
  "${kube[@]}" wait --for=condition=Established \
    crd/k8sbenchmarksupplychainpolicy.constraints.gatekeeper.sh --timeout=120s
  "${kube[@]}" apply -f "$repo_dir/security/gatekeeper/constraint-benchmark-workload.yaml"
  # Enable Gatekeeper admission webhook for L5 on the benchmark namespace.
  "${kube[@]}" label namespace "$namespace" supply-chain-benchmark/gatekeeper=enabled --overwrite >/dev/null
else
  # Bypass Gatekeeper webhook for L0–L4 so earlier levels have zero admission webhook overhead.
  "${kube[@]}" label namespace "$namespace" supply-chain-benchmark/gatekeeper- >/dev/null 2>&1 || true
fi

"${kube[@]}" -n "$namespace" apply -f "$deploy_dir/k8s/support.yaml"
for workload in postgres nats; do
  "${kube[@]}" -n "$namespace" rollout status "deployment/$workload" --timeout=180s
done

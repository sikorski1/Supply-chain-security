#!/usr/bin/env bash
set -euo pipefail

deploy_dir=$(CDPATH= cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
repo_dir=$(CDPATH= cd -- "$deploy_dir/.." && pwd)
kube=(kubectl --context "${KUBE_CONTEXT:-kind-benchmark}")
namespace=${KUBE_NAMESPACE:-benchmark}
enable_gatekeeper=${ENABLE_GATEKEEPER:-false}

curl --fail --silent --show-error --max-time 10 http://localhost:5001/v2/
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

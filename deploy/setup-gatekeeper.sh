#!/usr/bin/env bash
set -euo pipefail

# One-time infrastructure bootstrap for OPA Gatekeeper (outside timed benchmark execution).
deploy_dir=$(CDPATH= cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
repo_dir=$(CDPATH= cd -- "$deploy_dir/.." && pwd)
kube=(kubectl --context "${KUBE_CONTEXT:-kind-benchmark}")
namespace=${KUBE_NAMESPACE:-benchmark}
gatekeeper_version=${GATEKEEPER_VERSION:-v3.23.1}
manifest_url="https://raw.githubusercontent.com/open-policy-agent/gatekeeper/${gatekeeper_version}/deploy/gatekeeper.yaml"

for command in kubectl curl; do
  command -v "$command" >/dev/null || { echo "Missing command: $command" >&2; exit 1; }
done

"${kube[@]}" wait --for=condition=Ready nodes --all --timeout=120s
"${kube[@]}" apply -f "$deploy_dir/k8s/namespace.yaml"
# Keep benchmark namespace opted out by default so L0–L4 bypass Gatekeeper with zero webhook overhead.
"${kube[@]}" label namespace "$namespace" supply-chain-benchmark/gatekeeper- >/dev/null 2>&1 || true

echo "Installing OPA Gatekeeper ${gatekeeper_version} into ${KUBE_CONTEXT:-kind-benchmark}..."
"${kube[@]}" apply -f "$manifest_url"
# Single-node kind cluster only needs one webhook replica to minimize background resource footprint.
"${kube[@]}" -n gatekeeper-system scale deployment/gatekeeper-controller-manager --replicas=1

"${kube[@]}" -n gatekeeper-system rollout status deployment/gatekeeper-controller-manager --timeout=180s
"${kube[@]}" -n gatekeeper-system wait --for=condition=Ready pod -l control-plane=controller-manager --timeout=120s
"${kube[@]}" -n gatekeeper-system rollout status deployment/gatekeeper-audit --timeout=180s

# Scope Gatekeeper's validating webhook to namespaces opted in via supply-chain-benchmark/gatekeeper=enabled
# and make enforcement fail-closed for L5.
"${kube[@]}" patch validatingwebhookconfiguration gatekeeper-validating-webhook-configuration \
  --type=json \
  -p='[
    {"op":"replace","path":"/webhooks/0/failurePolicy","value":"Fail"},
    {"op":"replace","path":"/webhooks/0/namespaceSelector","value":{"matchExpressions":[{"key":"supply-chain-benchmark/gatekeeper","operator":"In","values":["enabled"]}]}}
  ]'

"${kube[@]}" apply -f "$repo_dir/security/gatekeeper/template-benchmark-workload.yaml"
"${kube[@]}" wait --for=jsonpath='{.status.created}'=true \
  constrainttemplate/k8sbenchmarksupplychainpolicy --timeout=60s
"${kube[@]}" wait --for=condition=Established \
  crd/k8sbenchmarksupplychainpolicy.constraints.gatekeeper.sh --timeout=120s
"${kube[@]}" apply -f "$repo_dir/security/gatekeeper/constraint-benchmark-workload.yaml"

echo "Ready: OPA Gatekeeper ${gatekeeper_version} installed (benchmark namespace opted out until L5 enables it)."

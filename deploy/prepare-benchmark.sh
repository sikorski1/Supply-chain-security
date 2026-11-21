#!/usr/bin/env bash
set -euo pipefail

deploy_dir=$(CDPATH= cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
kube=(kubectl --context "${KUBE_CONTEXT:-kind-benchmark}")
namespace=${KUBE_NAMESPACE:-benchmark}

curl --fail --silent --show-error --max-time 10 http://localhost:5001/v2/
"${kube[@]}" wait --for=condition=Ready nodes --all --timeout=120s
"${kube[@]}" apply -f "$deploy_dir/k8s/namespace.yaml"
"${kube[@]}" -n "$namespace" apply -f "$deploy_dir/k8s/support.yaml"
for workload in postgres nats; do
  "${kube[@]}" -n "$namespace" rollout status "deployment/$workload" --timeout=180s
done

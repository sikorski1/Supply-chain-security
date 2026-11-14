#!/usr/bin/env bash
set -euo pipefail

# This is an integration check, not part of timed benchmark execution.
deploy_dir=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
benchmark_context=kind-benchmark
smoke_namespace=benchmark-registry-check
smoke_image=localhost:5001/registry-smoke:check

for command in docker kubectl curl; do
  command -v "$command" >/dev/null || { echo "Missing command: $command" >&2; exit 1; }
done

discovery=$(kubectl --context "$benchmark_context" -n kube-public get configmap \
  local-registry-hosting -o 'jsonpath={.data.localRegistryHosting\.v1}')
[[ "$discovery" == *'host: "localhost:5001"'* ]] || {
  echo 'Registry discovery must advertise localhost:5001.' >&2
  exit 1
}
curl --fail --silent --show-error http://localhost:5001/v2/ >/dev/null

# Refuse to replace another check's namespace.
kubectl --context "$benchmark_context" create namespace "$smoke_namespace"
cleanup() {
  kubectl --context "$benchmark_context" delete namespace "$smoke_namespace" \
    --ignore-not-found --timeout=60s >/dev/null
}
trap cleanup EXIT

registry_image=$(docker compose -p benchmark-infra \
  -f "$deploy_dir/compose.benchmark.yaml" config --images)
docker pull "$registry_image"
docker tag "$registry_image" "$smoke_image"
docker push "$smoke_image"

# Reusing the registry image gives us /bin/sh without another base dependency.
# Always pull through containerd; do not use `kind load docker-image` here.
kubectl --context "$benchmark_context" -n "$smoke_namespace" run registry-smoke \
  --image="$smoke_image" --image-pull-policy=Always --restart=Never \
  --command -- /bin/sh -c 'printf registry-pull-ok'
if ! kubectl --context "$benchmark_context" -n "$smoke_namespace" wait \
  --for=jsonpath='{.status.phase}'=Succeeded pod/registry-smoke --timeout=90s; then
  kubectl --context "$benchmark_context" -n "$smoke_namespace" describe pod registry-smoke
  exit 1
fi
output=$(kubectl --context "$benchmark_context" -n "$smoke_namespace" logs registry-smoke)
[[ "$output" == registry-pull-ok ]]
echo 'Verified host push, registry pull by Kubernetes, and Pod execution.'
echo 'The smoke image remains in registry storage and node caches.'

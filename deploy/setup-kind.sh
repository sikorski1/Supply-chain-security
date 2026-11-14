#!/usr/bin/env bash
set -euo pipefail

# Infrastructure bootstrap is outside measured benchmark execution.
deploy_dir=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
cluster_name=benchmark
node_image=kindest/node:v1.37.0@sha256:a1ed56cfb0e7b93589bdf97c8cd566405a265939e3620fc4f5de89adff580ae5
registry_dir=/etc/containerd/certs.d/localhost:5001
export KIND_EXPERIMENTAL_PROVIDER=docker

for command in docker kind kubectl; do
  command -v "$command" >/dev/null || { echo "Missing command: $command" >&2; exit 1; }
done
docker info >/dev/null
docker compose version >/dev/null

clusters=$(kind get clusters)
if [[ $'\n'"$clusters"$'\n' != *$'\n'"$cluster_name"$'\n'* ]]; then
  kind create cluster --name "$cluster_name" --config "$deploy_dir/kind.yaml" \
    --image "$node_image" --wait 120s
else
  echo "Reusing kind cluster: $cluster_name"
  kind export kubeconfig --name "$cluster_name"
fi

nodes=$(kind get nodes --name "$cluster_name")
[[ -n "$nodes" ]] || { echo 'No benchmark cluster nodes found.' >&2; exit 1; }
while IFS= read -r node; do
  actual_image=$(docker inspect --format '{{.Config.Image}}' "$node")
  if [[ "$actual_image" != "$node_image" ]]; then
    echo "Node $node uses $actual_image; expected $node_image." >&2
    echo 'Delete and recreate the benchmark cluster to apply the pinned image.' >&2
    exit 1
  fi
done <<< "$nodes"

docker compose -p benchmark-infra -f "$deploy_dir/compose.benchmark.yaml" \
  up -d --wait --wait-timeout 60
registry_id=$(docker compose -p benchmark-infra \
  -f "$deploy_dir/compose.benchmark.yaml" ps -q registry)
registry_name=$(docker inspect --format '{{.Name}}' "$registry_id")
registry_name=${registry_name#/}
if [[ $(docker inspect --format '{{json .NetworkSettings.Networks.kind}}' "$registry_id") == null ]]; then
  docker network connect kind "$registry_id"
fi

# Containerd translates the public image name to the registry's Docker DNS name.
# Modern kind node images already enable /etc/containerd/certs.d.
while IFS= read -r node; do
  docker exec "$node" mkdir -p "$registry_dir"
  docker exec -i "$node" cp /dev/stdin "$registry_dir/hosts.toml" <<EOF
[host."http://$registry_name:5000"]
EOF
done <<< "$nodes"

kubectl --context "kind-$cluster_name" apply -f - <<'EOF'
apiVersion: v1
kind: ConfigMap
metadata:
  name: local-registry-hosting
  namespace: kube-public
data:
  localRegistryHosting.v1: |
    host: "localhost:5001"
    help: "https://kind.sigs.k8s.io/docs/user/local-registry/"
EOF
kubectl --context "kind-$cluster_name" wait --for=condition=Ready nodes --all --timeout=120s
echo "Ready: context kind-$cluster_name; registry localhost:5001"

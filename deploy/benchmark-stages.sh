#!/usr/bin/env bash

# Source this from the component job to record timings in the workflow shell.
now() {
  python3 -c 'import time; print(time.monotonic_ns())'
}

record_stage() {
  local end
  end=$(now)
  printf '%s\t%s\n' "$stage" "$((end - stage_start))" >> "$RESULT_DIR/stages.tsv"
  printf '%s\n' "$end" > "$RESULT_DIR/end"
  echo '::endgroup::'
}

run_stage() {
  stage=$1
  shift
  echo "::group::$stage"
  stage_start=$(now)
  trap record_stage EXIT
  "$@"
  record_stage
  trap - EXIT
}

install_dependencies() (
  CDPATH= cd -- "$COMPONENT_DIR"
  case "$RUNTIME" in
    go) go mod download ;;
    bun) bun install --frozen-lockfile ;;
    python) uv sync --locked --python "$pythonLocation/bin/python3" ;;
  esac
)

test_analytics() (
  # Test events and database rows belong to a disposable Compose project.
  local override="$RESULT_DIR/analytics-ports.yaml"
  cat > "$override" <<'YAML'
services:
  postgres:
    ports: ["127.0.0.1::5432"]
    healthcheck:
      test: ["CMD", "pg_isready", "-h", "127.0.0.1", "-U", "app", "-d", "supply_chain"]
  nats:
    ports: ["127.0.0.1::4222"]
YAML
  local compose=(docker compose -p "${BENCHMARK_LEVEL}-analytics-$GITHUB_RUN_ID-$GITHUB_RUN_ATTEMPT"
    -f "$1" -f "$override")
  trap '"${compose[@]}" down --volumes || exit 1' EXIT
  "${compose[@]}" up -d --wait --wait-timeout 60 postgres nats

  local postgres nats report="$RESULT_DIR/pytest.xml"
  postgres=$("${compose[@]}" port postgres 5432)
  nats=$("${compose[@]}" port nats 4222)
  export GOOSE_DB_STRING="postgresql://app:app@$postgres/supply_chain?sslmode=disable"
  export NATS_URL="nats://$nats"
  uv run --no-sync pytest -q -rs --junitxml="$report"
  python3 - "$report" <<'PY'
import sys
import xml.etree.ElementTree as ET

if ET.parse(sys.argv[1]).find('.//skipped') is not None:
    sys.exit('Analytics benchmark requires all tests to run.')
PY
)

run_tests() (
  local compose_file="$PWD/deploy/docker-compose.yaml"
  CDPATH= cd -- "$COMPONENT_DIR"
  case "$RUNTIME" in
    go) go test -race -count=1 ./... ;;
    bun) bun run test ;;
    python) test_analytics "$compose_file" ;;
  esac
)

push_image_with_digest() {
  docker push "$IMAGE" | tee "$RESULT_DIR/push.log"
  awk -v tag="${IMAGE##*:}:" '
    $1 == tag && $2 == "digest:" && $3 ~ /^sha256:[0-9a-f]+$/ { digest = $3 }
    END {
      if (length(digest) != 71) {
        print "Could not read the pushed image digest." > "/dev/stderr"
        exit 1
      }
      print digest
    }
  ' "$RESULT_DIR/push.log" > "$RESULT_DIR/image-digest"
}

deploy_image() {
  local kube=(kubectl --context "$KUBE_CONTEXT" -n "$KUBE_NAMESPACE")
  "${kube[@]}" set image --local -f "deploy/k8s/$COMPONENT.yaml" "$COMPONENT=$IMAGE" -o yaml |
    "${kube[@]}" apply -f -
  "${kube[@]}" rollout status "deployment/$COMPONENT" --timeout=180s
}

image_digest_ref() {
  local digest
  digest=$(tr -d '\r\n' < "$RESULT_DIR/image-digest")
  [[ "$digest" =~ ^sha256:[0-9a-f]{64}$ ]] || {
    echo "Missing or invalid pushed image digest in $RESULT_DIR/image-digest." >&2
    return 1
  }
  printf '%s@%s\n' "${IMAGE%:*}" "$digest"
}

deploy_image_by_digest() {
  local digest kube=(kubectl --context "$KUBE_CONTEXT" -n "$KUBE_NAMESPACE")
  digest=$(tr -d '\r\n' < "$RESULT_DIR/image-digest")
  [[ "$digest" =~ ^sha256:[0-9a-f]{64}$ ]] || {
    echo "Missing or invalid pushed image digest in $RESULT_DIR/image-digest." >&2
    return 1
  }
  # Tag + digest pins the immutable manifest while keeping a unique per-run spec so Kubernetes rolls out.
  "${kube[@]}" set image --local -f "deploy/k8s/$COMPONENT.yaml" "$COMPONENT=$IMAGE@$digest" -o yaml |
    "${kube[@]}" apply -f -
  "${kube[@]}" rollout status "deployment/$COMPONENT" --timeout=180s
}

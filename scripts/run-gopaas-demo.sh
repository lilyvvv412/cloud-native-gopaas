#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

export PATH="${HOME}/sdk/go/bin:${HOME}/go/bin:/opt/homebrew/bin:${PATH}"
export CONSUL_ADDRS="${CONSUL_ADDRS:-127.0.0.1:8500}"
export GOPAAS_MODE="${GOPAAS_MODE:-memory}"

GATEWAY_URL="${GATEWAY_URL:-http://127.0.0.1:8080}"
PASS=0
FAIL=0

ok() { echo "✔ $1"; PASS=$((PASS+1)); }
bad() { echo "✘ $1"; FAIL=$((FAIL+1)); }

need_binaries() {
  if [[ ! -x bin/api-gateway || ! -x bin/resource-service || ! -x bin/scheduler-service ]]; then
    echo "building binaries..."
    make build
  fi
}

start_consul() {
  if curl -fsS "http://${CONSUL_ADDRS}/v1/status/leader" >/dev/null 2>&1; then
    ok "Consul already available at ${CONSUL_ADDRS}"
    return
  fi
  if ! command -v docker >/dev/null 2>&1; then
    echo "Consul is not running and docker is unavailable. Start Consul on :8500 first."
    exit 1
  fi
  docker rm -f gopaas-consul >/dev/null 2>&1 || true
  docker run -d --name gopaas-consul -p 8500:8500 hashicorp/consul:1.17 \
    agent -dev -client=0.0.0.0 -ui >/dev/null
  for i in $(seq 1 30); do
    if curl -fsS "http://${CONSUL_ADDRS}/v1/status/leader" >/dev/null 2>&1; then
      ok "Consul started"
      return
    fi
    sleep 1
  done
  bad "Consul failed to start"
  exit 1
}

seed_config() {
  curl -fsS -X PUT "http://${CONSUL_ADDRS}/v1/kv/gopaas/config" \
    -d '{"rate_limit_qps":20,"feature_flags":{"auth":false}}' >/dev/null
  ok "Consul KV config seeded"
}

start_services() {
  mkdir -p .tmp

  # First use the PID files left by the previous run. They are more precise
  # than port-wide cleanup and keep unrelated local services untouched.
  for pair in "api-gateway:.tmp/gateway.pid" "resource-service:.tmp/resource.pid" "scheduler-service:.tmp/scheduler.pid"; do
    name="${pair%%:*}"
    file="${pair#*:}"
    if [[ -f "$file" ]]; then
      old_pid="$(cat "$file" 2>/dev/null || true)"
      if [[ "$old_pid" =~ ^[0-9]+$ ]] && kill -0 "$old_pid" >/dev/null 2>&1; then
        command_line="$(ps -p "$old_pid" -o command= 2>/dev/null || true)"
        if [[ "$command_line" == *"${ROOT}/bin/${name}"* || "$command_line" == *"./bin/${name}"* ]]; then
          kill "$old_pid" >/dev/null 2>&1 || true
        fi
      fi
    fi
  done

  # Make the demo repeatable: stop only binaries belonging to this checkout.
  for name in api-gateway resource-service scheduler-service; do
    while read -r pid; do
      [[ -z "$pid" ]] && continue
      kill "$pid" >/dev/null 2>&1 || true
    done < <(pgrep -f "${ROOT}/bin/${name}" || true)
    # When launched from this script macOS may report the relative path.
    while read -r pid; do
      [[ -z "$pid" ]] && continue
      kill "$pid" >/dev/null 2>&1 || true
    done < <(pgrep -f "\./bin/${name}" || true)
  done
  sleep 0.5

  # Do not silently talk to a different deployment (for example Compose).
  for port in 8080 8081 8083 9091 9093 9095 9190 9191 9193; do
    listener="$(lsof -nP -tiTCP:"$port" -sTCP:LISTEN 2>/dev/null || true)"
    if [[ -n "$listener" ]]; then
      echo "Port $port is already in use (PID $listener). Stop the other GoPaaS mode first."
      echo "Hint: run 'make compose-down' before the local process demo."
      exit 1
    fi
  done

  SERVICE_HOST=127.0.0.1 SERVICE_PORT=8081 CONSUL_ADDRS="$CONSUL_ADDRS" \
    METRICS_PORT=9191 HYSTRIX_PORT=9091 GOPAAS_MODE="$GOPAAS_MODE" \
    ./bin/resource-service >.tmp/resource.log 2>&1 &
  echo $! >.tmp/resource.pid

  SERVICE_HOST=127.0.0.1 SERVICE_PORT=8083 CONSUL_ADDRS="$CONSUL_ADDRS" \
    METRICS_PORT=9193 HYSTRIX_PORT=9093 GOPAAS_MODE="$GOPAAS_MODE" \
    ./bin/scheduler-service >.tmp/scheduler.log 2>&1 &
  echo $! >.tmp/scheduler.pid

  SERVICE_HOST=127.0.0.1 SERVICE_PORT=8000 HTTP_PORT=8080 CONSUL_ADDRS="$CONSUL_ADDRS" \
    METRICS_PORT=9190 HYSTRIX_PORT=9095 RATE_LIMIT_QPS=20 GOPAAS_MODE="$GOPAAS_MODE" \
    ./bin/api-gateway >.tmp/gateway.log 2>&1 &
  echo $! >.tmp/gateway.pid

  for i in $(seq 1 40); do
    if curl -fsS "${GATEWAY_URL}/healthz" >/dev/null 2>&1; then
      ok "Gateway is reachable"
      return
    fi
    sleep 0.5
  done
  bad "Gateway not reachable"
  echo "---- gateway log ----"; tail -n 50 .tmp/gateway.log || true
  echo "---- resource log ----"; tail -n 50 .tmp/resource.log || true
  exit 1
}

cleanup() {
  for f in .tmp/gateway.pid .tmp/resource.pid .tmp/scheduler.pid; do
    if [[ -f "$f" ]]; then
      kill "$(cat "$f")" >/dev/null 2>&1 || true
    fi
  done
}
trap cleanup EXIT

check_discovery() {
  sleep 2
  services="$(curl -fsS "http://${CONSUL_ADDRS}/v1/catalog/services" || true)"
  echo "$services" | grep -q 'go.micro.service.resource' && ok "Consul discovered resource-service" || bad "resource-service not in Consul"
  echo "$services" | grep -q 'go.micro.service.scheduler' && ok "Consul discovered scheduler-service" || bad "scheduler-service not in Consul"
  echo "$services" | grep -q 'go.micro.api.gateway' && ok "Consul discovered api-gateway" || bad "api-gateway not in Consul"
}

check_grpc_path() {
  body='{"namespace":"default","name":"demo-web","image":"nginx:1.25","replicas":2,"container_port":80}'
  created=0
  for i in $(seq 1 10); do
    if curl -fsS -X POST "${GATEWAY_URL}/api/v1/deployments" -H 'Content-Type: application/json' -d "$body" >/dev/null 2>&1; then
      created=1
      break
    fi
    sleep 1
  done
  if [[ "$created" == "1" ]]; then
    ok "Gateway → gRPC create deployment succeeded"
  else
    bad "create deployment failed"
  fi

  if curl -fsS "${GATEWAY_URL}/api/v1/deployments/demo-web?namespace=default" | grep -q demo-web; then
    ok "gRPC get deployment succeeded"
  else
    bad "get deployment failed"
  fi

  if curl -fsS -X POST "${GATEWAY_URL}/api/v1/deployments/demo-web/scale?namespace=default" \
    -H 'Content-Type: application/json' -d '{"replicas":3}' | grep -q '"replicas":3'; then
    ok "scale deployment succeeded"
  else
    bad "scale deployment failed"
  fi

  task='{"namespace":"default","name":"demo-task","image":"busybox:1.36","schedule":"*/1 * * * *","command":"echo gopaas"}'
  submitted=0
  for i in $(seq 1 10); do
    if curl -fsS -X POST "${GATEWAY_URL}/api/v1/tasks" -H 'Content-Type: application/json' -d "$task" >/dev/null 2>&1; then
      submitted=1
      break
    fi
    sleep 1
  done
  if [[ "$submitted" == "1" ]]; then
    ok "scheduler submit task succeeded"
  else
    bad "scheduler submit failed"
  fi
}

check_rate_limit() {
  limited=0
  for i in $(seq 1 80); do
    code=$(curl -s -o /dev/null -w '%{http_code}' "${GATEWAY_URL}/api/v1/deployments?namespace=default" || true)
    if [[ "$code" == "429" ]]; then
      limited=1
      break
    fi
  done
  if [[ "$limited" == "1" ]]; then
    ok "rate limit rejected excess traffic (429)"
  else
    bad "rate limit did not trigger"
  fi
}

check_circuit() {
  if [[ -f .tmp/resource.pid ]]; then
    kill "$(cat .tmp/resource.pid)" >/dev/null 2>&1 || true
    sleep 1
  fi
  opened=0
  for i in $(seq 1 20); do
    body=$(curl -s -X POST "${GATEWAY_URL}/api/v1/deployments" -H 'Content-Type: application/json' \
      -d '{"namespace":"default","name":"cb-'"$i"'","image":"nginx:1.25","replicas":1}' || true)
    if echo "$body" | grep -qi 'circuit open'; then
      opened=1
      break
    fi
  done
  if [[ "$opened" == "1" ]]; then
    ok "circuit breaker opened after downstream failure"
  else
    bad "circuit breaker evidence not observed (check hystrix/metrics)"
  fi

  # restart resource for cleanup completeness
  SERVICE_HOST=127.0.0.1 SERVICE_PORT=8081 CONSUL_ADDRS="$CONSUL_ADDRS" \
    METRICS_PORT=9191 HYSTRIX_PORT=9091 GOPAAS_MODE="$GOPAAS_MODE" \
    ./bin/resource-service >.tmp/resource.log 2>&1 &
  echo $! >.tmp/resource.pid
  sleep 2
}

check_metrics() {
  if curl -fsS "http://127.0.0.1:9190/metrics" | grep -q gopaas_http_requests_total; then
    ok "Prometheus metrics exposed on gateway"
  else
    bad "gateway /metrics missing"
  fi
}

main() {
  echo "== GoPaaS local demo =="
  need_binaries
  start_consul
  seed_config
  start_services
  check_discovery
  check_grpc_path
  check_rate_limit
  check_circuit
  check_metrics

  echo
  echo "Passed: $PASS  Failed: $FAIL"
  if [[ "$FAIL" -gt 0 ]]; then
    exit 1
  fi
  echo
  echo "Demo evidence ready:"
  echo "  Gateway:   ${GATEWAY_URL}/healthz"
  echo "  Consul UI: http://127.0.0.1:8500"
  echo "  Metrics:   http://127.0.0.1:9190/metrics"
  echo "  Hystrix:   http://127.0.0.1:9095 (stream)"
}

main "$@"

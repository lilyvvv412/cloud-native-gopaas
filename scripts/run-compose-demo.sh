#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"
export PATH="${HOME}/sdk/bin:${HOME}/sdk/go/bin:${HOME}/go/bin:/opt/homebrew/bin:/usr/local/bin:${PATH}"

PASS=0
FAIL=0
ok() { echo "✔ $1"; PASS=$((PASS+1)); }
bad() { echo "✘ $1"; FAIL=$((FAIL+1)); }

echo "== GoPaaS compose monitoring demo =="
bash "${ROOT}/scripts/build-images.sh"
docker compose -f deploy/compose/docker-compose.yml up -d --force-recreate

echo "waiting for gateway..."
for i in $(seq 1 60); do
  if curl -fsS http://127.0.0.1:8080/healthz >/dev/null 2>&1; then
    ok "gateway healthy"
    break
  fi
  sleep 2
  if [[ $i -eq 60 ]]; then bad "gateway not healthy"; fi
done

created=0
for i in $(seq 1 30); do
  if curl -fsS -X POST http://127.0.0.1:8080/api/v1/deployments \
    -H 'Content-Type: application/json' \
    -d '{"namespace":"default","name":"compose-web","image":"nginx:1.25","replicas":1}' >/dev/null 2>&1; then
    created=1
    break
  fi
  sleep 1
done
if [[ "$created" == "1" ]]; then
  ok "create deployment via compose gateway"
else
  bad "create deployment failed"
  docker compose -f deploy/compose/docker-compose.yml logs --tail=80 api-gateway resource-service || true
fi

# generate a few requests for metrics
for i in $(seq 1 10); do
  curl -sS http://127.0.0.1:8080/api/v1/deployments >/dev/null || true
done

if curl -fsS http://127.0.0.1:9190/metrics | grep -q gopaas_http_requests_total; then
  ok "gateway metrics exposed"
else
  bad "gateway metrics missing"
fi

if curl -fsS 'http://127.0.0.1:9090/api/v1/query?query=up' | grep -q '"result"'; then
  ok "Prometheus API responding"
else
  bad "Prometheus not ready"
fi

# wait for scrape
sleep 15
if curl -fsS 'http://127.0.0.1:9090/api/v1/query?query=gopaas_http_requests_total' | grep -q 'gopaas_http_requests_total'; then
  ok "Prometheus scraped gopaas_http_requests_total"
else
  bad "Prometheus did not scrape gateway metrics yet"
fi

if curl -fsS -u admin:admin http://127.0.0.1:13000/api/health | grep -q 'ok\|database'; then
  ok "Grafana healthy (admin/admin)"
else
  # grafana health returns {"commit":"...","database":"ok","version":"..."}
  code=$(curl -s -o /tmp/grafana-health.json -w '%{http_code}' -u admin:admin http://127.0.0.1:13000/api/health || true)
  if [[ "$code" == "200" ]]; then
    ok "Grafana healthy (admin/admin)"
  else
    bad "Grafana not healthy"
    cat /tmp/grafana-health.json || true
  fi
fi

echo
echo "Passed: $PASS  Failed: $FAIL"
echo "  Gateway:    http://127.0.0.1:8080"
echo "  Prometheus: http://127.0.0.1:9090"
echo "  Grafana:    http://127.0.0.1:13000 (admin/admin)"
if [[ "$FAIL" -gt 0 ]]; then
  exit 1
fi

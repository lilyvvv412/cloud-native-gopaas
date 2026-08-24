#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"
export PATH="${HOME}/sdk/bin:${HOME}/sdk/go/bin:${HOME}/go/bin:/opt/homebrew/bin:/usr/local/bin:${PATH}"

CLUSTER_NAME="${CLUSTER_NAME:-gopaas}"
CONTEXT="kind-${CLUSTER_NAME}"

bash "${ROOT}/scripts/setup-kind.sh"
bash "${ROOT}/scripts/build-images.sh"

echo "loading images into kind (via image-archive to avoid attestation issues)..."
load_img() {
  local img="$1"
  docker save "$img" -o /tmp/kind-img.tar
  kind load image-archive /tmp/kind-img.tar --name "$CLUSTER_NAME"
  rm -f /tmp/kind-img.tar
}
for img in \
  gopaas/api-gateway:dev \
  gopaas/resource-service:dev \
  gopaas/scheduler-service:dev \
  gopaas/scheduledtask-controller:dev; do
  load_img "$img"
done
# optional runtime images used by demos
for img in busybox:1.36 nginx:1.25 \
  docker.m.daocloud.io/prom/prometheus:v2.52.0 \
  docker.m.daocloud.io/grafana/grafana:10.4.2 \
  docker.m.daocloud.io/hashicorp/consul:1.17; do
  if docker image inspect "$img" >/dev/null 2>&1; then
    load_img "$img" || true
  fi
done

kubectl --context "$CONTEXT" apply -f deploy/k8s/namespace.yaml
kubectl --context "$CONTEXT" apply -f deploy/k8s/rbac.yaml
kubectl --context "$CONTEXT" apply -f deploy/k8s/consul.yaml
kubectl --context "$CONTEXT" apply -f deploy/k8s/crd-scheduledtask.yaml
kubectl --context "$CONTEXT" wait --for=condition=Established crd/scheduledtasks.gopaas.io --timeout=60s
kubectl --context "$CONTEXT" apply -f deploy/k8s/services.yaml
kubectl --context "$CONTEXT" apply -f deploy/k8s/controller.yaml
kubectl --context "$CONTEXT" apply -f deploy/k8s/ingress.yaml
kubectl --context "$CONTEXT" apply -f deploy/k8s/monitoring.yaml

echo "waiting for workloads..."
kubectl --context "$CONTEXT" -n gopaas rollout status deploy/consul --timeout=180s
kubectl --context "$CONTEXT" -n gopaas rollout status deploy/resource-service --timeout=180s
kubectl --context "$CONTEXT" -n gopaas rollout status deploy/scheduler-service --timeout=180s
kubectl --context "$CONTEXT" -n gopaas rollout status deploy/api-gateway --timeout=180s
kubectl --context "$CONTEXT" -n gopaas rollout status deploy/scheduledtask-controller --timeout=180s

# sample CR only after controller is up
kubectl --context "$CONTEXT" apply -f deploy/k8s/sample-scheduledtask.yaml

echo "deploy complete"
kubectl --context "$CONTEXT" -n gopaas get pods,svc,ingress,hpa
kubectl --context "$CONTEXT" get scheduledtasks -A


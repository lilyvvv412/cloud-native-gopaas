#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
export PATH="${HOME}/sdk/bin:${HOME}/.linkerd2/bin:/opt/homebrew/bin:/usr/local/bin:${PATH}"

CLUSTER_NAME="${CLUSTER_NAME:-gopaas}"
CONTEXT="kind-${CLUSTER_NAME}"

if ! command -v linkerd >/dev/null 2>&1; then
  echo "linkerd CLI not found. Install with: brew install linkerd"
  exit 1
fi

echo "linkerd client: $(linkerd version --client 2>/dev/null || true)"
kubectl --context "$CONTEXT" cluster-info >/dev/null

GATEWAY_API_MANIFEST="${ROOT}/deploy/k8s/third_party/gateway-api-standard-install.yaml"
if [[ ! -f "$GATEWAY_API_MANIFEST" ]]; then
  GATEWAY_API_MANIFEST="https://github.com/kubernetes-sigs/gateway-api/releases/download/v1.2.1/standard-install.yaml"
fi
echo "installing Gateway API CRDs (required by modern Linkerd)..."
kubectl --context "$CONTEXT" apply --server-side -f "$GATEWAY_API_MANIFEST"

echo "installing Linkerd CRDs..."
linkerd install --crds | kubectl --context "$CONTEXT" apply -f -

echo "installing Linkerd control plane..."
linkerd install | kubectl --context "$CONTEXT" apply -f -

echo "waiting for linkerd control plane..."
kubectl --context "$CONTEXT" -n linkerd rollout status deploy/linkerd-destination --timeout=300s
kubectl --context "$CONTEXT" -n linkerd rollout status deploy/linkerd-identity --timeout=300s
kubectl --context "$CONTEXT" -n linkerd rollout status deploy/linkerd-proxy-injector --timeout=300s

echo "running linkerd check (control plane)..."
linkerd --context "$CONTEXT" check --proxy=false || true

echo "annotating gopaas namespace for sidecar injection..."
kubectl --context "$CONTEXT" annotate namespace gopaas linkerd.io/inject=enabled --overwrite

# Mesh the core data-plane services (skip consul/prometheus/grafana/controller for a cleaner demo)
for deploy in api-gateway resource-service scheduler-service; do
  kubectl --context "$CONTEXT" -n gopaas patch deploy/"$deploy" -p \
    '{"spec":{"template":{"metadata":{"annotations":{"linkerd.io/inject":"enabled"}}}}}'
done

echo "rolling meshed workloads..."
kubectl --context "$CONTEXT" -n gopaas rollout restart deploy/api-gateway deploy/resource-service deploy/scheduler-service
kubectl --context "$CONTEXT" -n gopaas rollout status deploy/api-gateway --timeout=240s
kubectl --context "$CONTEXT" -n gopaas rollout status deploy/resource-service --timeout=240s
kubectl --context "$CONTEXT" -n gopaas rollout status deploy/scheduler-service --timeout=240s

echo "applying mesh authorization policy..."
kubectl --context "$CONTEXT" apply -f "${ROOT}/deploy/k8s/linkerd/mesh-policy.yaml"

echo "Linkerd mesh ready"
kubectl --context "$CONTEXT" -n gopaas get pods

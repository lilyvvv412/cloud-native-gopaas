#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
export PATH="${HOME}/sdk/bin:${HOME}/sdk/go/bin:${HOME}/go/bin:/opt/homebrew/bin:/usr/local/bin:${PATH}"

CLUSTER_NAME="${CLUSTER_NAME:-gopaas}"
INGRESS_MANIFEST="${INGRESS_MANIFEST:-${ROOT}/deploy/k8s/third_party/ingress-nginx-kind.yaml}"
if [[ ! -f "$INGRESS_MANIFEST" ]]; then
  INGRESS_MANIFEST="https://raw.githubusercontent.com/kubernetes/ingress-nginx/controller-v1.11.3/deploy/static/provider/kind/deploy.yaml"
fi

if ! command -v kind >/dev/null 2>&1; then
  echo "kind not found; install into ${HOME}/sdk/bin first"
  exit 1
fi
if ! command -v kubectl >/dev/null 2>&1; then
  echo "kubectl not found"
  exit 1
fi

if kind get clusters 2>/dev/null | grep -qx "$CLUSTER_NAME"; then
  echo "kind cluster '${CLUSTER_NAME}' already exists"
else
  echo "creating kind cluster '${CLUSTER_NAME}'..."
  kind create cluster --config "${ROOT}/deploy/k8s/kind-config.yaml"
fi

kubectl cluster-info --context "kind-${CLUSTER_NAME}" >/dev/null
echo "installing nginx ingress controller..."
kubectl apply -f "$INGRESS_MANIFEST"

echo "waiting for ingress-nginx controller..."
kubectl -n ingress-nginx wait --for=condition=Available deploy/ingress-nginx-controller --timeout=180s
# Label selectors vary by ingress-nginx version; fall back to any Running controller pod.
if ! kubectl -n ingress-nginx wait --for=condition=ready pod \
  --selector=app.kubernetes.io/component=controller --timeout=120s 2>/dev/null; then
  echo "waiting for any ingress-nginx controller pod..."
  for i in $(seq 1 60); do
    ready=$(kubectl -n ingress-nginx get pods -o jsonpath='{range .items[*]}{.status.phase}{"\n"}{end}' \
      | grep -c Running || true)
    if [[ "${ready:-0}" -ge 1 ]]; then
      break
    fi
    sleep 2
  done
fi
kubectl -n ingress-nginx get pods

echo "kind + nginx ingress ready (host http://127.0.0.1:8088)"

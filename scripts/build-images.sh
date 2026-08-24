#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"
export PATH="${HOME}/sdk/bin:${HOME}/sdk/go/bin:${HOME}/go/bin:/opt/homebrew/bin:/usr/local/bin:${PATH}"
export GOPROXY="${GOPROXY:-https://goproxy.cn,direct}"
export GOSUMDB="${GOSUMDB:-sum.golang.google.cn}"
export GOTOOLCHAIN="${GOTOOLCHAIN:-local}"

ARCH="$(uname -m)"
case "$ARCH" in
  arm64|aarch64) GOARCH=arm64 ;;
  x86_64|amd64) GOARCH=amd64 ;;
  *) echo "unsupported arch: $ARCH"; exit 1 ;;
esac

mkdir -p bin/linux
echo "cross-compiling linux/${GOARCH} binaries..."
CGO_ENABLED=0 GOOS=linux GOARCH="$GOARCH" go build -o bin/linux/api-gateway ./services/api-gateway
CGO_ENABLED=0 GOOS=linux GOARCH="$GOARCH" go build -o bin/linux/resource-service ./services/resource-service
CGO_ENABLED=0 GOOS=linux GOARCH="$GOARCH" go build -o bin/linux/scheduler-service ./services/scheduler-service
CGO_ENABLED=0 GOOS=linux GOARCH="$GOARCH" go build -o bin/linux/scheduledtask-controller ./controllers/scheduledtask/cmd

echo "building docker images..."
# Disable attestations — kind load fails on provenance/SBOM manifest lists.
export BUILDX_NO_DEFAULT_ATTESTATIONS=1
build_img() {
  local tag="$1"
  local file="$2"
  docker build --provenance=false --sbom=false -t "$tag" -f "$file" .
}
build_img gopaas/api-gateway:dev deploy/docker/Dockerfile.gateway
build_img gopaas/resource-service:dev deploy/docker/Dockerfile.resource
build_img gopaas/scheduler-service:dev deploy/docker/Dockerfile.scheduler
build_img gopaas/scheduledtask-controller:dev deploy/docker/Dockerfile.controller

echo "images ready:"
docker images 'gopaas/*' --format '{{.Repository}}:{{.Tag}} {{.Size}}'

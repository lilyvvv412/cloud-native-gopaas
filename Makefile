export PATH := $(HOME)/sdk/bin:$(HOME)/sdk/go/bin:$(HOME)/go/bin:/opt/homebrew/bin:$(PATH)
GO ?= $(HOME)/sdk/go/bin/go
export GOPROXY ?= https://goproxy.cn,direct
export GOSUMDB ?= sum.golang.google.cn
export GOTOOLCHAIN ?= local

.PHONY: proto tidy build test demo compose-up compose-down images kind-up kind-demo

proto:
	bash hack/gen-proto.sh

tidy:
	$(GO) mod tidy

build:
	mkdir -p bin
	$(GO) build -o bin/api-gateway ./services/api-gateway
	$(GO) build -o bin/resource-service ./services/resource-service
	$(GO) build -o bin/scheduler-service ./services/scheduler-service
	$(GO) build -o bin/scheduledtask-controller ./controllers/scheduledtask/cmd

test:
	$(GO) test ./services/resource-service/... ./services/scheduler-service/... ./pkg/...

demo: build
	bash scripts/run-gopaas-demo.sh

compose-up:
	bash scripts/build-images.sh
	docker compose -f deploy/compose/docker-compose.yml up -d

compose-down:
	docker compose -f deploy/compose/docker-compose.yml down -v

compose-demo:
	bash scripts/run-compose-demo.sh

images:
	bash scripts/build-images.sh

kind-up:
	bash scripts/deploy-k8s.sh

kind-demo:
	bash scripts/run-k8s-demo.sh

mesh-up:
	bash scripts/setup-linkerd.sh

mesh-demo:
	bash scripts/run-mesh-demo.sh

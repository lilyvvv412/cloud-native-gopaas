package main

import (
	"log"

	resourcev1 "github.com/gopaas/platform/api/gen/resource/v1"
	"github.com/gopaas/platform/pkg/config"
	"github.com/gopaas/platform/pkg/k8sclient"
	"github.com/gopaas/platform/pkg/micromain"
	"github.com/gopaas/platform/services/resource-service/handler"
	"github.com/gopaas/platform/services/resource-service/internal/store"
)

func main() {
	cfg := config.Load("go.micro.service.resource", "8081")
	svc := micromain.NewService(cfg)

	var s store.Store
	switch cfg.Mode {
	case "k8s":
		cs, err := k8sclient.New(cfg.KubeConfigPath)
		if err != nil {
			log.Fatalf("k8s client: %v", err)
		}
		s = store.NewK8sStore(cs)
		log.Printf("resource-service mode=k8s")
	default:
		s = store.NewMemoryStore()
		log.Printf("resource-service mode=memory (set GOPAAS_MODE=k8s for cluster)")
	}

	if err := resourcev1.RegisterResourceServiceHandler(svc.Server(), &handler.ResourceHandler{Store: s}); err != nil {
		log.Fatalf("register handler: %v", err)
	}
	if err := svc.Run(); err != nil {
		log.Fatal(err)
	}
}

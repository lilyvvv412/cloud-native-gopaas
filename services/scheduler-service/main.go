package main

import (
	"log"

	schedulerv1 "github.com/gopaas/platform/api/gen/scheduler/v1"
	"github.com/gopaas/platform/pkg/config"
	"github.com/gopaas/platform/pkg/k8sclient"
	"github.com/gopaas/platform/pkg/micromain"
	"github.com/gopaas/platform/services/scheduler-service/handler"
	"github.com/gopaas/platform/services/scheduler-service/internal/store"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"
	"os"
	"path/filepath"
)

func main() {
	cfg := config.Load("go.micro.service.scheduler", "8083")
	svc := micromain.NewService(cfg)

	var s store.Store
	switch cfg.Mode {
	case "k8s":
		cs, err := k8sclient.New(cfg.KubeConfigPath)
		if err != nil {
			log.Fatalf("k8s client: %v", err)
		}
		dyn, err := newDynamic(cfg.KubeConfigPath)
		if err != nil {
			log.Fatalf("dynamic client: %v", err)
		}
		s = store.NewK8sStore(cs, dyn)
		log.Printf("scheduler-service mode=k8s")
	default:
		s = store.NewMemoryStore()
		log.Printf("scheduler-service mode=memory (set GOPAAS_MODE=k8s for cluster)")
	}

	if err := schedulerv1.RegisterSchedulerServiceHandler(svc.Server(), &handler.SchedulerHandler{Store: s}); err != nil {
		log.Fatalf("register handler: %v", err)
	}
	if err := svc.Run(); err != nil {
		log.Fatal(err)
	}
}

func newDynamic(kubeConfigPath string) (dynamic.Interface, error) {
	if cfg, err := rest.InClusterConfig(); err == nil {
		return dynamic.NewForConfig(cfg)
	}
	path := kubeConfigPath
	if path == "" {
		path = os.Getenv("KUBECONFIG")
	}
	if path == "" {
		if home := homedir.HomeDir(); home != "" {
			path = filepath.Join(home, ".kube", "config")
		}
	}
	cfg, err := clientcmd.BuildConfigFromFlags("", path)
	if err != nil {
		return nil, err
	}
	return dynamic.NewForConfig(cfg)
}

package main

import (
	"context"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/asim/go-micro/plugins/registry/consul/v3"
	"github.com/asim/go-micro/v3"
	"github.com/asim/go-micro/v3/registry"
	resourcev1 "github.com/gopaas/platform/api/gen/resource/v1"
	schedulerv1 "github.com/gopaas/platform/api/gen/scheduler/v1"
	"github.com/gopaas/platform/pkg/breaker"
	"github.com/gopaas/platform/pkg/config"
	"github.com/gopaas/platform/pkg/consulcfg"
	"github.com/gopaas/platform/pkg/metrics"
	"github.com/gopaas/platform/pkg/micromain"
	"golang.org/x/time/rate"
)

func main() {
	cfg := config.Load("go.micro.api.gateway", "8000")
	if v := os.Getenv("HTTP_PORT"); v != "" {
		cfg.HTTPPort = v
	} else {
		cfg.HTTPPort = "8080"
	}

	// Register gateway itself in Consul for service discovery demos.
	reg := consul.NewRegistry(func(o *registry.Options) {
		o.Addrs = cfg.ConsulAddrs
	})
	svc := micro.NewService(
		micro.Name(cfg.ServiceName),
		micro.Version("latest"),
		micro.Address(":"+cfg.ServicePort),
		micro.Registry(reg),
	)
	svc.Init()
	go func() {
		if err := svc.Run(); err != nil {
			log.Printf("gateway micro registration stopped: %v", err)
		}
	}()

	metrics.Boot(cfg.MetricsPort)
	breaker.Configure()
	breaker.BootStream(cfg.HystrixPort)

	client := micromain.NewClientOnly(cfg)
	resourceSvc := resourcev1.NewResourceService("go.micro.service.resource", client)
	schedulerSvc := schedulerv1.NewSchedulerService("go.micro.service.scheduler", client)

	defaults := consulcfg.KV{RateLimitQPS: cfg.RateLimitQPS, FeatureFlags: map[string]bool{"auth": false}}
	kv := consulcfg.LoadJSON(cfg.ConsulAddrs[0], "gopaas/config", defaults)
	limiter := rate.NewLimiter(rate.Limit(kv.RateLimitQPS), kv.RateLimitQPS)

	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "service": cfg.ServiceName})
	})
	mux.HandleFunc("/api/v1/deployments", func(w http.ResponseWriter, r *http.Request) {
		if !allow(limiter, w) {
			return
		}
		started := time.Now()
		status := http.StatusOK
		defer func() { metrics.ObserveHTTP(r.Method, "/api/v1/deployments", strconv.Itoa(status), started) }()

		switch r.Method {
		case http.MethodGet:
			ns := r.URL.Query().Get("namespace")
			rsp, err := resourceSvc.ListDeployments(r.Context(), &resourcev1.ListDeploymentsRequest{Namespace: ns})
			if err != nil {
				status = http.StatusBadGateway
				writeErr(w, status, err)
				return
			}
			writeJSON(w, status, rsp)
		case http.MethodPost:
			var req resourcev1.CreateDeploymentRequest
			if err := decodeJSON(r.Body, &req); err != nil {
				status = http.StatusBadRequest
				writeErr(w, status, err)
				return
			}
			rsp, err := resourceSvc.CreateDeployment(r.Context(), &req)
			if err != nil {
				status = http.StatusBadGateway
				writeErr(w, status, err)
				return
			}
			status = http.StatusCreated
			writeJSON(w, status, rsp)
		default:
			status = http.StatusMethodNotAllowed
			writeErr(w, status, errMethod())
		}
	})
	mux.HandleFunc("/api/v1/deployments/", func(w http.ResponseWriter, r *http.Request) {
		if !allow(limiter, w) {
			return
		}
		started := time.Now()
		status := http.StatusOK
		defer func() { metrics.ObserveHTTP(r.Method, "/api/v1/deployments/{name}", strconv.Itoa(status), started) }()

		path := strings.TrimPrefix(r.URL.Path, "/api/v1/deployments/")
		parts := strings.Split(strings.Trim(path, "/"), "/")
		if len(parts) == 0 || parts[0] == "" {
			status = http.StatusBadRequest
			writeJSON(w, status, map[string]string{"error": "missing deployment name"})
			return
		}
		name := parts[0]
		ns := r.URL.Query().Get("namespace")
		if ns == "" {
			ns = "default"
		}

		switch {
		case r.Method == http.MethodGet && len(parts) == 1:
			rsp, err := resourceSvc.GetDeployment(r.Context(), &resourcev1.GetDeploymentRequest{Namespace: ns, Name: name})
			if err != nil {
				status = http.StatusBadGateway
				writeErr(w, status, err)
				return
			}
			writeJSON(w, status, rsp)
		case r.Method == http.MethodPost && len(parts) == 2 && parts[1] == "scale":
			var body struct {
				Replicas int32 `json:"replicas"`
			}
			if err := decodeJSON(r.Body, &body); err != nil {
				status = http.StatusBadRequest
				writeErr(w, status, err)
				return
			}
			rsp, err := resourceSvc.ScaleDeployment(r.Context(), &resourcev1.ScaleDeploymentRequest{
				Namespace: ns, Name: name, Replicas: body.Replicas,
			})
			if err != nil {
				status = http.StatusBadGateway
				writeErr(w, status, err)
				return
			}
			writeJSON(w, status, rsp)
		case r.Method == http.MethodDelete && len(parts) == 1:
			rsp, err := resourceSvc.DeleteDeployment(r.Context(), &resourcev1.DeleteDeploymentRequest{Namespace: ns, Name: name})
			if err != nil {
				status = http.StatusBadGateway
				writeErr(w, status, err)
				return
			}
			writeJSON(w, status, rsp)
		default:
			status = http.StatusMethodNotAllowed
			writeErr(w, status, errMethod())
		}
	})
	mux.HandleFunc("/api/v1/tasks", func(w http.ResponseWriter, r *http.Request) {
		if !allow(limiter, w) {
			return
		}
		started := time.Now()
		status := http.StatusOK
		defer func() { metrics.ObserveHTTP(r.Method, "/api/v1/tasks", strconv.Itoa(status), started) }()

		switch r.Method {
		case http.MethodGet:
			ns := r.URL.Query().Get("namespace")
			rsp, err := schedulerSvc.ListTasks(r.Context(), &schedulerv1.ListTasksRequest{Namespace: ns})
			if err != nil {
				status = http.StatusBadGateway
				writeErr(w, status, err)
				return
			}
			writeJSON(w, status, rsp)
		case http.MethodPost:
			var req schedulerv1.SubmitTaskRequest
			if err := decodeJSON(r.Body, &req); err != nil {
				status = http.StatusBadRequest
				writeErr(w, status, err)
				return
			}
			rsp, err := schedulerSvc.SubmitTask(context.Background(), &req)
			if err != nil {
				status = http.StatusBadGateway
				writeErr(w, status, err)
				return
			}
			status = http.StatusCreated
			writeJSON(w, status, rsp)
		default:
			status = http.StatusMethodNotAllowed
			writeErr(w, status, errMethod())
		}
	})
	mux.HandleFunc("/api/v1/tasks/", func(w http.ResponseWriter, r *http.Request) {
		if !allow(limiter, w) {
			return
		}
		started := time.Now()
		status := http.StatusOK
		defer func() { metrics.ObserveHTTP(r.Method, "/api/v1/tasks/{name}", strconv.Itoa(status), started) }()

		name := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/v1/tasks/"), "/")
		ns := r.URL.Query().Get("namespace")
		if ns == "" {
			ns = "default"
		}
		switch r.Method {
		case http.MethodGet:
			rsp, err := schedulerSvc.GetTask(r.Context(), &schedulerv1.GetTaskRequest{Namespace: ns, Name: name})
			if err != nil {
				status = http.StatusBadGateway
				writeErr(w, status, err)
				return
			}
			writeJSON(w, status, rsp)
		case http.MethodDelete:
			rsp, err := schedulerSvc.CancelTask(r.Context(), &schedulerv1.CancelTaskRequest{Namespace: ns, Name: name})
			if err != nil {
				status = http.StatusBadGateway
				writeErr(w, status, err)
				return
			}
			writeJSON(w, status, rsp)
		default:
			status = http.StatusMethodNotAllowed
			writeErr(w, status, errMethod())
		}
	})

	addr := ":" + cfg.HTTPPort
	log.Printf("api-gateway HTTP listening on %s (go-micro name=%s)", addr, cfg.ServiceName)
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatal(err)
	}
}

func allow(l *rate.Limiter, w http.ResponseWriter) bool {
	if l.Allow() {
		return true
	}
	metrics.RateLimitedTotal.Inc()
	writeJSON(w, http.StatusTooManyRequests, map[string]string{"error": "rate limit exceeded"})
	return false
}

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, status int, err error) {
	writeJSON(w, status, map[string]string{"error": err.Error()})
}

func decodeJSON(r io.Reader, dst interface{}) error {
	dec := json.NewDecoder(r)
	dec.DisallowUnknownFields()
	return dec.Decode(dst)
}

func errMethod() error { return errString("method not allowed") }

type errString string

func (e errString) Error() string { return string(e) }

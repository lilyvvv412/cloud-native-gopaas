package config

import (
	"os"
	"strconv"
	"strings"
)

// Config holds runtime settings for GoPaaS services.
type Config struct {
	ServiceName    string
	ServiceHost    string
	ServicePort    string
	ConsulAddrs    []string
	MetricsPort    int
	HystrixPort    int
	RateLimitQPS   int
	KubeConfigPath string
	KubeNamespace  string
	Mode           string // "k8s" or "memory"
	HTTPPort       string // gateway only
}

func getenv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func getenvInt(key string, def int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}

// Load reads configuration from environment variables.
func Load(serviceName, defaultPort string) Config {
	consul := getenv("CONSUL_ADDRS", "127.0.0.1:8500")
	return Config{
		ServiceName:    serviceName,
		ServiceHost:    getenv("SERVICE_HOST", "127.0.0.1"),
		ServicePort:    getenv("SERVICE_PORT", defaultPort),
		ConsulAddrs:    strings.Split(consul, ","),
		MetricsPort:    getenvInt("METRICS_PORT", 9191),
		HystrixPort:    getenvInt("HYSTRIX_PORT", 9091),
		RateLimitQPS:   getenvInt("RATE_LIMIT_QPS", 100),
		KubeConfigPath: getenv("KUBECONFIG", ""),
		KubeNamespace:  getenv("KUBE_NAMESPACE", "default"),
		Mode:           getenv("GOPAAS_MODE", "memory"),
		HTTPPort:       getenv("HTTP_PORT", "8080"),
	}
}

func (c Config) Advertise() string {
	return c.ServiceHost + ":" + c.ServicePort
}

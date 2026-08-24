package micromain

import (
	"log"
	"strconv"

	grpcclient "github.com/asim/go-micro/plugins/client/grpc/v3"
	"github.com/asim/go-micro/plugins/registry/consul/v3"
	grpcserver "github.com/asim/go-micro/plugins/server/grpc/v3"
	ratelimit "github.com/asim/go-micro/plugins/wrapper/ratelimiter/uber/v3"
	"github.com/asim/go-micro/v3"
	"github.com/asim/go-micro/v3/client"
	"github.com/asim/go-micro/v3/registry"
	"github.com/asim/go-micro/v3/server"
	"github.com/gopaas/platform/pkg/breaker"
	"github.com/gopaas/platform/pkg/config"
	"github.com/gopaas/platform/pkg/metrics"
)

// NewService creates a go-micro v3 service with Consul registry and gRPC transport.
func NewService(cfg config.Config, opts ...micro.Option) micro.Service {
	reg := consul.NewRegistry(func(o *registry.Options) {
		o.Addrs = cfg.ConsulAddrs
	})

	breaker.Configure()
	metrics.Boot(cfg.MetricsPort)
	if cfg.HystrixPort > 0 {
		breaker.BootStream(cfg.HystrixPort)
	}

	base := []micro.Option{
		micro.Server(grpcserver.NewServer(func(o *server.Options) {
			o.Advertise = cfg.Advertise()
		})),
		micro.Client(grpcclient.NewClient()),
		micro.Name(cfg.ServiceName),
		micro.Version("latest"),
		micro.Address(":" + cfg.ServicePort),
		micro.Registry(reg),
		micro.WrapHandler(ratelimit.NewHandlerWrapper(cfg.RateLimitQPS)),
		micro.WrapClient(breaker.NewClientWrapper()),
	}
	base = append(base, opts...)
	svc := micro.NewService(base...)
	svc.Init()
	log.Printf("go-micro service %s advertise=%s consul=%v", cfg.ServiceName, cfg.Advertise(), cfg.ConsulAddrs)
	return svc
}

// NewClientOnly creates a go-micro client wired to Consul + gRPC + Hystrix.
func NewClientOnly(cfg config.Config) client.Client {
	reg := consul.NewRegistry(func(o *registry.Options) {
		o.Addrs = cfg.ConsulAddrs
	})
	breaker.Configure()
	return grpcclient.NewClient(
		client.Registry(reg),
		client.Wrap(breaker.NewClientWrapper()),
	)
}

// PortInt converts a port string to int.
func PortInt(port string, fallback int) int {
	n, err := strconv.Atoi(port)
	if err != nil {
		return fallback
	}
	return n
}

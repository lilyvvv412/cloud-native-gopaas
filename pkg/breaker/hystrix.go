package breaker

import (
	"context"
	"fmt"
	"log"
	"net"
	"net/http"
	"strconv"

	"github.com/afex/hystrix-go/hystrix"
	"github.com/asim/go-micro/v3/client"
	"github.com/gopaas/platform/pkg/metrics"
)

// Configure sets default Hystrix command settings for GoPaaS RPC calls.
func Configure() {
	hystrix.DefaultTimeout = 3000
	hystrix.DefaultMaxConcurrent = 100
	hystrix.DefaultVolumeThreshold = 10
	hystrix.DefaultSleepWindow = 5000
	hystrix.DefaultErrorPercentThreshold = 50
}

// BootStream exposes the Hystrix metrics stream (dashboard-compatible).
func BootStream(port int) {
	handler := hystrix.NewStreamHandler()
	handler.Start()
	go func() {
		addr := net.JoinHostPort("0.0.0.0", strconv.Itoa(port))
		log.Printf("hystrix stream listening on %s", addr)
		if err := http.ListenAndServe(addr, handler); err != nil {
			log.Printf("hystrix stream stopped: %v", err)
		}
	}()
}

type clientWrapper struct {
	client.Client
}

func (c *clientWrapper) Call(ctx context.Context, req client.Request, rsp interface{}, opts ...client.CallOption) error {
	name := req.Service() + "." + req.Endpoint()
	err := hystrix.DoC(ctx, name, func(ctx context.Context) error {
		callErr := c.Client.Call(ctx, req, rsp, opts...)
		if callErr != nil {
			metrics.RPCCallsTotal.WithLabelValues(req.Service(), req.Endpoint(), "error").Inc()
			return callErr
		}
		metrics.RPCCallsTotal.WithLabelValues(req.Service(), req.Endpoint(), "ok").Inc()
		return nil
	}, func(ctx context.Context, e error) error {
		metrics.CircuitOpenTotal.WithLabelValues(name).Inc()
		return fmt.Errorf("circuit open for %s: %w", name, e)
	})
	return err
}

// NewClientWrapper returns a go-micro client wrapper backed by Hystrix.
func NewClientWrapper() client.Wrapper {
	return func(c client.Client) client.Client {
		return &clientWrapper{Client: c}
	}
}

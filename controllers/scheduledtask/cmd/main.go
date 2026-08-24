package main

import (
	"flag"
	"os"

	gopaasv1 "github.com/gopaas/platform/controllers/scheduledtask/api/v1"
	"github.com/gopaas/platform/controllers/scheduledtask/controllers"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
)

var scheme = runtime.NewScheme()

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(gopaasv1.AddToScheme(scheme))
}

func main() {
	var metricsAddr string
	var probeAddr string
	flag.StringVar(&metricsAddr, "metrics-bind-address", ":8085", "metrics bind address")
	flag.StringVar(&probeAddr, "health-probe-bind-address", ":8086", "health probe bind address")
	opts := zap.Options{Development: true}
	opts.BindFlags(flag.CommandLine)
	flag.Parse()

	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme: scheme,
		Metrics: metricsserver.Options{
			BindAddress: metricsAddr,
		},
		HealthProbeBindAddress: probeAddr,
	})
	if err != nil {
		setupLog("unable to start manager", err)
	}

	if err := (&controllers.ScheduledTaskReconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
	}).SetupWithManager(mgr); err != nil {
		setupLog("unable to create controller", err)
	}

	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		setupLog("unable to set up health check", err)
	}
	if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		setupLog("unable to set up ready check", err)
	}

	setupLog("starting scheduledtask controller", nil)
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		setupLog("problem running manager", err)
	}
}

func setupLog(msg string, err error) {
	logger := ctrl.Log.WithName("setup")
	if err != nil {
		logger.Error(err, msg)
		os.Exit(1)
	}
	logger.Info(msg)
}

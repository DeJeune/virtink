package main

import (
	"context"
	"flag"
	"os"

	netv1 "github.com/k8snetworkplumbingwg/network-attachment-definition-client/pkg/apis/k8s.cni.cncf.io/v1"
	"github.com/spf13/pflag"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/kubernetes"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	cdiv1beta1 "kubevirt.io/containerized-data-importer-api/pkg/apis/core/v1beta1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	virtv1alpha1 "github.com/DeJeune/virtink/pkg/apis/virt/v1alpha1"
	"github.com/DeJeune/virtink/pkg/apiserver"
	apiserveroptions "github.com/DeJeune/virtink/pkg/apiserver/options"
	"github.com/DeJeune/virtink/pkg/controller"
	"github.com/DeJeune/virtink/pkg/controller/expectations"
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))

	utilruntime.Must(virtv1alpha1.AddToScheme(scheme))
	utilruntime.Must(cdiv1beta1.AddToScheme(scheme))
	utilruntime.Must(netv1.AddToScheme(scheme))
}

// +kubebuilder:rbac:groups="",resources=configmaps,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=coordination.k8s.io,resources=leases,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=events,verbs=create;update;patch

func main() {
	var metricsAddr string
	var enableLeaderElection bool
	var probeAddr string
	var enableAPIServer bool

	// Use pflag instead of flag
	pflag.StringVar(&metricsAddr, "metrics-bind-address", ":8080", "The address the metric endpoint binds to.")
	pflag.StringVar(&probeAddr, "health-probe-bind-address", ":8081", "The address the probe endpoint binds to.")
	pflag.BoolVar(&enableLeaderElection, "leader-elect", true,
		"Enable leader election for controller manager. "+
			"Enabling this will ensure there is only one active controller manager.")
	pflag.BoolVar(&enableAPIServer, "enable-apiserver", false,
		"Enable aggregated API server for subresources (console, etc.) - EXPERIMENTAL")

	// API server options
	serverOptions := apiserveroptions.NewServerOptions()
	serverOptions.AddFlags(pflag.CommandLine)

	opts := zap.Options{
		Development: true,
	}
	opts.BindFlags(flag.CommandLine)

	// Add pflag to flag for compatibility
	pflag.CommandLine.AddGoFlagSet(flag.CommandLine)
	pflag.Parse()

	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme:                 scheme,
		Metrics:                metricsserver.Options{BindAddress: metricsAddr},
		HealthProbeBindAddress: probeAddr,
		LeaderElection:         enableLeaderElection,
		LeaderElectionID:       "controller.virtink.smartx.com",
	})
	if err != nil {
		setupLog.Error(err, "unable to create manager")
		os.Exit(1)
	}

	if err = (&controller.VMReconciler{
		Client:             mgr.GetClient(),
		Scheme:             mgr.GetScheme(),
		Recorder:           mgr.GetEventRecorderFor("virt-controller"),
		PrerunnerImageName: os.Getenv("PRERUNNER_IMAGE"),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "VM")
		os.Exit(1)
	}

	if err := (&controller.VMMutator{}).SetupWebhookWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create webhook", "webhook", "VMMutator")
		os.Exit(1)
	}
	if err := (&controller.VMValidator{}).SetupWebhookWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create webhook", "webhook", "VMValidator")
		os.Exit(1)
	}

	if err = (&controller.VMMReconciler{
		Client:   mgr.GetClient(),
		Scheme:   mgr.GetScheme(),
		Recorder: mgr.GetEventRecorderFor("virt-controller"),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "VMM")
		os.Exit(1)
	}

	if err := (&controller.VMMValidator{}).SetupWebhookWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create webhook", "webhook", "VMMValidator")
		os.Exit(1)
	}

	if err := (&controller.VMReplicaSetReconciler{
		Client:       mgr.GetClient(),
		Scheme:       mgr.GetScheme(),
		Recorder:     mgr.GetEventRecorderFor("virt-controller"),
		Expectations: expectations.NewUIDTrackingControllerExpectations(expectations.NewControllerExpectations()),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "VMReplicaSet")
		os.Exit(1)
	}

	if err := (&controller.VMReplicaSetValidator{}).SetupWebhookWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create webhook", "webhook", "VMReplicaSetValidator")
		os.Exit(1)
	}

	if err := (&controller.VMReplicaSetMutator{}).SetupWebhookWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create webhook", "webhook", "VMReplicaSetMutator")
		os.Exit(1)
	}

	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up health check")
		os.Exit(1)
	}
	if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up ready check")
		os.Exit(1)
	}

	// Setup signal handler context
	ctx := ctrl.SetupSignalHandler()

	// Start aggregated API server if enabled
	if enableAPIServer {
		if err := startAPIServer(ctx, serverOptions, mgr); err != nil {
			setupLog.Error(err, "unable to start API server")
			os.Exit(1)
		}
	}

	setupLog.Info("starting manager")
	if err := mgr.Start(ctx); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}
}

func startAPIServer(ctx context.Context, serverOptions *apiserveroptions.ServerOptions, mgr ctrl.Manager) error {
	setupLog.Info("setting up aggregated API server")

	// Validate and complete server options
	if err := serverOptions.Validate(); err != nil {
		return err
	}
	if err := serverOptions.Complete(); err != nil {
		return err
	}

	// Create Kubernetes client
	kubeClient, err := kubernetes.NewForConfig(mgr.GetConfig())
	if err != nil {
		return err
	}

	// Create API server config
	serverConfig, err := apiserver.NewConfig(serverOptions, kubeClient, mgr.GetClient())
	if err != nil {
		return err
	}

	// Create and complete the server
	completedConfig := serverConfig.Complete()
	server, err := completedConfig.New()
	if err != nil {
		return err
	}

	// Start API server in a goroutine
	go func() {
		setupLog.Info("starting aggregated API server", "port", serverOptions.SecureServing.BindPort)
		if err := server.GenericAPIServer.PrepareRun().Run(ctx.Done()); err != nil {
			setupLog.Error(err, "error running aggregated API server")
			os.Exit(1)
		}
	}()

	return nil
}

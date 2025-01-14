package controller

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"go.uber.org/zap/zapcore"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	virtv1alpha1 "github.com/smartxworks/virtink/pkg/apis/virt/v1alpha1"
	"github.com/smartxworks/virtink/pkg/controller/expectations"
)

var (
	cfg       *rest.Config
	k8sClient client.Client
	testEnv   *envtest.Environment
	ctx       context.Context
	cancel    context.CancelFunc
)

func TestAPIs(t *testing.T) {
	RegisterFailHandler(Fail)

	RunSpecs(t, "Controller Suite")
}

var _ = BeforeSuite(func() {
	opts := &zap.Options{
		Development: true,                       // 根据需要设置为 true 或 false
		TimeEncoder: zapcore.ISO8601TimeEncoder, // 使用 ISO8601 时间编码
	}
	// 创建日志记录器
	logf.SetLogger(zap.New(
		zap.WriteTo(GinkgoWriter),
		zap.UseDevMode(true),
		zap.UseFlagOptions(opts),
	))

	ctx, cancel = context.WithCancel(context.TODO())

	By("bootstrapping test environment")
	testEnv = &envtest.Environment{
		CRDDirectoryPaths:     []string{filepath.Join("..", "..", "deploy", "crd")},
		ErrorIfCRDPathMissing: true,
		ControlPlane: envtest.ControlPlane{
			APIServer: &envtest.APIServer{
				StartTimeout: time.Minute,
			},
		},
	}

	var err error
	cfg, err = testEnv.Start()
	cfg.WarningHandler = rest.NoWarnings{}
	Expect(err).NotTo(HaveOccurred())
	Expect(cfg).NotTo(BeNil())

	err = virtv1alpha1.AddToScheme(scheme.Scheme)
	Expect(err).NotTo(HaveOccurred())

	k8sClient, err = client.New(cfg, client.Options{Scheme: scheme.Scheme})
	Expect(err).NotTo(HaveOccurred())
	Expect(k8sClient).NotTo(BeNil())

	k8sManager, err := ctrl.NewManager(cfg, ctrl.Options{
		Scheme:           scheme.Scheme,
		LeaderElection:   false,
		LeaderElectionID: "virtink-test-leader-election",
	})
	Expect(err).ToNot(HaveOccurred())

	err = (&VMReconciler{
		Client:             k8sManager.GetClient(),
		Scheme:             k8sManager.GetScheme(),
		Recorder:           k8sManager.GetEventRecorderFor("virt-controller"),
		PrerunnerImageName: "prerunner",
	}).SetupWithManager(k8sManager)
	Expect(err).ToNot(HaveOccurred())

	err = (&VMReplicaSetReconciler{
		Client:       k8sManager.GetClient(),
		Scheme:       k8sManager.GetScheme(),
		Recorder:     k8sManager.GetEventRecorderFor("vmr-controller"),
		Expectations: expectations.NewUIDTrackingControllerExpectations(expectations.NewControllerExpectations()),
	}).SetupWithManager(k8sManager)
	Expect(err).ToNot(HaveOccurred())

	go func() {
		defer GinkgoRecover()
		err = k8sManager.Start(ctx)
		Expect(err).ToNot(HaveOccurred(), "failed to run manager")
	}()

}, 60)

var _ = AfterSuite(func() {
	cancel()
	By("tearing down the test environment")
	err := testEnv.Stop()
	Expect(err).NotTo(HaveOccurred())
})

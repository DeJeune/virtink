package options

import (
	"net"

	"github.com/spf13/pflag"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	genericapiserver "k8s.io/apiserver/pkg/server"
	genericoptions "k8s.io/apiserver/pkg/server/options"

	subresourcesv1alpha1 "github.com/DeJeune/virtink/pkg/apis/subresources/v1alpha1"
)

// ServerOptions contains configuration for the aggregated API server
// Following metrics-server pattern with individual options instead of RecommendedOptions
type ServerOptions struct {
	SecureServing *genericoptions.SecureServingOptionsWithLoopback
}

// NewServerOptions creates a new ServerOptions with default values
func NewServerOptions() *ServerOptions {
	secureServing := genericoptions.NewSecureServingOptions().WithLoopback()

	// Default settings
	secureServing.BindPort = 8443
	secureServing.ServerCert.CertDirectory = "/var/run/virtink/serving-cert"
	secureServing.ServerCert.PairName = "apiserver"

	return &ServerOptions{
		SecureServing: secureServing,
	}
}

// AddFlags adds flags to the flagset
func (o *ServerOptions) AddFlags(fs *pflag.FlagSet) {
	o.SecureServing.AddFlags(fs)
}

// Validate validates the options
func (o *ServerOptions) Validate() error {
	return nil
}

// Complete fills in missing options
func (o *ServerOptions) Complete() error {
	return nil
}

// Config returns a server configuration
func (o *ServerOptions) Config() (*Config, error) {
	if err := o.SecureServing.MaybeDefaultWithSelfSignedCerts(
		"localhost",
		nil,
		[]net.IP{net.ParseIP("127.0.0.1")},
	); err != nil {
		return nil, err
	}

	// Use NewConfig directly (like metrics-server) instead of NewRecommendedConfig
	serverConfig := genericapiserver.NewConfig(Codecs)

	// Set ExternalAddress with port to avoid nil pointer in Complete()
	serverConfig.ExternalAddress = "localhost:8443"

	// Apply SecureServing
	if err := o.SecureServing.ApplyTo(&serverConfig.SecureServing, &serverConfig.LoopbackClientConfig); err != nil {
		return nil, err
	}

	config := &Config{
		GenericConfig: serverConfig,
		ExtraConfig:   ExtraConfig{},
	}

	return config, nil
}

var (
	// Scheme is the runtime scheme for the aggregated API server
	scheme = runtime.NewScheme()
	// Codecs provides access to encoding and decoding for the scheme
	Codecs = serializer.NewCodecFactory(scheme)
)

func init() {
	_ = subresourcesv1alpha1.AddToScheme(scheme)

	// Register the group version
	scheme.AddUnversionedTypes(schema.GroupVersion{Group: "", Version: "v1"},
		&subresourcesv1alpha1.VirtualMachineConsoleOptions{},
	)
}

// Config contains configuration for the aggregated API server
type Config struct {
	GenericConfig *genericapiserver.Config
	ExtraConfig   ExtraConfig
}

// ExtraConfig holds custom configuration
type ExtraConfig struct {
	// Add any extra configuration fields here if needed
}

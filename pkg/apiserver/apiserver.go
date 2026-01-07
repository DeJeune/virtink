package apiserver

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	genericapiserver "k8s.io/apiserver/pkg/server"
	"k8s.io/apiserver/pkg/registry/rest"
	"k8s.io/client-go/kubernetes"
	"sigs.k8s.io/controller-runtime/pkg/client"

	subresourcesv1alpha1 "github.com/DeJeune/virtink/pkg/apis/subresources/v1alpha1"
	"github.com/DeJeune/virtink/pkg/apiserver/options"
	"github.com/DeJeune/virtink/pkg/apiserver/registry/subresources/console"
)

var (
	// Scheme defines methods for serializing and deserializing API objects
	Scheme = runtime.NewScheme()
	// Codecs provides methods for retrieving codecs and serializers
	Codecs = serializer.NewCodecFactory(Scheme)
)

func init() {
	_ = subresourcesv1alpha1.AddToScheme(Scheme)

	// Register the types with the Scheme
	metav1.AddToGroupVersion(Scheme, schema.GroupVersion{Version: "v1"})

	// Register unversioned types with v1
	Scheme.AddUnversionedTypes(schema.GroupVersion{Version: "v1"},
		&metav1.Status{},
		&metav1.APIVersions{},
		&metav1.APIGroupList{},
		&metav1.APIGroup{},
		&metav1.APIResourceList{},
	)
}

// Config contains configuration for the aggregated API server
type Config struct {
	GenericConfig *genericapiserver.Config
	ExtraConfig   ExtraConfig
}

// ExtraConfig holds custom server configuration
type ExtraConfig struct {
	KubeClient kubernetes.Interface
	VirtClient client.Client
}

// SubresourcesAPIServer contains state for the aggregated API server
type SubresourcesAPIServer struct {
	GenericAPIServer *genericapiserver.GenericAPIServer
}

// Complete fills in any fields not set that are required to have valid data
func (cfg *Config) Complete() CompletedConfig {
	c := completedConfig{
		// Pass nil for SharedInformerFactory since we don't need it for stateless API server
		cfg.GenericConfig.Complete(nil),
		&cfg.ExtraConfig,
	}

	return CompletedConfig{&c}
}

// CompletedConfig is the completed configuration for the API server
type CompletedConfig struct {
	*completedConfig
}

type completedConfig struct {
	GenericConfig genericapiserver.CompletedConfig
	ExtraConfig   *ExtraConfig
}

// New returns a new instance of SubresourcesAPIServer from the given config
func (c CompletedConfig) New() (*SubresourcesAPIServer, error) {
	genericServer, err := c.GenericConfig.New("subresources-apiserver", genericapiserver.NewEmptyDelegate())
	if err != nil {
		return nil, err
	}

	s := &SubresourcesAPIServer{
		GenericAPIServer: genericServer,
	}

	// Install API groups
	if err := s.installAPIGroups(c.ExtraConfig); err != nil {
		return nil, err
	}

	return s, nil
}

// installAPIGroups installs the API groups into the server
func (s *SubresourcesAPIServer) installAPIGroups(extraConfig *ExtraConfig) error {
	apiGroupInfo := genericapiserver.NewDefaultAPIGroupInfo(
		subresourcesv1alpha1.GroupName,
		Scheme,
		metav1.ParameterCodec,
		Codecs,
	)

	// Create REST storage for console subresource
	consoleStorage := console.NewConsoleREST(extraConfig.KubeClient, extraConfig.VirtClient)

	// Register the storage
	v1alpha1Storage := map[string]rest.Storage{}
	v1alpha1Storage["virtualmachines/console"] = consoleStorage

	apiGroupInfo.VersionedResourcesStorageMap["v1alpha1"] = v1alpha1Storage

	if err := s.GenericAPIServer.InstallAPIGroup(&apiGroupInfo); err != nil {
		return err
	}

	return nil
}

// NewConfig creates a new Config from options
func NewConfig(opts *options.ServerOptions, kubeClient kubernetes.Interface, virtClient client.Client) (*Config, error) {
	optsCfg, err := opts.Config()
	if err != nil {
		return nil, err
	}

	cfg := &Config{
		GenericConfig: optsCfg.GenericConfig,
		ExtraConfig: ExtraConfig{
			KubeClient: kubeClient,
			VirtClient: virtClient,
		},
	}

	return cfg, nil
}

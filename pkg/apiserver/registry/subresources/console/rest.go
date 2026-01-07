package console

import (
	"context"
	"fmt"
	"net/http"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apiserver/pkg/registry/rest"
	"k8s.io/client-go/kubernetes"
	"sigs.k8s.io/controller-runtime/pkg/client"

	virtv1alpha1 "github.com/DeJeune/virtink/pkg/apis/virt/v1alpha1"
	subresourcesv1alpha1 "github.com/DeJeune/virtink/pkg/apis/subresources/v1alpha1"
)

// ConsoleREST implements the REST endpoint for console connections
type ConsoleREST struct {
	kubeClient kubernetes.Interface
	virtClient client.Client
}

var _ rest.Storage = &ConsoleREST{}
var _ rest.Connecter = &ConsoleREST{}

// NewConsoleREST creates a new ConsoleREST storage implementation
func NewConsoleREST(kubeClient kubernetes.Interface, virtClient client.Client) *ConsoleREST {
	return &ConsoleREST{
		kubeClient: kubeClient,
		virtClient: virtClient,
	}
}

// New implements rest.Storage
func (r *ConsoleREST) New() runtime.Object {
	return &subresourcesv1alpha1.VirtualMachineConsoleOptions{}
}

// Destroy implements rest.Storage
func (r *ConsoleREST) Destroy() {
	// No cleanup needed
}

// Connect implements rest.Connecter
func (r *ConsoleREST) Connect(ctx context.Context, name string, opts runtime.Object, responder rest.Responder) (http.Handler, error) {
	// Type assert the options
	consoleOpts, ok := opts.(*subresourcesv1alpha1.VirtualMachineConsoleOptions)
	if !ok {
		return nil, fmt.Errorf("invalid options object: %T", opts)
	}

	// Extract namespace from context
	namespace, ok := ctx.Value("namespace").(string)
	if !ok {
		return nil, fmt.Errorf("namespace not found in context")
	}

	// Get the VirtualMachine object
	vm := &virtv1alpha1.VirtualMachine{}
	vmKey := client.ObjectKey{Namespace: namespace, Name: name}
	if err := r.virtClient.Get(ctx, vmKey, vm); err != nil {
		return nil, fmt.Errorf("failed to get VirtualMachine: %w", err)
	}

	// Validate VM state
	if vm.Status.Phase != virtv1alpha1.VirtualMachineRunning {
		return nil, fmt.Errorf("VM is not in Running state (current: %s)", vm.Status.Phase)
	}

	// Ensure VM is scheduled to a node
	if vm.Status.NodeName == "" {
		return nil, fmt.Errorf("VM has not been scheduled to a node")
	}

	// Create and return the proxy handler
	handler := NewProxyHandler(r.kubeClient, vm, consoleOpts)
	return handler, nil
}

// NewConnectOptions implements rest.Connecter
func (r *ConsoleREST) NewConnectOptions() (runtime.Object, bool, string) {
	return &subresourcesv1alpha1.VirtualMachineConsoleOptions{}, false, ""
}

// ConnectMethods implements rest.Connecter
func (r *ConsoleREST) ConnectMethods() []string {
	return []string{"GET"}
}

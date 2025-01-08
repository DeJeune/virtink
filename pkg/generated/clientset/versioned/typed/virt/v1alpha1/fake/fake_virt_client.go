// Code generated by client-gen. DO NOT EDIT.

package fake

import (
	v1alpha1 "github.com/smartxworks/virtink/pkg/generated/clientset/versioned/typed/virt/v1alpha1"
	rest "k8s.io/client-go/rest"
	testing "k8s.io/client-go/testing"
)

type FakeVirtV1alpha1 struct {
	*testing.Fake
}

func (c *FakeVirtV1alpha1) VirtualMachines(namespace string) v1alpha1.VirtualMachineInterface {
	return &FakeVirtualMachines{c, namespace}
}

func (c *FakeVirtV1alpha1) VirtualMachineMigrations(namespace string) v1alpha1.VirtualMachineMigrationInterface {
	return &FakeVirtualMachineMigrations{c, namespace}
}

func (c *FakeVirtV1alpha1) VirtualMachineReplicaSets(namespace string) v1alpha1.VirtualMachineReplicaSetInterface {
	return &FakeVirtualMachineReplicaSets{c, namespace}
}

// RESTClient returns a RESTClient that is used to communicate
// with API server by this client implementation.
func (c *FakeVirtV1alpha1) RESTClient() rest.Interface {
	var ret *rest.RESTClient
	return ret
}

// Code generated by lister-gen. DO NOT EDIT.

package v1alpha1

import (
	v1alpha1 "github.com/DeJeune/virtink/pkg/apis/virt/v1alpha1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/tools/cache"
)

// VirtualMachineReplicaSetLister helps list VirtualMachineReplicaSets.
// All objects returned here must be treated as read-only.
type VirtualMachineReplicaSetLister interface {
	// List lists all VirtualMachineReplicaSets in the indexer.
	// Objects returned here must be treated as read-only.
	List(selector labels.Selector) (ret []*v1alpha1.VirtualMachineReplicaSet, err error)
	// VirtualMachineReplicaSets returns an object that can list and get VirtualMachineReplicaSets.
	VirtualMachineReplicaSets(namespace string) VirtualMachineReplicaSetNamespaceLister
	VirtualMachineReplicaSetListerExpansion
}

// virtualMachineReplicaSetLister implements the VirtualMachineReplicaSetLister interface.
type virtualMachineReplicaSetLister struct {
	indexer cache.Indexer
}

// NewVirtualMachineReplicaSetLister returns a new VirtualMachineReplicaSetLister.
func NewVirtualMachineReplicaSetLister(indexer cache.Indexer) VirtualMachineReplicaSetLister {
	return &virtualMachineReplicaSetLister{indexer: indexer}
}

// List lists all VirtualMachineReplicaSets in the indexer.
func (s *virtualMachineReplicaSetLister) List(selector labels.Selector) (ret []*v1alpha1.VirtualMachineReplicaSet, err error) {
	err = cache.ListAll(s.indexer, selector, func(m interface{}) {
		ret = append(ret, m.(*v1alpha1.VirtualMachineReplicaSet))
	})
	return ret, err
}

// VirtualMachineReplicaSets returns an object that can list and get VirtualMachineReplicaSets.
func (s *virtualMachineReplicaSetLister) VirtualMachineReplicaSets(namespace string) VirtualMachineReplicaSetNamespaceLister {
	return virtualMachineReplicaSetNamespaceLister{indexer: s.indexer, namespace: namespace}
}

// VirtualMachineReplicaSetNamespaceLister helps list and get VirtualMachineReplicaSets.
// All objects returned here must be treated as read-only.
type VirtualMachineReplicaSetNamespaceLister interface {
	// List lists all VirtualMachineReplicaSets in the indexer for a given namespace.
	// Objects returned here must be treated as read-only.
	List(selector labels.Selector) (ret []*v1alpha1.VirtualMachineReplicaSet, err error)
	// Get retrieves the VirtualMachineReplicaSet from the indexer for a given namespace and name.
	// Objects returned here must be treated as read-only.
	Get(name string) (*v1alpha1.VirtualMachineReplicaSet, error)
	VirtualMachineReplicaSetNamespaceListerExpansion
}

// virtualMachineReplicaSetNamespaceLister implements the VirtualMachineReplicaSetNamespaceLister
// interface.
type virtualMachineReplicaSetNamespaceLister struct {
	indexer   cache.Indexer
	namespace string
}

// List lists all VirtualMachineReplicaSets in the indexer for a given namespace.
func (s virtualMachineReplicaSetNamespaceLister) List(selector labels.Selector) (ret []*v1alpha1.VirtualMachineReplicaSet, err error) {
	err = cache.ListAllByNamespace(s.indexer, s.namespace, selector, func(m interface{}) {
		ret = append(ret, m.(*v1alpha1.VirtualMachineReplicaSet))
	})
	return ret, err
}

// Get retrieves the VirtualMachineReplicaSet from the indexer for a given namespace and name.
func (s virtualMachineReplicaSetNamespaceLister) Get(name string) (*v1alpha1.VirtualMachineReplicaSet, error) {
	obj, exists, err := s.indexer.GetByKey(s.namespace + "/" + name)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, errors.NewNotFound(v1alpha1.Resource("virtualmachinereplicaset"), name)
	}
	return obj.(*v1alpha1.VirtualMachineReplicaSet), nil
}

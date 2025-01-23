// Code generated by client-gen. DO NOT EDIT.

package fake

import (
	"context"

	v1alpha1 "github.com/DeJeune/virtink/pkg/apis/virt/v1alpha1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	labels "k8s.io/apimachinery/pkg/labels"
	schema "k8s.io/apimachinery/pkg/runtime/schema"
	types "k8s.io/apimachinery/pkg/types"
	watch "k8s.io/apimachinery/pkg/watch"
	testing "k8s.io/client-go/testing"
)

// FakeVirtualMachineReplicaSets implements VirtualMachineReplicaSetInterface
type FakeVirtualMachineReplicaSets struct {
	Fake *FakeVirtV1alpha1
	ns   string
}

var virtualmachinereplicasetsResource = schema.GroupVersionResource{Group: "virt.virtink.smartx.com", Version: "v1alpha1", Resource: "virtualmachinereplicasets"}

var virtualmachinereplicasetsKind = schema.GroupVersionKind{Group: "virt.virtink.smartx.com", Version: "v1alpha1", Kind: "VirtualMachineReplicaSet"}

// Get takes name of the virtualMachineReplicaSet, and returns the corresponding virtualMachineReplicaSet object, and an error if there is any.
func (c *FakeVirtualMachineReplicaSets) Get(ctx context.Context, name string, options v1.GetOptions) (result *v1alpha1.VirtualMachineReplicaSet, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewGetAction(virtualmachinereplicasetsResource, c.ns, name), &v1alpha1.VirtualMachineReplicaSet{})

	if obj == nil {
		return nil, err
	}
	return obj.(*v1alpha1.VirtualMachineReplicaSet), err
}

// List takes label and field selectors, and returns the list of VirtualMachineReplicaSets that match those selectors.
func (c *FakeVirtualMachineReplicaSets) List(ctx context.Context, opts v1.ListOptions) (result *v1alpha1.VirtualMachineReplicaSetList, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewListAction(virtualmachinereplicasetsResource, virtualmachinereplicasetsKind, c.ns, opts), &v1alpha1.VirtualMachineReplicaSetList{})

	if obj == nil {
		return nil, err
	}

	label, _, _ := testing.ExtractFromListOptions(opts)
	if label == nil {
		label = labels.Everything()
	}
	list := &v1alpha1.VirtualMachineReplicaSetList{ListMeta: obj.(*v1alpha1.VirtualMachineReplicaSetList).ListMeta}
	for _, item := range obj.(*v1alpha1.VirtualMachineReplicaSetList).Items {
		if label.Matches(labels.Set(item.Labels)) {
			list.Items = append(list.Items, item)
		}
	}
	return list, err
}

// Watch returns a watch.Interface that watches the requested virtualMachineReplicaSets.
func (c *FakeVirtualMachineReplicaSets) Watch(ctx context.Context, opts v1.ListOptions) (watch.Interface, error) {
	return c.Fake.
		InvokesWatch(testing.NewWatchAction(virtualmachinereplicasetsResource, c.ns, opts))

}

// Create takes the representation of a virtualMachineReplicaSet and creates it.  Returns the server's representation of the virtualMachineReplicaSet, and an error, if there is any.
func (c *FakeVirtualMachineReplicaSets) Create(ctx context.Context, virtualMachineReplicaSet *v1alpha1.VirtualMachineReplicaSet, opts v1.CreateOptions) (result *v1alpha1.VirtualMachineReplicaSet, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewCreateAction(virtualmachinereplicasetsResource, c.ns, virtualMachineReplicaSet), &v1alpha1.VirtualMachineReplicaSet{})

	if obj == nil {
		return nil, err
	}
	return obj.(*v1alpha1.VirtualMachineReplicaSet), err
}

// Update takes the representation of a virtualMachineReplicaSet and updates it. Returns the server's representation of the virtualMachineReplicaSet, and an error, if there is any.
func (c *FakeVirtualMachineReplicaSets) Update(ctx context.Context, virtualMachineReplicaSet *v1alpha1.VirtualMachineReplicaSet, opts v1.UpdateOptions) (result *v1alpha1.VirtualMachineReplicaSet, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewUpdateAction(virtualmachinereplicasetsResource, c.ns, virtualMachineReplicaSet), &v1alpha1.VirtualMachineReplicaSet{})

	if obj == nil {
		return nil, err
	}
	return obj.(*v1alpha1.VirtualMachineReplicaSet), err
}

// UpdateStatus was generated because the type contains a Status member.
// Add a +genclient:noStatus comment above the type to avoid generating UpdateStatus().
func (c *FakeVirtualMachineReplicaSets) UpdateStatus(ctx context.Context, virtualMachineReplicaSet *v1alpha1.VirtualMachineReplicaSet, opts v1.UpdateOptions) (*v1alpha1.VirtualMachineReplicaSet, error) {
	obj, err := c.Fake.
		Invokes(testing.NewUpdateSubresourceAction(virtualmachinereplicasetsResource, "status", c.ns, virtualMachineReplicaSet), &v1alpha1.VirtualMachineReplicaSet{})

	if obj == nil {
		return nil, err
	}
	return obj.(*v1alpha1.VirtualMachineReplicaSet), err
}

// Delete takes name of the virtualMachineReplicaSet and deletes it. Returns an error if one occurs.
func (c *FakeVirtualMachineReplicaSets) Delete(ctx context.Context, name string, opts v1.DeleteOptions) error {
	_, err := c.Fake.
		Invokes(testing.NewDeleteActionWithOptions(virtualmachinereplicasetsResource, c.ns, name, opts), &v1alpha1.VirtualMachineReplicaSet{})

	return err
}

// DeleteCollection deletes a collection of objects.
func (c *FakeVirtualMachineReplicaSets) DeleteCollection(ctx context.Context, opts v1.DeleteOptions, listOpts v1.ListOptions) error {
	action := testing.NewDeleteCollectionAction(virtualmachinereplicasetsResource, c.ns, listOpts)

	_, err := c.Fake.Invokes(action, &v1alpha1.VirtualMachineReplicaSetList{})
	return err
}

// Patch applies the patch and returns the patched virtualMachineReplicaSet.
func (c *FakeVirtualMachineReplicaSets) Patch(ctx context.Context, name string, pt types.PatchType, data []byte, opts v1.PatchOptions, subresources ...string) (result *v1alpha1.VirtualMachineReplicaSet, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewPatchSubresourceAction(virtualmachinereplicasetsResource, c.ns, name, pt, data, subresources...), &v1alpha1.VirtualMachineReplicaSet{})

	if obj == nil {
		return nil, err
	}
	return obj.(*v1alpha1.VirtualMachineReplicaSet), err
}

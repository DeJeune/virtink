package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// GroupName is the group name for subresources
const GroupName = "subresources.virt.virtink.smartx.com"

// SchemeGroupVersion is the group version for subresources
var SchemeGroupVersion = schema.GroupVersion{Group: GroupName, Version: "v1alpha1"}

// GroupVersion is kept for backward compatibility
var GroupVersion = SchemeGroupVersion

// Resource takes an unqualified resource and returns a Group qualified GroupResource
func Resource(resource string) schema.GroupResource {
	return SchemeGroupVersion.WithResource(resource).GroupResource()
}

var (
	// SchemeBuilder is the scheme builder for this API group
	SchemeBuilder = runtime.NewSchemeBuilder(addKnownTypes)
	// AddToScheme adds the types in this group-version to the given scheme
	AddToScheme = SchemeBuilder.AddToScheme
)

// Adds the list of known types to the given scheme
func addKnownTypes(scheme *runtime.Scheme) error {
	scheme.AddKnownTypes(SchemeGroupVersion,
		&VirtualMachineConsoleOptions{},
	)
	metav1.AddToGroupVersion(scheme, SchemeGroupVersion)
	return nil
}

package controller

import (
	"k8s.io/apimachinery/pkg/labels"

	virtv1alpha1 "github.com/DeJeune/virtink/pkg/apis/virt/v1alpha1"
)

func deepcopyVMsLabelSet(template *virtv1alpha1.VirtualMachineTemplateSpec) labels.Set {
	desiredLabels := make(labels.Set)
	for k, v := range template.ObjectMeta.Labels {
		desiredLabels[k] = v
	}
	return desiredLabels
}

func deepcopyVMsAnnotationSet(template *virtv1alpha1.VirtualMachineTemplateSpec) labels.Set {
	desiredAnnotations := make(labels.Set)
	for k, v := range template.ObjectMeta.Annotations {
		desiredAnnotations[k] = v
	}
	return desiredAnnotations
}

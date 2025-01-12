package controller

import (
	"k8s.io/apimachinery/pkg/labels"

	virtv1alpha1 "github.com/smartxworks/virtink/pkg/apis/virt/v1alpha1"
)

func deepcopyVMsLabelSet(template *virtv1alpha1.VirtualMachineTemplateSpec) labels.Set {
	desiredLabels := make(labels.Set)
	for k, v := range template.ObjectMeta.Labels {
		desiredLabels[k] = v
	}
	return desiredLabels
}

func deepcopyVMsFinalizers(template *virtv1alpha1.VirtualMachineTemplateSpec) []string {
	desiredFinalizers := make([]string, len(template.ObjectMeta.Finalizers))
	copy(desiredFinalizers, template.ObjectMeta.Finalizers)
	return desiredFinalizers
}

func deepcopyVMsAnnotationSet(template *virtv1alpha1.VirtualMachineTemplateSpec) labels.Set {
	desiredAnnotations := make(labels.Set)
	for k, v := range template.ObjectMeta.Annotations {
		desiredAnnotations[k] = v
	}
	return desiredAnnotations
}

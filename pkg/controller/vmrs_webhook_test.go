package controller

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	virtv1alpha1 "github.com/DeJeune/virtink/pkg/apis/virt/v1alpha1"
)

func TestValidateVMReplicaSet(t *testing.T) {
	validVMR := &virtv1alpha1.VirtualMachineReplicaSet{
		Spec: virtv1alpha1.VirtualMachineReplicaSetSpec{
			Replicas: int32Ptr(1),
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app": "test",
				},
			},
			Template: &virtv1alpha1.VirtualMachineTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app": "test",
					},
				},
				Spec: virtv1alpha1.VirtualMachineSpec{
					Instance: virtv1alpha1.Instance{
						CPU: virtv1alpha1.CPU{
							Sockets:        1,
							CoresPerSocket: 1,
						},
						Memory: virtv1alpha1.Memory{
							Size: resource.MustParse("1Gi"),
						},
					},
				},
			},
		},
	}

	tests := []struct {
		name          string
		vmrs          *virtv1alpha1.VirtualMachineReplicaSet
		oldVMR        *virtv1alpha1.VirtualMachineReplicaSet
		invalidFields []string
	}{
		{
			name: "valid VMR",
			vmrs: validVMR,
		},
		{
			name: "missing selector",
			vmrs: func() *virtv1alpha1.VirtualMachineReplicaSet {
				vmrs := validVMR.DeepCopy()
				vmrs.Spec.Selector = nil
				return vmrs
			}(),
			invalidFields: []string{"spec.selector"},
		},
		{
			name: "negative replicas",
			vmrs: func() *virtv1alpha1.VirtualMachineReplicaSet {
				vmrs := validVMR.DeepCopy()
				vmrs.Spec.Replicas = int32Ptr(-1)
				return vmrs
			}(),
			invalidFields: []string{"spec.replicas"},
		},
		{
			name: "missing template labels",
			vmrs: func() *virtv1alpha1.VirtualMachineReplicaSet {
				vmrs := validVMR.DeepCopy()
				vmrs.Spec.Template.ObjectMeta.Labels = nil
				return vmrs
			}(),
			invalidFields: []string{"spec.template.metadata.labels"},
		},
		{
			name: "selector not matching template labels",
			vmrs: func() *virtv1alpha1.VirtualMachineReplicaSet {
				vmrs := validVMR.DeepCopy()
				vmrs.Spec.Template.ObjectMeta.Labels = map[string]string{
					"app": "different",
				}
				return vmrs
			}(),
			invalidFields: []string{"spec.template.metadata.labels"},
		},
		{
			name: "update immutable selector",
			vmrs: func() *virtv1alpha1.VirtualMachineReplicaSet {
				vmrs := validVMR.DeepCopy()
				vmrs.Spec.Selector.MatchLabels["app"] = "changed"
				return vmrs
			}(),
			oldVMR:        validVMR,
			invalidFields: []string{"spec.template.metadata.labels", "spec.selector"},
		},
		{
			name: "update immutable template spec",
			vmrs: func() *virtv1alpha1.VirtualMachineReplicaSet {
				vmrs := validVMR.DeepCopy()
				vmrs.Spec.Template.Spec.Instance.CPU.Sockets = 2
				return vmrs
			}(),
			oldVMR:        validVMR,
			invalidFields: []string{"spec.template.spec"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errs := ValidateVMReplicaSet(context.Background(), tt.vmrs, tt.oldVMR)
			if len(tt.invalidFields) == 0 {
				assert.Empty(t, errs)
			} else {
				assert.Len(t, errs, len(tt.invalidFields))
				errFields := make([]string, len(errs))
				for i, err := range errs {
					errFields[i] = err.Field
				}
				assert.ElementsMatch(t, tt.invalidFields, errFields)
			}
		})
	}
}

func TestMutateVMReplicaSet(t *testing.T) {
	tests := []struct {
		name     string
		vmrs     *virtv1alpha1.VirtualMachineReplicaSet
		oldVMR   *virtv1alpha1.VirtualMachineReplicaSet
		expected *virtv1alpha1.VirtualMachineReplicaSet
	}{
		{
			name: "set default replicas",
			vmrs: &virtv1alpha1.VirtualMachineReplicaSet{
				Spec: virtv1alpha1.VirtualMachineReplicaSetSpec{},
			},
			expected: &virtv1alpha1.VirtualMachineReplicaSet{
				Spec: virtv1alpha1.VirtualMachineReplicaSetSpec{
					Replicas: int32Ptr(defaultReplicas),
				},
			},
		},
		{
			name: "keep existing replicas",
			vmrs: &virtv1alpha1.VirtualMachineReplicaSet{
				Spec: virtv1alpha1.VirtualMachineReplicaSetSpec{
					Replicas: int32Ptr(3),
				},
			},
			expected: &virtv1alpha1.VirtualMachineReplicaSet{
				Spec: virtv1alpha1.VirtualMachineReplicaSetSpec{
					Replicas: int32Ptr(3),
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := MutateVMReplicaSet(context.Background(), tt.vmrs, tt.oldVMR)
			assert.NoError(t, err)
			assert.Equal(t, tt.expected.Spec.Replicas, tt.vmrs.Spec.Replicas)
		})
	}
}

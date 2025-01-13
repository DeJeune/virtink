package controller

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"reflect"

	admissionv1 "k8s.io/api/admission/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/validation/field"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	virtv1alpha1 "github.com/smartxworks/virtink/pkg/apis/virt/v1alpha1"
)

// +kubebuilder:webhook:path=/mutate-v1alpha1-virtualmachinereplicaset,mutating=true,failurePolicy=fail,sideEffects=None,groups=virt.virtink.smartx.com,resources=virtualmachinereplicasets,verbs=create;update,versions=v1alpha1,name=mutate.virtualmachinereplicaset.v1alpha1.virt.virtink.smartx.com,admissionReviewVersions={v1,v1beta1}

type VMReplicaSetMutator struct {
	decoder admission.Decoder
}

var _ admission.Handler = &VMReplicaSetMutator{}

func (h *VMReplicaSetMutator) SetupWebhookWithManager(mgr ctrl.Manager) error {
	h.decoder = admission.NewDecoder(mgr.GetScheme())

	mgr.GetWebhookServer().Register("/mutate-v1alpha1-virtualmachinereplicaset", &webhook.Admission{
		Handler: h,
	})
	return nil
}

func (h *VMReplicaSetMutator) Handle(ctx context.Context, req admission.Request) admission.Response {
	var vmrs virtv1alpha1.VirtualMachineReplicaSet
	if err := h.decoder.Decode(req, &vmrs); err != nil {
		return admission.Errored(http.StatusBadRequest, fmt.Errorf("unmarshal VMReplicaSet: %s", err))
	}

	var err error
	switch req.Operation {
	case admissionv1.Create:
		err = MutateVMReplicaSet(ctx, &vmrs, nil)
	case admissionv1.Update:
		var oldVMR virtv1alpha1.VirtualMachineReplicaSet
		if err := h.decoder.DecodeRaw(req.OldObject, &oldVMR); err != nil {
			return admission.Errored(http.StatusBadRequest, fmt.Errorf("unmarshal old VMReplicaSet: %s", err))
		}
		err = MutateVMReplicaSet(ctx, &vmrs, &oldVMR)
	default:
		return admission.Allowed("")
	}

	if err != nil {
		return admission.Errored(http.StatusInternalServerError, err)
	}

	vmrJSON, err := json.Marshal(vmrs)
	if err != nil {
		return admission.Errored(http.StatusInternalServerError, fmt.Errorf("marshal VMReplicaSet: %s", err))
	}
	return admission.PatchResponseFromRaw(req.Object.Raw, vmrJSON)
}

func MutateVMReplicaSet(ctx context.Context, vmrs *virtv1alpha1.VirtualMachineReplicaSet, oldVMR *virtv1alpha1.VirtualMachineReplicaSet) error {
	if vmrs.Spec.Replicas == nil {
		replicas := defaultReplicas
		vmrs.Spec.Replicas = &replicas
	}
	return nil
}

// +kubebuilder:webhook:path=/validate-v1alpha1-virtualmachinereplicaset,mutating=false,failurePolicy=fail,sideEffects=None,groups=virt.virtink.smartx.com,resources=virtualmachinereplicasets,verbs=create;update,versions=v1alpha1,name=validate.virtualmachinereplicaset.v1alpha1.virt.virtink.smartx.com,admissionReviewVersions={v1,v1beta1}

type VMReplicaSetValidator struct {
	decoder admission.Decoder
}

var _ admission.Handler = &VMReplicaSetValidator{}

func (h *VMReplicaSetValidator) SetupWebhookWithManager(mgr ctrl.Manager) error {
	h.decoder = admission.NewDecoder(mgr.GetScheme())

	mgr.GetWebhookServer().Register("/validate-v1alpha1-virtualmachinereplicaset", &webhook.Admission{
		Handler: h,
	})
	return nil
}

func (h *VMReplicaSetValidator) Handle(ctx context.Context, req admission.Request) admission.Response {
	var vmrs virtv1alpha1.VirtualMachineReplicaSet
	if err := h.decoder.Decode(req, &vmrs); err != nil {
		return admission.Errored(http.StatusBadRequest, fmt.Errorf("unmarshal VMReplicaSet: %s", err))
	}

	var errs field.ErrorList
	switch req.Operation {
	case admissionv1.Create:
		errs = ValidateVMReplicaSet(ctx, &vmrs, nil)
	case admissionv1.Update:
		var oldVmrs virtv1alpha1.VirtualMachineReplicaSet
		if err := h.decoder.DecodeRaw(req.OldObject, &oldVmrs); err != nil {
			return admission.Errored(http.StatusBadRequest, fmt.Errorf("unmarshal old VMReplicaSet: %s", err))
		}
		errs = ValidateVMReplicaSet(ctx, &vmrs, &oldVmrs)
	default:
		return admission.Allowed("")
	}

	if len(errs) > 0 {
		return webhook.Denied(errs.ToAggregate().Error())
	}
	return admission.Allowed("")
}

func ValidateVMReplicaSet(ctx context.Context, vmrs *virtv1alpha1.VirtualMachineReplicaSet, oldVmrs *virtv1alpha1.VirtualMachineReplicaSet) field.ErrorList {
	var errs field.ErrorList
	errs = append(errs, ValidateVMReplicaSetSpec(ctx, &vmrs.Spec, field.NewPath("spec"))...)
	if oldVmrs != nil {
		errs = append(errs, ValidateVMReplicaSetUpdate(ctx, vmrs, oldVmrs)...)
	}
	return errs
}

func ValidateVMReplicaSetSpec(ctx context.Context, spec *virtv1alpha1.VirtualMachineReplicaSetSpec, fieldPath *field.Path) field.ErrorList {
	var errs field.ErrorList
	if spec == nil {
		errs = append(errs, field.Required(fieldPath, ""))
		return errs
	}

	if spec.Selector == nil {
		errs = append(errs, field.Required(fieldPath.Child("selector"), ""))
	}

	if spec.Replicas != nil && *spec.Replicas < 0 {
		errs = append(errs, field.Invalid(fieldPath.Child("replicas"), *spec.Replicas, "must be greater than or equal to 0"))
	}

	// Validate template existence and labels
	if spec.Template == nil {
		errs = append(errs, field.Required(fieldPath.Child("template"), ""))
		return errs
	}

	if spec.Template.ObjectMeta.Labels == nil {
		errs = append(errs, field.Required(fieldPath.Child("template", "metadata", "labels"), ""))
	}

	// Validate selector matches template labels
	if spec.Selector != nil && spec.Template.ObjectMeta.Labels != nil {
		selector, err := metav1.LabelSelectorAsSelector(spec.Selector)
		if err != nil {
			errs = append(errs, field.Invalid(fieldPath.Child("selector"), spec.Selector, fmt.Sprintf("invalid selector: %v", err)))
		} else if !selector.Matches(labels.Set(spec.Template.ObjectMeta.Labels)) {
			errs = append(errs, field.Invalid(fieldPath.Child("template", "metadata", "labels"), spec.Template.ObjectMeta.Labels, "must match selector"))
		}
	}

	return errs
}

func ValidateVMReplicaSetUpdate(ctx context.Context, vmrs *virtv1alpha1.VirtualMachineReplicaSet, oldVmrs *virtv1alpha1.VirtualMachineReplicaSet) field.ErrorList {
	var errs field.ErrorList

	// Validate immutable fields
	if !reflect.DeepEqual(vmrs.Spec.Selector, oldVmrs.Spec.Selector) {
		errs = append(errs, field.Forbidden(field.NewPath("spec", "selector"), "field is immutable"))
	}

	if !reflect.DeepEqual(vmrs.Spec.Template.Spec, oldVmrs.Spec.Template.Spec) {
		errs = append(errs, field.Forbidden(field.NewPath("spec", "template", "spec"), "field is immutable"))
	}

	return errs
}

package controller

import (
	"context"
	"errors"
	"fmt"
	"math"
	"reflect"
	"sort"
	"sync"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/workqueue"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	virtv1alpha1 "github.com/DeJeune/virtink/pkg/apis/virt/v1alpha1"
	"github.com/DeJeune/virtink/pkg/controller/expectations"
)

const (
	defaultReplicas       = int32(1)
	requeueInterval       = time.Minute
	deleteRequeueInterval = time.Second * 10
	healthCheckTimeout    = time.Minute * 5
	maxRetries            = 5
	BurstReplicas         = 10
)

// VMReplicaSetReconciler reconciles a VirtualMachineReplicaSet object
type VMReplicaSetReconciler struct {
	client.Client
	Scheme       *runtime.Scheme
	Recorder     record.EventRecorder
	Expectations *expectations.UIDTrackingControllerExpectations
}

// +kubebuilder:rbac:groups=virt.virtink.smartx.com,resources=virtualmachinereplicasets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=virt.virtink.smartx.com,resources=virtualmachinereplicasets/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=virt.virtink.smartx.com,resources=virtualmachinereplicasets/scale,verbs=get;update;patch
// +kubebuilder:rbac:groups=virt.virtink.smartx.com,resources=virtualmachines,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=events,verbs=create;update;patch

// Reconcile handles the reconciliation loop for VMReplicaSet resources
// It ensures the desired state matches the actual state in the cluster
func (r *VMReplicaSetReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	vmrs := &virtv1alpha1.VirtualMachineReplicaSet{}
	if err := r.Get(ctx, req.NamespacedName, vmrs); err != nil {
		logger.Info("VMReplicaSet not found, ignoring", "namespacedName", req.NamespacedName)
		r.Expectations.DeleteExpectations(req.String())
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	oldStatus := vmrs.Status.DeepCopy()

	rerr := r.reconcile(ctx, vmrs)

	if oldStatus.Replicas == vmrs.Status.Replicas &&
		oldStatus.ReadyReplicas == vmrs.Status.ReadyReplicas &&
		oldStatus.AvailableReplicas == vmrs.Status.AvailableReplicas &&
		oldStatus.ObservedGeneration == vmrs.Status.ObservedGeneration &&
		reflect.DeepEqual(oldStatus.Conditions, vmrs.Status.Conditions) {
		return ctrl.Result{}, nil
	}

	if !reflect.DeepEqual(oldStatus, vmrs.Status) {
		if err := r.Status().Update(ctx, vmrs); err != nil {
			if rerr == nil {
				if apierrors.IsConflict(err) {
					return ctrl.Result{Requeue: true}, nil
				}
				return ctrl.Result{}, fmt.Errorf("update VMReplicaSet status: %s", err)
			}
			if !apierrors.IsConflict(err) {
				logger.Error(err, "update VMReplicaSet status")
			}
		}

	}

	if rerr != nil {
		reconcileErr := reconcileError{}
		if errors.As(rerr, &reconcileErr) {
			return reconcileErr.Result, nil
		}

		r.Recorder.Eventf(vmrs, corev1.EventTypeWarning, "FailedReconcile", "Failed to reconcile VMReplicaSet: %s", rerr)
		return ctrl.Result{}, rerr
	}

	return ctrl.Result{}, nil
}

func (r *VMReplicaSetReconciler) reconcile(ctx context.Context, vmrs *virtv1alpha1.VirtualMachineReplicaSet) error {

	needsSync := r.Expectations.SatisfiedExpectations(client.ObjectKeyFromObject(vmrs).String())
	// Get controlled VMs
	vms, err := r.getControlledVMs(ctx, vmrs)
	if err != nil {
		return fmt.Errorf("failed to get controlled VMs: %v", err)
	}

	activeVms := filterActiveVMs(vms)
	finishedVms := append(filterFinishedVMs(vms), filterUnknownVMs(vms)...)

	var scaleErr error

	if needsSync && !vmrs.Spec.Paused && vmrs.DeletionTimestamp.IsZero() {
		scaleErr = r.scaleVMs(ctx, vmrs, activeVms)
		if len(finishedVms) > 0 && scaleErr == nil {
			scaleErr = r.cleanVMs(ctx, vmrs, finishedVms)
		}
	}

	return r.calculateStatus(vmrs, activeVms, scaleErr)
}

func (r *VMReplicaSetReconciler) scaleVMs(ctx context.Context, vmrs *virtv1alpha1.VirtualMachineReplicaSet, vms []*virtv1alpha1.VirtualMachine) error {
	// Calculate how many replicas we should have
	replicas := int32(1)
	if vmrs.Spec.Replicas != nil {
		replicas = *vmrs.Spec.Replicas
	}

	// Calculate the difference between current and desired replicas
	diff := len(vms) - int(replicas)
	if diff == 0 {
		return nil
	}
	maxDiff := int(math.Min(math.Abs(float64(diff)), float64(BurstReplicas)))

	errChan := make(chan error, maxDiff)

	var wg sync.WaitGroup
	wg.Add(maxDiff)
	if diff < 0 {
		// Need to create VMs
		r.Expectations.ExpectCreations(client.ObjectKeyFromObject(vmrs).String(), maxDiff)
		for i := 0; i < maxDiff; i++ {
			go func() {
				defer wg.Done()
				if err := r.createVM(ctx, vmrs.Spec.Template, vmrs); err != nil {
					errChan <- err
					return
				}
			}()
		}
	} else if diff > 0 {
		// Need to delete VMs
		vmToDelete := getVMsToDelete(vms, maxDiff)
		keys := []string{}
		for _, vm := range vmToDelete {
			keys = append(keys, client.ObjectKeyFromObject(vm).String())
		}
		r.Expectations.ExpectDeletions(client.ObjectKeyFromObject(vmrs).String(), keys)
		for _, vm := range vmToDelete {
			go func(vm *virtv1alpha1.VirtualMachine) {
				defer wg.Done()
				if err := r.DeleteVM(ctx, vm, vmrs); err != nil {
					r.Recorder.Eventf(vmrs, corev1.EventTypeWarning, "FailedDelete",
						"Error deleting virtual machine instance %s: %v", vm.ObjectMeta.Name, err)
					errChan <- err
					return
				}
			}(vm)
		}
	}

	wg.Wait()
	select {
	case err := <-errChan:
		return err
	default:
	}
	return nil
}

func (r *VMReplicaSetReconciler) cleanVMs(ctx context.Context, vmrs *virtv1alpha1.VirtualMachineReplicaSet, vms []*virtv1alpha1.VirtualMachine) error {
	maxDiff := int(math.Min(math.Abs(float64(len(vms))), float64(BurstReplicas)))
	errChan := make(chan error, maxDiff)
	var wg sync.WaitGroup
	wg.Add(maxDiff)
	for _, vm := range vms {
		go func(targetVM *virtv1alpha1.VirtualMachine) {
			defer wg.Done()
			if err := r.DeleteVM(ctx, targetVM, vmrs); err != nil {
				errChan <- err
				return
			}
		}(vm)
	}
	wg.Wait()
	select {
	case err := <-errChan:
		return err
	default:
	}
	return nil
}

func (r *VMReplicaSetReconciler) DeleteVM(ctx context.Context, vm *virtv1alpha1.VirtualMachine, obj client.Object) error {
	if err := r.Delete(ctx, vm); err != nil && !apierrors.IsNotFound(err) {
		r.Expectations.DeletionObserved(client.ObjectKeyFromObject(obj).String(), client.ObjectKeyFromObject(vm).String())
		r.Recorder.Eventf(obj, corev1.EventTypeWarning, "FailedDelete",
			"Error deleting virtual machine instance %s: %v", vm.ObjectMeta.Name, err)
		return err
	}

	r.Recorder.Eventf(obj, corev1.EventTypeNormal, "SuccessfulDelete",
		"Deleted virtual machine: %v", vm.Name)
	return nil
}

func (r *VMReplicaSetReconciler) createVM(ctx context.Context, template *virtv1alpha1.VirtualMachineTemplateSpec, parentObject client.Object) error {
	desiredLabels := deepcopyVMsLabelSet(template)
	desiredAnnotations := deepcopyVMsAnnotationSet(template)
	accessor, err := meta.Accessor(parentObject)
	if err != nil {
		return fmt.Errorf("parentObject does not have ObjectMeta, %v", err)
	}

	vm := &virtv1alpha1.VirtualMachine{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: fmt.Sprintf("%s-", accessor.GetName()),
			Namespace:    accessor.GetNamespace(),
			Labels:       desiredLabels,
			Annotations:  desiredAnnotations,
		},
		Spec: *template.Spec.DeepCopy(),
	}

	// Set owner reference
	if err := controllerutil.SetControllerReference(parentObject, vm, r.Scheme); err != nil {
		return fmt.Errorf("failed to set controller reference: %v", err)
	}

	// Create the VM
	if err := r.Create(ctx, vm); err != nil {
		r.Expectations.CreationObserved(client.ObjectKeyFromObject(parentObject).String())
		// only send an event if the namespace isn't terminating
		if !apierrors.HasStatusCause(err, corev1.NamespaceTerminatingCause) {
			r.Recorder.Eventf(parentObject, corev1.EventTypeWarning, "FailedCreate",
				"Error creating: %v", err)
		}
		return err
	}

	r.Recorder.Eventf(parentObject, corev1.EventTypeNormal, "SuccessfulCreate",
		"Created virtual machine: %v", vm.Name)
	return nil
}

func (r *VMReplicaSetReconciler) getControlledVMs(ctx context.Context, vmrs *virtv1alpha1.VirtualMachineReplicaSet) ([]*virtv1alpha1.VirtualMachine, error) {
	// List all VMs in the namespace
	vmList := &virtv1alpha1.VirtualMachineList{}
	if err := r.List(ctx, vmList, client.InNamespace(vmrs.Namespace), client.MatchingFields{"vmrUID": string(vmrs.UID)}); err != nil {
		return nil, err
	}

	selector, err := metav1.LabelSelectorAsSelector(vmrs.Spec.Selector)
	if err != nil {
		return nil, fmt.Errorf("failed to convert label selector: %w", err)
	}

	// Filter for VMs controlled by this VMReplicaSet and match the selector
	var controlled []*virtv1alpha1.VirtualMachine
	for _, vm := range vmList.Items {
		if selector.Matches(labels.Set(vm.Labels)) {
			controlled = append(controlled, &vm)
		}
	}
	return controlled, nil
}

func (r *VMReplicaSetReconciler) calculateStatus(vmrs *virtv1alpha1.VirtualMachineReplicaSet, vms []*virtv1alpha1.VirtualMachine, scaleErr error) error {
	readyReplicasCount := 0
	availableReplicasCount := 0
	templateLabel := labels.Set(vmrs.Spec.Template.ObjectMeta.Labels).AsSelectorPreValidated()
	for _, vm := range vms {
		if templateLabel.Matches(labels.Set(vm.Labels)) {
			if isVMReady(vm) {
				readyReplicasCount++
			}
			if isVMAvailable(vm) {
				availableReplicasCount++
			}
		}
	}

	failureCondition := getCondition(vmrs.Status, virtv1alpha1.VirtualMachineReplicaSetReplicaFailure)
	if scaleErr != nil && failureCondition == nil {
		var reason string
		if diff := len(vms) - int(*(vmrs.Spec.Replicas)); diff < 0 {
			reason = "FailedCreate"
		} else if diff > 0 {
			reason = "FailedDelete"
		}
		vmrs.Status.Conditions = append(vmrs.Status.Conditions, virtv1alpha1.VirtualMachineReplicaSetCondition{
			Type:               virtv1alpha1.VirtualMachineReplicaSetReplicaFailure,
			Status:             corev1.ConditionTrue,
			Reason:             reason,
			Message:            scaleErr.Error(),
			LastTransitionTime: metav1.Now(),
			LastProbeTime:      metav1.Now(),
		})
	} else if scaleErr == nil && failureCondition != nil {
		removeCondition(&vmrs.Status, virtv1alpha1.VirtualMachineReplicaSetReplicaFailure)
	}

	if vmrs.Spec.Paused && !hasCondition(vmrs.Status, virtv1alpha1.VirtualMachineReplicaSetPaused) {
		vmrs.Status.Conditions = append(vmrs.Status.Conditions, virtv1alpha1.VirtualMachineReplicaSetCondition{
			Type:               virtv1alpha1.VirtualMachineReplicaSetPaused,
			Status:             corev1.ConditionTrue,
			LastTransitionTime: metav1.Now(),
			LastProbeTime:      metav1.Now(),
			Reason:             "Paused",
			Message:            "VirtualMachineReplicaSet is paused",
		})
	} else if !vmrs.Spec.Paused && hasCondition(vmrs.Status, virtv1alpha1.VirtualMachineReplicaSetPaused) {
		removeCondition(&vmrs.Status, virtv1alpha1.VirtualMachineReplicaSetPaused)
	}

	vmrs.Status.Replicas = int32(len(vms))
	vmrs.Status.ReadyReplicas = int32(readyReplicasCount)
	vmrs.Status.AvailableReplicas = int32(availableReplicasCount)
	vmrs.Status.ObservedGeneration = vmrs.Generation
	return nil
}

func (r *VMReplicaSetReconciler) getMatchingControllers(ctx context.Context, vm *virtv1alpha1.VirtualMachine) []*virtv1alpha1.VirtualMachineReplicaSet {
	var vmrl virtv1alpha1.VirtualMachineReplicaSetList
	var vmrss []*virtv1alpha1.VirtualMachineReplicaSet
	if err := r.List(ctx, &vmrl, client.InNamespace(vm.Namespace)); err != nil {
		return nil
	}

	for _, vmrs := range vmrl.Items {
		selector, err := metav1.LabelSelectorAsSelector(vmrs.Spec.Selector)
		if err != nil {
			return nil
		}
		if selector.Matches(labels.Set(vm.Labels)) {
			vmrss = append(vmrss, &vmrs)
		}
	}
	return vmrss
}

func (r *VMReplicaSetReconciler) deleteVM(ctx context.Context, obj client.Object, queue workqueue.TypedRateLimitingInterface[reconcile.Request]) {
	logger := log.FromContext(ctx)
	vm := obj.(*virtv1alpha1.VirtualMachine)
	namespace := vm.GetNamespace()

	controllerRef := metav1.GetControllerOf(vm)
	if controllerRef == nil {
		logger.V(2).Info("No controller ref found for VM", "vm", vm.GetName())
		return
	}
	vmrs := r.resolveControllerRef(namespace, controllerRef)
	if vmrs == nil {
		logger.V(2).Info("No VMReplicaSet found for VM", "vm", vm.GetName())
		return
	}

	rsKey := types.NamespacedName{Namespace: namespace, Name: controllerRef.Name}
	vmKey := types.NamespacedName{Namespace: namespace, Name: vm.GetName()}

	r.Expectations.DeletionObserved(rsKey.String(), vmKey.String())
	queue.Add(reconcile.Request{NamespacedName: client.ObjectKeyFromObject(vmrs)})
}

func (r *VMReplicaSetReconciler) resolveControllerRef(namespace string, ref *metav1.OwnerReference) *virtv1alpha1.VirtualMachineReplicaSet {
	if ref.Kind != "VirtualMachineReplicaSet" {
		return nil
	}

	vmrs := &virtv1alpha1.VirtualMachineReplicaSet{}
	vmrKey := types.NamespacedName{Namespace: namespace, Name: ref.Name}
	if err := r.Get(context.Background(), vmrKey, vmrs); err != nil {
		return nil
	}
	if vmrs.UID != ref.UID {
		return nil
	}
	return vmrs
}

func (r *VMReplicaSetReconciler) SetupWithManager(mgr ctrl.Manager) error {
	if err := mgr.GetFieldIndexer().IndexField(context.Background(), &virtv1alpha1.VirtualMachine{}, "vmrUID", func(obj client.Object) []string {
		vm := obj.(*virtv1alpha1.VirtualMachine)
		controllerRef := metav1.GetControllerOf(vm)
		if controllerRef != nil && controllerRef.APIVersion == virtv1alpha1.SchemeGroupVersion.String() && controllerRef.Kind == "VirtualMachineReplicaSet" {
			return []string{string(controllerRef.UID)}
		}
		return nil
	}); err != nil {
		return fmt.Errorf("index VirtualMachine by vmrUID: %s", err)
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&virtv1alpha1.VirtualMachineReplicaSet{}).
		Owns(&virtv1alpha1.VirtualMachine{}).
		Watches(
			&virtv1alpha1.VirtualMachine{},
			handler.Funcs{
				CreateFunc: func(ctx context.Context, event event.CreateEvent, queue workqueue.TypedRateLimitingInterface[reconcile.Request]) {
					vm := event.Object.(*virtv1alpha1.VirtualMachine)
					if !vm.DeletionTimestamp.IsZero() {
						r.deleteVM(ctx, vm, queue)
						return
					}

					if controllerRef := metav1.GetControllerOf(vm); controllerRef != nil {
						rs := r.resolveControllerRef(vm.Namespace, controllerRef)
						if rs != nil {
							rsKey := client.ObjectKeyFromObject(rs)
							r.Expectations.CreationObserved(rsKey.String())
							queue.Add(reconcile.Request{NamespacedName: rsKey})
							return
						}
						return
					}

					rss := r.getMatchingControllers(ctx, vm)
					if len(rss) == 0 {
						return
					}
					for _, rs := range rss {
						rsKey := client.ObjectKeyFromObject(rs)
						queue.Add(reconcile.Request{NamespacedName: rsKey})
					}
				},
				DeleteFunc: func(ctx context.Context, event event.DeleteEvent, queue workqueue.TypedRateLimitingInterface[reconcile.Request]) {
					r.deleteVM(ctx, event.Object, queue)
				},
				UpdateFunc: func(ctx context.Context, event event.UpdateEvent, queue workqueue.TypedRateLimitingInterface[reconcile.Request]) {
					oldVM := event.ObjectOld.(*virtv1alpha1.VirtualMachine)
					newVM := event.ObjectNew.(*virtv1alpha1.VirtualMachine)
					if oldVM.ResourceVersion == newVM.ResourceVersion {
						return
					}

					labelChanged := !equality.Semantic.DeepEqual(oldVM.Labels, newVM.Labels)
					if !newVM.DeletionTimestamp.IsZero() {
						r.deleteVM(ctx, newVM, queue)
						if labelChanged {
							r.deleteVM(ctx, oldVM, queue)
						}
					}

					oldControllerRef := metav1.GetControllerOf(oldVM)
					newControllerRef := metav1.GetControllerOf(newVM)
					controllerRefChanged := !equality.Semantic.DeepEqual(newControllerRef, oldControllerRef)
					if controllerRefChanged && oldControllerRef != nil {
						if rs := r.resolveControllerRef(oldVM.Namespace, oldControllerRef); rs != nil {
							queue.Add(reconcile.Request{NamespacedName: client.ObjectKeyFromObject(rs)})
						}
					}
					if newControllerRef != nil {
						if rs := r.resolveControllerRef(newVM.Namespace, newControllerRef); rs != nil {
							queue.Add(reconcile.Request{NamespacedName: client.ObjectKeyFromObject(rs)})
						}
						return
					}

					if labelChanged || controllerRefChanged {
						vmrs := r.getMatchingControllers(ctx, newVM)
						if len(vmrs) == 0 {
							return
						}
						for _, vmrs := range vmrs {
							queue.Add(reconcile.Request{NamespacedName: client.ObjectKeyFromObject(vmrs)})
						}
					}
				},
			},
		).
		Complete(r)
}

// Helper functions

func isVMFinal(vm *virtv1alpha1.VirtualMachine) bool {
	return vm.Status.Phase == virtv1alpha1.VirtualMachineFailed ||
		vm.Status.Phase == virtv1alpha1.VirtualMachineSucceeded
}

func isVMReady(vm *virtv1alpha1.VirtualMachine) bool {
	return vm.Status.Phase != "" && isVMActive(vm) && vm.Status.Phase == virtv1alpha1.VirtualMachineRunning &&
		meta.IsStatusConditionTrue(vm.Status.Conditions, string(virtv1alpha1.VirtualMachineReady))
}

func isVMActive(vm *virtv1alpha1.VirtualMachine) bool {
	return !isVMFinal(vm) &&
		vm.DeletionTimestamp.IsZero()
}

func isVMAvailable(vm *virtv1alpha1.VirtualMachine) bool {
	return isVMReady(vm) && vm.Status.Phase != virtv1alpha1.VirtualMachineUnknown
}

func isVMFinished(vm *virtv1alpha1.VirtualMachine) bool {
	return vm.Status.Phase == virtv1alpha1.VirtualMachineFailed ||
		vm.Status.Phase == virtv1alpha1.VirtualMachineSucceeded
}

func isVMUnknown(vm *virtv1alpha1.VirtualMachine) bool {
	return vm.Status.Phase == virtv1alpha1.VirtualMachineUnknown
}

func filter(vms []*virtv1alpha1.VirtualMachine, f func(vm *virtv1alpha1.VirtualMachine) bool) []*virtv1alpha1.VirtualMachine {
	filtered := []*virtv1alpha1.VirtualMachine{}
	for _, vm := range vms {
		if f(vm) {
			filtered = append(filtered, vm)
		}
	}
	return filtered
}

func filterFinishedVMs(vms []*virtv1alpha1.VirtualMachine) []*virtv1alpha1.VirtualMachine {
	return filter(vms, isVMFinished)
}

func filterUnknownVMs(vms []*virtv1alpha1.VirtualMachine) []*virtv1alpha1.VirtualMachine {
	return filter(vms, isVMUnknown)
}

func filterReadyVMs(vms []*virtv1alpha1.VirtualMachine) []*virtv1alpha1.VirtualMachine {
	return filter(vms, isVMReady)
}

func filterActiveVMs(vms []*virtv1alpha1.VirtualMachine) []*virtv1alpha1.VirtualMachine {
	return filter(vms, isVMActive)
}

func sortVMsByCreationTimestamp(vms []*virtv1alpha1.VirtualMachine, ascending bool) {
	if ascending {
		sort.Slice(vms, func(i, j int) bool {
			return vms[i].CreationTimestamp.Before(&vms[j].CreationTimestamp)
		})
	} else {
		sort.Slice(vms, func(i, j int) bool {
			return vms[j].CreationTimestamp.Before(&vms[i].CreationTimestamp)
		})
	}
}

func getVMsToDelete(filterVms []*virtv1alpha1.VirtualMachine, diff int) []*virtv1alpha1.VirtualMachine {
	// No need to sort Vms if we want to delete all VMs
	// diff always <= length of filterVms, so no need to handle diff > case
	if diff < len(filterVms) {
		sortVMsByCreationTimestamp(filterVms, false)
	}
	return filterVms[:diff]
}

func getCondition(status virtv1alpha1.VirtualMachineReplicaSetStatus, conditionType virtv1alpha1.VirtualMachineReplicaSetConditionType) *virtv1alpha1.VirtualMachineReplicaSetCondition {
	for _, cond := range status.Conditions {
		if cond.Type == conditionType {
			return &cond
		}
	}
	return nil
}

// removeCondition removes the condition with the provided type from the replicaset status.
func removeCondition(status *virtv1alpha1.VirtualMachineReplicaSetStatus, condType virtv1alpha1.VirtualMachineReplicaSetConditionType) {
	status.Conditions = filterOutCondition(status.Conditions, condType)
}

func hasCondition(status virtv1alpha1.VirtualMachineReplicaSetStatus, condType virtv1alpha1.VirtualMachineReplicaSetConditionType) bool {
	for _, cond := range status.Conditions {
		if cond.Type == condType {
			return true
		}
	}
	return false
}

// filterOutCondition returns a new slice of replicaset conditions without conditions with the provided type.
func filterOutCondition(conditions []virtv1alpha1.VirtualMachineReplicaSetCondition, condType virtv1alpha1.VirtualMachineReplicaSetConditionType) []virtv1alpha1.VirtualMachineReplicaSetCondition {
	var newConditions []virtv1alpha1.VirtualMachineReplicaSetCondition
	for _, c := range conditions {
		if c.Type == condType {
			continue
		}
		newConditions = append(newConditions, c)
	}
	return newConditions
}

func OwnerRef(rs *virtv1alpha1.VirtualMachineReplicaSet) metav1.OwnerReference {
	t := true
	gvk := virtv1alpha1.SchemeGroupVersion.WithKind("VirtualMachineReplicaSet")
	return metav1.OwnerReference{
		APIVersion:         gvk.GroupVersion().String(),
		Kind:               gvk.Kind,
		Name:               rs.ObjectMeta.Name,
		UID:                rs.ObjectMeta.UID,
		Controller:         &t,
		BlockOwnerDeletion: &t,
	}
}

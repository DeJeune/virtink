package controller

import (
	"context"
	"fmt"
	"math"
	"reflect"
	"sort"
	"strings"
	"sync"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/rand"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	virtv1alpha1 "github.com/smartxworks/virtink/pkg/apis/virt/v1alpha1"
)

const (
	// VMReplicaSetFinalizer is the name of the finalizer added to VMReplicaSets
	VMReplicaSetFinalizer = "virtink.io/vmrs-protection"
	defaultReplicas       = int32(1)
	requeueInterval       = time.Minute
	deleteRequeueInterval = time.Second * 10
	healthCheckTimeout    = time.Minute * 5
	maxRetries            = 5
	BurstReplicas         = 250
)

// VMReplicaSetReconciler reconciles a VirtualMachineReplicaSet object
type VMReplicaSetReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder
}

// +kubebuilder:rbac:groups=virt.virtink.smartx.com,resources=virtualmachinereplicasets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=virt.virtink.smartx.com,resources=virtualmachinereplicasets/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=virt.virtink.smartx.com,resources=virtualmachinereplicasets/finalizers,verbs=update
// +kubebuilder:rbac:groups=virt.virtink.smartx.com,resources=virtualmachines,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=events,verbs=create;update;patch
// Reconcile handles the reconciliation loop for VMReplicaSet resources
// It ensures the desired state matches the actual state in the cluster
func (r *VMReplicaSetReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	logger.Info("Starting reconciliation", "namespacedName", req.NamespacedName)

	var vmr virtv1alpha1.VirtualMachineReplicaSet
	if err := r.Get(ctx, req.NamespacedName, &vmr); err != nil {
		logger.Info("VMReplicaSet not found, ignoring", "namespacedName", req.NamespacedName)
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Handle reconciliation
	if err := r.reconcile(ctx, &vmr); err != nil {
		if apierrors.IsConflict(err) {
			return ctrl.Result{Requeue: true}, nil
		}
		ctrl.LoggerFrom(ctx).Error(err, "Failed to reconcile VMReplicaSet")
		return ctrl.Result{}, err
	}

	if err := r.gcVMs(ctx, &vmr); err != nil {
		return ctrl.Result{}, fmt.Errorf("GC VMs: %w", err)
	}

	return ctrl.Result{}, nil
}

func (r *VMReplicaSetReconciler) reconcile(ctx context.Context, vmr *virtv1alpha1.VirtualMachineReplicaSet) error {
	// Add finalizer if not present
	if vmr.DeletionTimestamp.IsZero() && !controllerutil.ContainsFinalizer(vmr, VMReplicaSetFinalizer) {
		controllerutil.AddFinalizer(vmr, VMReplicaSetFinalizer)
		return r.Client.Update(ctx, vmr)
	}
	// Get controlled VMs
	vms, err := r.getControlledVMs(ctx, vmr)
	if err != nil {
		return fmt.Errorf("failed to get controlled VMs: %v", err)
	}
	var scaleErr error
	activeVms := filterReadyVMs(vms)
	finishedVms := append(filterFinishedVMs(vms), filterUnknownVMs(vms)...)
	if !vmr.Spec.Paused && vmr.DeletionTimestamp.IsZero() {
		scaleErr = r.scaleVMs(ctx, vmr, activeVms)
		if len(finishedVms) > 0 && scaleErr == nil {
			scaleErr = r.cleanVMs(ctx, vmr, finishedVms)
		}
	}

	if scaleErr != nil {
		return fmt.Errorf("failed to scale VMs: %v", scaleErr)
	}

	newStatus := r.calculateStatus(vmr, activeVms, scaleErr)

	return r.updateStatus(ctx, vmr, newStatus)
}

func (r *VMReplicaSetReconciler) scaleVMs(ctx context.Context, vmr *virtv1alpha1.VirtualMachineReplicaSet, vms []*virtv1alpha1.VirtualMachine) error {
	// Calculate how many replicas we should have
	replicas := int32(1)
	if vmr.Spec.Replicas != nil {
		replicas = *vmr.Spec.Replicas
	}

	// Update replica failure condition
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
		for i := 0; i < -diff && i < maxDiff; i++ {
			go func() {
				defer wg.Done()
				select {
				case <-ctx.Done():
					errChan <- ctx.Err()
					return
				default:
					if err := r.createVM(ctx, &vmr.Spec.Template, vmr); err != nil {
						r.Recorder.Eventf(vmr, corev1.EventTypeWarning, "FailedCreate",
							"Failed to create VM: %v", err)
						errChan <- err
						return
					}
				}
			}()
		}
	} else if diff > 0 {
		// Need to delete VMs
		vmToDelete := getVMsToDelete(vms, maxDiff)
		for _, vm := range vmToDelete {
			vm := vm // Create new variable for goroutine
			go func() {
				defer wg.Done()
				select {
				case <-ctx.Done():
					errChan <- ctx.Err()
					return
				default:
					if err := r.DeleteVM(ctx, vm, vmr); err != nil {
						r.Recorder.Eventf(vmr, corev1.EventTypeWarning, "FailedDelete",
							"Error deleting virtual machine instance %s: %v", vm.ObjectMeta.Name, err)
						errChan <- err
						return
					}
					r.Recorder.Eventf(vmr, corev1.EventTypeNormal, "SuccessfulDelete",
						"Deleted virtual machine instance %s", vm.ObjectMeta.Name)
				}
			}()
		}
	}

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case err := <-errChan:
		return err
	case <-done:
		return nil
	}
}

func (r *VMReplicaSetReconciler) cleanVMs(ctx context.Context, vmr *virtv1alpha1.VirtualMachineReplicaSet, vms []*virtv1alpha1.VirtualMachine) error {
	maxDiff := int(math.Min(math.Abs(float64(len(vms))), float64(BurstReplicas)))
	errChan := make(chan error, maxDiff)
	var wg sync.WaitGroup
	wg.Add(maxDiff)
	for _, vm := range vms {
		go func(targetVM *virtv1alpha1.VirtualMachine) {
			defer wg.Done()
			if err := r.DeleteVM(ctx, targetVM, vmr); err != nil {
				errChan <- err
				return
			}
			r.Recorder.Eventf(vmr, corev1.EventTypeNormal, "SuccessfulDelete",
				"Cleaned up virtual machine: %v", targetVM.Name)
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

	// Set owner reference
	vm := &virtv1alpha1.VirtualMachine{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: fmt.Sprintf("%s-", accessor.GetName()),
			Namespace:    accessor.GetNamespace(),
			Labels:       desiredLabels,
			Annotations:  desiredAnnotations,
		},
		Spec: *template.Spec.DeepCopy(),
	}

	if err := controllerutil.SetControllerReference(parentObject, vm, r.Scheme); err != nil {
		return fmt.Errorf("Failed to set controller reference: %v", err)
	}

	// Create the VM
	if err := r.Client.Create(ctx, vm); err != nil {
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

func (r *VMReplicaSetReconciler) getControlledVMs(ctx context.Context, vmr *virtv1alpha1.VirtualMachineReplicaSet) ([]*virtv1alpha1.VirtualMachine, error) {
	// List all VMs in the namespace
	var vmList virtv1alpha1.VirtualMachineList
	if err := r.Client.List(ctx, &vmList, client.InNamespace(vmr.Namespace)); err != nil {
		return nil, err
	}

	selector, err := metav1.LabelSelectorAsSelector(vmr.Spec.Selector)
	if err != nil {
		return nil, fmt.Errorf("failed to convert label selector: %w", err)
	}

	// Filter for VMs controlled by this VMReplicaSet and match the selector
	var controlled []*virtv1alpha1.VirtualMachine
	for _, vm := range vmList.Items {
		if metav1.IsControlledBy(&vm, vmr) && selector.Matches(labels.Set(vm.Labels)) {
			controlled = append(controlled, &vm)
		}
	}
	return controlled, nil
}

func (r *VMReplicaSetReconciler) updateStatus(ctx context.Context, vmr *virtv1alpha1.VirtualMachineReplicaSet, newStatus virtv1alpha1.VirtualMachineReplicaSetStatus) error {
	logger := log.FromContext(ctx)
	// This is the steady state. It happens when the ReplicaSet doesn't have any expectations, since
	// we do a periodic relist every 30s. If the generations differ but the replicas are
	// the same, a caller might've resized to the same replica count.
	if vmr.Status.Replicas == newStatus.Replicas &&
		vmr.Status.ReadyReplicas == newStatus.ReadyReplicas &&
		vmr.Status.AvailableReplicas == newStatus.AvailableReplicas &&
		vmr.Generation == vmr.Status.ObservedGeneration &&
		reflect.DeepEqual(vmr.Status.Conditions, newStatus.Conditions) {
		return nil
	}

	newStatus.ObservedGeneration = vmr.Generation

	var getErr, updateErr error
	for i, vmr := 0, vmr; ; i++ {
		logger.Info(fmt.Sprintf("Updating status for %v: %s/%s, ", vmr.Kind, vmr.Namespace, vmr.Name) +
			fmt.Sprintf("replicas: %d->%d (need %d), ",
				vmr.Status.Replicas, newStatus.Replicas, *(vmr.Spec.Replicas)) +
			fmt.Sprintf("readyReplicas: %d->%d, ",
				vmr.Status.ReadyReplicas, newStatus.ReadyReplicas) +
			fmt.Sprintf("availableReplicas: %d->%d, ",
				vmr.Status.AvailableReplicas, newStatus.AvailableReplicas) +
			fmt.Sprintf("sequence No: %v->%v", vmr.Status.ObservedGeneration, newStatus.ObservedGeneration))

		vmr.Status = newStatus
		updateErr = r.Client.Status().Update(ctx, vmr)
		if updateErr != nil {
			logger.Error(updateErr, "Failed to update status", "namespacedName", types.NamespacedName{Namespace: vmr.Namespace, Name: vmr.Name})
			return updateErr
		}

		if i >= maxRetries {
			break
		}

		if getErr = r.Client.Get(ctx, types.NamespacedName{Namespace: vmr.Namespace, Name: vmr.Name}, vmr); getErr != nil {
			return getErr
		}
	}

	return nil
}

func (r *VMReplicaSetReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&virtv1alpha1.VirtualMachineReplicaSet{}).
		Owns(&virtv1alpha1.VirtualMachine{}).
		Watches(
			&virtv1alpha1.VirtualMachine{},
			handler.EnqueueRequestsFromMapFunc(func(ctx context.Context, obj client.Object) []reconcile.Request {
				vm, ok := obj.(*virtv1alpha1.VirtualMachine)
				if !ok {
					return nil
				}

				controllerRef := metav1.GetControllerOf(vm)
				if controllerRef == nil || controllerRef.Kind != "VirtualMachineReplicaSet" {
					return nil
				}

				return []reconcile.Request{
					{NamespacedName: types.NamespacedName{
						Namespace: vm.GetNamespace(),
						Name:      controllerRef.Name,
					}},
				}
			}),
		).
		Complete(r)
}

func (r *VMReplicaSetReconciler) calculateStatus(vmr *virtv1alpha1.VirtualMachineReplicaSet, vms []*virtv1alpha1.VirtualMachine, scaleErr error) virtv1alpha1.VirtualMachineReplicaSetStatus {

	readyReplicasCount := 0
	availableReplicasCount := 0
	newStatus := vmr.Status.DeepCopy()
	templateLabel := labels.Set(vmr.Spec.Template.Labels).AsSelectorPreValidated()
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

	failureCondition := getCondition(vmr.Status, virtv1alpha1.VirtualMachineReplicaSetReplicaFailure)
	if scaleErr != nil && failureCondition == nil {
		var reason string
		if diff := len(vms) - int(*(vmr.Spec.Replicas)); diff < 0 {
			reason = "FailedCreate"
		} else if diff > 0 {
			reason = "FailedDelete"
		}
		newStatus.Conditions = append(newStatus.Conditions, virtv1alpha1.VirtualMachineReplicaSetCondition{
			Type:               virtv1alpha1.VirtualMachineReplicaSetReplicaFailure,
			Status:             corev1.ConditionTrue,
			Reason:             reason,
			Message:            scaleErr.Error(),
			LastTransitionTime: metav1.Now(),
			LastProbeTime:      metav1.Now(),
		})
	} else if scaleErr == nil && failureCondition != nil {
		removeCondition(newStatus, virtv1alpha1.VirtualMachineReplicaSetReplicaFailure)
	}

	pauseCondition := getCondition(vmr.Status, virtv1alpha1.VirtualMachineReplicaSetPaused)
	if vmr.Spec.Paused && pauseCondition == nil {
		newStatus.Conditions = append(newStatus.Conditions, virtv1alpha1.VirtualMachineReplicaSetCondition{
			Type:               virtv1alpha1.VirtualMachineReplicaSetPaused,
			Status:             corev1.ConditionTrue,
			LastTransitionTime: metav1.Now(),
			LastProbeTime:      metav1.Now(),
			Reason:             "Paused",
			Message:            "VirtualMachineReplicaSet is paused",
		})
		r.Recorder.Eventf(vmr, corev1.EventTypeNormal, "Paused", "VirtualMachineReplicaSet is paused")
	} else if !vmr.Spec.Paused && pauseCondition != nil {
		removeCondition(newStatus, virtv1alpha1.VirtualMachineReplicaSetPaused)
		r.Recorder.Eventf(vmr, corev1.EventTypeNormal, "Resumed", "VirtualMachineReplicaSet is resumed")
	}

	newStatus.Replicas = int32(len(vms))
	newStatus.ReadyReplicas = int32(readyReplicasCount)
	newStatus.AvailableReplicas = int32(availableReplicasCount)

	return *newStatus
}

// Helper functions

func isVMReady(vm *virtv1alpha1.VirtualMachine) bool {
	return isVMActive(vm) && vm.Status.Phase == virtv1alpha1.VirtualMachineRunning &&
		meta.IsStatusConditionTrue(vm.Status.Conditions, string(virtv1alpha1.VirtualMachineReady))
}

func isVMActive(vm *virtv1alpha1.VirtualMachine) bool {
	return vm.Status.Phase != virtv1alpha1.VirtualMachineFailed &&
		vm.Status.Phase != virtv1alpha1.VirtualMachineSucceeded &&
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

func generateRandomSuffix() string {
	return strings.ToLower(rand.String(5))
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

func (r *VMReplicaSetReconciler) gcVMs(ctx context.Context, vmr *virtv1alpha1.VirtualMachineReplicaSet) error {
	// List all VMs controlled by this VMReplicaSet
	vms, err := r.getControlledVMs(ctx, vmr)
	if err != nil {
		return fmt.Errorf("failed to get controlled VMs: %v", err)
	}

	// If VMReplicaSet is being deleted, delete all VMs and remove finalizer when done
	if !vmr.DeletionTimestamp.IsZero() {
		// Check if there are any VMs still being deleted
		hasVMsBeingDeleted := false
		for _, vm := range vms {
			if vm.DeletionTimestamp != nil && !vm.DeletionTimestamp.IsZero() {
				hasVMsBeingDeleted = true
				break
			}
		}

		if len(vms) == 0 || (!hasVMsBeingDeleted && len(vms) == 0) {
			controllerutil.RemoveFinalizer(vmr, VMReplicaSetFinalizer)
			return r.Update(ctx, vmr)
		}

		// Delete all remaining VMs
		for _, vm := range vms {
			if vm.DeletionTimestamp != nil && !vm.DeletionTimestamp.IsZero() {
				continue
			}
			if err := r.DeleteVM(ctx, vm, vmr); err != nil {
				return fmt.Errorf("failed to delete VM %s: %v", vm.Name, err)
			}
			r.Recorder.Eventf(vmr, corev1.EventTypeNormal, "DeletedVM",
				"Deleted VM %q because VMReplicaSet is being deleted", vm.Name)
		}
		return &reconcileError{Result: ctrl.Result{RequeueAfter: deleteRequeueInterval}}
	}

	// Get selector for the VMReplicaSet
	selector, err := metav1.LabelSelectorAsSelector(vmr.Spec.Selector)
	if err != nil {
		return fmt.Errorf("failed to convert label selector: %w", err)
	}

	// Delete VMs that no longer match the selector
	for _, vm := range vms {
		if vm.DeletionTimestamp != nil && !vm.DeletionTimestamp.IsZero() {
			continue
		}

		if !selector.Matches(labels.Set(vm.Labels)) {
			if err := r.DeleteVM(ctx, vm, vmr); err != nil {
				return fmt.Errorf("failed to delete VM %s: %v", vm.Name, err)
			}
			r.Recorder.Eventf(vmr, corev1.EventTypeNormal, "DeletedVM",
				"Deleted VM %q that no longer matched selector", vm.Name)
		}
	}

	return nil
}

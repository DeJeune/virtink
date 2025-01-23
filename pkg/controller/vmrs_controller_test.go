package controller

import (
	"fmt"
	"time"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
	gomegaTypes "github.com/onsi/gomega/types"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	virtv1alpha1 "github.com/DeJeune/virtink/pkg/apis/virt/v1alpha1"
)

const (
	timeout  = time.Second * 10
	interval = time.Millisecond * 250
)

var _ = Describe("VMReplicaSet controller", func() {

	Context("when creating missing VMs", func() {
		var vmrKey types.NamespacedName
		var vmrs *virtv1alpha1.VirtualMachineReplicaSet

		BeforeEach(func() {
			vmrKey = types.NamespacedName{
				Name:      uuid.New().String(),
				Namespace: metav1.NamespaceDefault,
			}
			vmrs, _ = defaultReplicaSet(2, vmrKey)
			Expect(k8sClient.Create(ctx, vmrs)).To(Succeed())
		})

		It("should create VMs", func() {
			expectVMReplicas(vmrs, HaveLen(2))
		})

		AfterEach(func() {
			By("deleting the VMReplicaSet")
			Expect(k8sClient.Delete(ctx, vmrs)).To(Succeed())
		})
	})

	Context("when unpausing VMReplicaSet", func() {
		var vmrKey types.NamespacedName
		var vmrs *virtv1alpha1.VirtualMachineReplicaSet

		BeforeEach(func() {
			By("creating the VMReplicaSet in paused state")
			vmrKey = types.NamespacedName{
				Name:      uuid.New().String(),
				Namespace: metav1.NamespaceDefault,
			}
			vmrs, _ = defaultReplicaSet(2, vmrKey)
			vmrs.Spec.Paused = true
			Expect(k8sClient.Create(ctx, vmrs)).To(Succeed())

			// 等待 paused condition 被设置
			Eventually(func() bool {
				if err := k8sClient.Get(ctx, vmrKey, vmrs); err != nil {
					return false
				}
				return hasCondition(vmrs.Status, virtv1alpha1.VirtualMachineReplicaSetPaused)
			}, timeout, interval).Should(BeTrue())

			By("unpausing the VMReplicaSet")
			Expect(k8sClient.Get(ctx, vmrKey, vmrs)).To(Succeed())
			vmrs.Spec.Paused = false
			Expect(k8sClient.Update(ctx, vmrs)).To(Succeed())
		})

		AfterEach(func() {
			By("deleting the VMReplicaSet")
			Expect(k8sClient.Delete(ctx, vmrs)).To(Succeed())
		})

		It("should handle unpausing correctly", func() {
			By("creating expected number of VMs")
			expectVMReplicas(vmrs, HaveLen(2))

			By("updating replicas status")
			expectReplicasAndReadyReplicas(vmrKey.Name, 2, 0)

			By("removing paused condition")
			expectConditions(vmrKey, BeNil())
		})
	})

	Context("when VMReplicaSet is paused", func() {
		var vmrKey types.NamespacedName
		var vmrs *virtv1alpha1.VirtualMachineReplicaSet

		BeforeEach(func() {
			vmrKey = types.NamespacedName{
				Name:      uuid.New().String(),
				Namespace: metav1.NamespaceDefault,
			}
			vmrs, _ = defaultReplicaSet(2, vmrKey)
			vmrs.Spec.Paused = true
			vmrs.Status.Replicas = 1
			Expect(k8sClient.Create(ctx, vmrs)).To(Succeed())

			// 等待 paused condition 被设置
			Eventually(func() bool {
				if err := k8sClient.Get(ctx, vmrKey, vmrs); err != nil {
					return false
				}
				return hasCondition(vmrs.Status, virtv1alpha1.VirtualMachineReplicaSetPaused)
			}, timeout, interval).Should(BeTrue())
		})

		AfterEach(func() {
			By("deleting the VMReplicaSet")
			Expect(k8sClient.Delete(ctx, vmrs)).To(Succeed())
		})

		It("should not create VMs and add paused condition", func() {
			By("expecting no VMs")
			expectVMReplicas(vmrs, BeEmpty())

			By("expecting replicas and ready replicas")
			expectReplicasAndReadyReplicas(vmrKey.Name, 0, 0)

			By("expecting paused condition")
			expectConditions(vmrKey, ContainElement(
				MatchFields(IgnoreExtras, Fields{
					"Type":   Equal(virtv1alpha1.VirtualMachineReplicaSetPaused),
					"Status": Equal(corev1.ConditionTrue),
				}),
			))
		})
	})

	Context("when creating VMs in batches", func() {
		var vmrKey types.NamespacedName
		var vmrs *virtv1alpha1.VirtualMachineReplicaSet

		BeforeEach(func() {
			vmrKey = types.NamespacedName{
				Name:      uuid.New().String(),
				Namespace: metav1.NamespaceDefault,
			}
			vmrs, _ = defaultReplicaSet(15, vmrKey)
			Expect(k8sClient.Create(ctx, vmrs)).To(Succeed())
		})

		AfterEach(func() {
			By("deleting the VMReplicaSet")
			Expect(k8sClient.Delete(ctx, vmrs)).To(Succeed())
		})

		It("should create VMs", func() {
			expectVMReplicas(vmrs, HaveLen(15))
			expectReplicasAndReadyReplicas(vmrs.Name, 15, 0)
		})

		It("should scale down", func() {
			vmrs.Spec.Replicas = int32Ptr(10)
			Expect(k8sClient.Update(ctx, vmrs)).To(Succeed())
			expectVMReplicas(vmrs, HaveLen(10))
			expectReplicasAndReadyReplicas(vmrs.Name, 10, 0)
		})

		It("should scale up", func() {
			vmrs.Spec.Replicas = int32Ptr(20)
			Expect(k8sClient.Update(ctx, vmrs)).To(Succeed())
			expectVMReplicas(vmrs, HaveLen(20))
			expectReplicasAndReadyReplicas(vmrs.Name, 20, 0)
		})
	})

	Context("when handling non-matching VMIs", func() {
		var vmrKey types.NamespacedName
		var vmrs *virtv1alpha1.VirtualMachineReplicaSet
		var nonMatchingVM *virtv1alpha1.VirtualMachine

		BeforeEach(func() {
			vmrKey = types.NamespacedName{
				Name:      uuid.New().String(),
				Namespace: metav1.NamespaceDefault,
			}
			vmrs, _ = defaultReplicaSet(3, vmrKey)
			nonMatchingVM = &virtv1alpha1.VirtualMachine{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "non-matching-vm",
					Namespace: "default",
					Labels:    map[string]string{"test": "test1"},
				},
				Spec: virtv1alpha1.VirtualMachineSpec{
					RunPolicy: virtv1alpha1.RunPolicyManual,
				},
			}
			Expect(k8sClient.Create(ctx, vmrs)).To(Succeed())
			Expect(k8sClient.Create(ctx, nonMatchingVM)).To(Succeed())
		})

		AfterEach(func() {
			By("deleting the VMReplicaSet and non-matching VM")
			Expect(k8sClient.Delete(ctx, vmrs)).To(Succeed())
			Expect(k8sClient.Delete(ctx, nonMatchingVM)).To(Succeed())
		})

		It("should ignore non-matching VMIs", func() {
			expectVMReplicas(vmrs, HaveLen(3))
		})
	})

	Context("when deleting a VM instance", func() {
		var vmrKey types.NamespacedName
		var vmrs *virtv1alpha1.VirtualMachineReplicaSet
		var vm *virtv1alpha1.VirtualMachine

		BeforeEach(func() {
			vmrKey = types.NamespacedName{
				Name:      uuid.New().String(),
				Namespace: metav1.NamespaceDefault,
			}
			vmrs, vm = defaultReplicaSet(1, vmrKey)
			Expect(k8sClient.Create(ctx, vmrs)).To(Succeed())
			Expect(k8sClient.Create(ctx, vm)).To(Succeed())
			expectVMReplicas(vmrs, HaveLen(1))
			expectReplicasAndReadyReplicas(vmrs.Name, 1, 0)
			Expect(k8sClient.Delete(ctx, vm)).To(Succeed())
		})

		AfterEach(func() {
			By("cleaning up VMReplicaSet and VM")
			Expect(k8sClient.Delete(ctx, vmrs)).To(Succeed())
		})

		It("should recreate the VM", func() {
			By("waiting for VM to be recreated")
			expectVMReplicas(vmrs, HaveLen(1))
			expectReplicasAndReadyReplicas(vmrs.Name, 1, 0)
		})
	})

})

func int32Ptr(i int32) *int32 {
	return &i
}

func replicaSetFromVM(vmrKey types.NamespacedName, vm *virtv1alpha1.VirtualMachine, replicas int32) *virtv1alpha1.VirtualMachineReplicaSet {

	vmrs := &virtv1alpha1.VirtualMachineReplicaSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      vmrKey.Name,
			Namespace: vm.Namespace,
		},
		Spec: virtv1alpha1.VirtualMachineReplicaSetSpec{
			Replicas: int32Ptr(replicas),
			Selector: &metav1.LabelSelector{
				MatchLabels: vm.ObjectMeta.Labels,
			},
			Template: &virtv1alpha1.VirtualMachineTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Name:   vm.ObjectMeta.Name,
					Labels: vm.ObjectMeta.Labels,
				},
				Spec: vm.Spec,
			},
		},
	}
	return vmrs
}

func defaultReplicaSet(replicas int32, vmrKey types.NamespacedName) (*virtv1alpha1.VirtualMachineReplicaSet, *virtv1alpha1.VirtualMachine) {
	vm := &virtv1alpha1.VirtualMachine{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: vmrKey.Namespace,
			Name:      fmt.Sprintf("%s-template", vmrKey.Name),
			Labels: map[string]string{
				"test": "test",
			},
		},
		Spec: virtv1alpha1.VirtualMachineSpec{
			RunPolicy: virtv1alpha1.RunPolicyManual,
		},
	}
	vmrs := replicaSetFromVM(vmrKey, vm, replicas)

	return vmrs, vm
}

// 基础的获取函数
func getVMR(name string) (*virtv1alpha1.VirtualMachineReplicaSet, error) {
	var vmrs virtv1alpha1.VirtualMachineReplicaSet
	vmrKey := types.NamespacedName{Name: name, Namespace: "default"}
	err := k8sClient.Get(ctx, vmrKey, &vmrs)
	return &vmrs, err
}

// 期望状态检查函数
func expectVMRStatus(vmrs *virtv1alpha1.VirtualMachineReplicaSet, replicas, readyReplicas int) error {
	if vmrs.Status.Replicas != int32(replicas) {
		return fmt.Errorf("expected replicas %d but got %d", replicas, vmrs.Status.Replicas)
	}
	if vmrs.Status.ReadyReplicas != int32(readyReplicas) {
		return fmt.Errorf("expected ready replicas %d but got %d", readyReplicas, vmrs.Status.ReadyReplicas)
	}
	return nil
}

// 组合使用
func expectReplicasAndReadyReplicas(replicaName string, replicas, readyReplicas int) {
	Eventually(func() error {
		vmrs, err := getVMR(replicaName)
		if err != nil {
			return err
		}
		return expectVMRStatus(vmrs, replicas, readyReplicas)
	}, timeout, interval).Should(Succeed())
}

// 基础的条件检查函数
func checkConditions(conditions []virtv1alpha1.VirtualMachineReplicaSetCondition, matcher gomegaTypes.GomegaMatcher) error {
	success, err := matcher.Match(conditions)
	if err != nil {
		return fmt.Errorf("error matching conditions: %v", err)
	}
	if !success {
		return fmt.Errorf("conditions did not match: %v", conditions)
	}
	return nil
}

func expectConditions(vmrKey types.NamespacedName, matcher gomegaTypes.GomegaMatcher) {
	Eventually(func() error {
		vmrs, err := getVMR(vmrKey.Name)
		if err != nil {
			return err
		}
		return checkConditions(vmrs.Status.Conditions, matcher)
	}, timeout, interval).Should(Succeed())
}

func expectVMReplicas(vmrs *virtv1alpha1.VirtualMachineReplicaSet, matcher gomegaTypes.GomegaMatcher) {
	Eventually(func() []virtv1alpha1.VirtualMachine {
		var vmiList virtv1alpha1.VirtualMachineList
		Expect(k8sClient.List(ctx, &vmiList)).To(Succeed())
		var rsVMs []virtv1alpha1.VirtualMachine
		for _, vm := range vmiList.Items {
			for _, or := range vm.OwnerReferences {
				if equality.Semantic.DeepEqual(or, OwnerRef(vmrs)) {
					rsVMs = append(rsVMs, vm)
					break
				}
			}
		}
		return rsVMs
	}, timeout, interval).Should(matcher)
}

package controller

import (
	"github.com/google/uuid"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"time"

	virtv1alpha1 "github.com/smartxworks/virtink/pkg/apis/virt/v1alpha1"
)

var _ = Describe("VMReplicaSet controller", func() {
	Context("for a new VMReplicaSet", func() {
		var vmrKey types.NamespacedName

		BeforeEach(func() {
			By("creating a new VMReplicaSet")
			vmrKey = types.NamespacedName{
				Name:      uuid.New().String(),
				Namespace: "default",
			}
			vmr := virtv1alpha1.VirtualMachineReplicaSet{
				ObjectMeta: metav1.ObjectMeta{
					Name:      vmrKey.Name,
					Namespace: vmrKey.Namespace,
				},
				Spec: virtv1alpha1.VirtualMachineReplicaSetSpec{
					Replicas: int32Ptr(2),
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{
							"test-vmr": vmrKey.Name,
						},
					},
					Template: virtv1alpha1.VirtualMachineTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Labels: map[string]string{
								"test-vmr": vmrKey.Name,
							},
						},
						Spec: virtv1alpha1.VirtualMachineSpec{
							RunPolicy: virtv1alpha1.RunPolicyManual,
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, &vmr)).To(Succeed())
		})

		It("should add finalizer", func() {
			Eventually(func() bool {
				var vmr virtv1alpha1.VirtualMachineReplicaSet
				Expect(k8sClient.Get(ctx, vmrKey, &vmr)).To(Succeed())
				return containsFinalizer(&vmr, VMReplicaSetFinalizer)
			}).Should(BeTrue())
		})

		It("should create VMs", func() {
			Eventually(func() bool {
				var vmList virtv1alpha1.VirtualMachineList
				Expect(k8sClient.List(ctx, &vmList, client.InNamespace(vmrKey.Namespace))).To(Succeed())

				matchingVMs := 0
				for _, vm := range vmList.Items {
					if vm.Labels["test-vmr"] == vmrKey.Name {
						matchingVMs++
					}
				}
				return matchingVMs == 2
			}, 30*time.Second, time.Second).Should(BeTrue())
		})

		It("should update status", func() {
			Eventually(func() bool {
				var vmr virtv1alpha1.VirtualMachineReplicaSet
				Expect(k8sClient.Get(ctx, vmrKey, &vmr)).To(Succeed())
				return vmr.Status.Replicas == 2
			}, 30*time.Second, time.Second).Should(BeTrue())
		})
	})

	Context("when scaling VMReplicaSet", func() {
		var vmrKey types.NamespacedName

		BeforeEach(func() {
			By("creating a new VMReplicaSet")
			vmrKey = types.NamespacedName{
				Name:      uuid.New().String(),
				Namespace: "default",
			}
			vmr := virtv1alpha1.VirtualMachineReplicaSet{
				ObjectMeta: metav1.ObjectMeta{
					Name:      vmrKey.Name,
					Namespace: vmrKey.Namespace,
				},
				Spec: virtv1alpha1.VirtualMachineReplicaSetSpec{
					Replicas: int32Ptr(1),
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{
							"test-vmr": vmrKey.Name,
						},
					},
					Template: virtv1alpha1.VirtualMachineTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Labels: map[string]string{
								"test-vmr": vmrKey.Name,
							},
						},
						Spec: virtv1alpha1.VirtualMachineSpec{
							RunPolicy: virtv1alpha1.RunPolicyManual,
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, &vmr)).To(Succeed())

			By("waiting for initial VM to be created")
			Eventually(func() bool {
				var vmList virtv1alpha1.VirtualMachineList
				Expect(k8sClient.List(ctx, &vmList, client.InNamespace(vmrKey.Namespace))).To(Succeed())

				matchingVMs := 0
				for _, vm := range vmList.Items {
					if vm.Labels["test-vmr"] == vmrKey.Name {
						matchingVMs++
					}
				}
				return matchingVMs == 1
			}, 30*time.Second, time.Second).Should(BeTrue())
		})

		It("should scale up when replicas increased", func() {
			By("increasing replicas to 3")
			Eventually(func() error {
				var vmr virtv1alpha1.VirtualMachineReplicaSet
				if err := k8sClient.Get(ctx, vmrKey, &vmr); err != nil {
					return err
				}
				vmr.Spec.Replicas = int32Ptr(3)
				return k8sClient.Update(ctx, &vmr)
			}, 30*time.Second, time.Second).Should(Succeed())

			By("checking that new VMs are created")
			Eventually(func() bool {
				var vmList virtv1alpha1.VirtualMachineList
				Expect(k8sClient.List(ctx, &vmList, client.InNamespace(vmrKey.Namespace))).To(Succeed())

				matchingVMs := 0
				for _, vm := range vmList.Items {
					if vm.Labels["test-vmr"] == vmrKey.Name {
						matchingVMs++
					}
				}
				return matchingVMs == 3
			}, 30*time.Second, time.Second).Should(BeTrue())
		})

		It("should scale down when replicas decreased", func() {
			By("first scaling up to 3")
			Eventually(func() error {
				var vmr virtv1alpha1.VirtualMachineReplicaSet
				if err := k8sClient.Get(ctx, vmrKey, &vmr); err != nil {
					return err
				}
				vmr.Spec.Replicas = int32Ptr(3)
				return k8sClient.Update(ctx, &vmr)
			}, 30*time.Second, time.Second).Should(Succeed())

			By("waiting for scale up to complete")
			Eventually(func() bool {
				var vmList virtv1alpha1.VirtualMachineList
				Expect(k8sClient.List(ctx, &vmList, client.InNamespace(vmrKey.Namespace))).To(Succeed())

				matchingVMs := 0
				for _, vm := range vmList.Items {
					if vm.Labels["test-vmr"] == vmrKey.Name {
						matchingVMs++
					}
				}
				return matchingVMs == 3
			}, 30*time.Second, time.Second).Should(BeTrue())

			By("decreasing replicas to 1")
			Eventually(func() error {
				var vmr virtv1alpha1.VirtualMachineReplicaSet
				if err := k8sClient.Get(ctx, vmrKey, &vmr); err != nil {
					return err
				}
				vmr.Spec.Replicas = int32Ptr(1)
				return k8sClient.Update(ctx, &vmr)
			}, 30*time.Second, time.Second).Should(Succeed())

			By("checking that VMs are removed")
			Eventually(func() bool {
				var vmList virtv1alpha1.VirtualMachineList
				Expect(k8sClient.List(ctx, &vmList, client.InNamespace(vmrKey.Namespace))).To(Succeed())

				matchingVMs := 0
				for _, vm := range vmList.Items {
					if vm.Labels["test-vmr"] == vmrKey.Name {
						matchingVMs++
					}
				}
				return matchingVMs == 1
			}, 30*time.Second, time.Second).Should(BeTrue())
		})
	})

	Context("when deleting VMReplicaSet", func() {
		var vmrKey types.NamespacedName

		BeforeEach(func() {
			By("creating a new VMReplicaSet")
			vmrKey = types.NamespacedName{
				Name:      uuid.New().String(),
				Namespace: "default",
			}
			vmr := virtv1alpha1.VirtualMachineReplicaSet{
				ObjectMeta: metav1.ObjectMeta{
					Name:      vmrKey.Name,
					Namespace: vmrKey.Namespace,
				},
				Spec: virtv1alpha1.VirtualMachineReplicaSetSpec{
					Replicas: int32Ptr(2),
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{
							"test-vmr": vmrKey.Name,
						},
					},
					Template: virtv1alpha1.VirtualMachineTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Labels: map[string]string{
								"test-vmr": vmrKey.Name,
							},
						},
						Spec: virtv1alpha1.VirtualMachineSpec{
							RunPolicy: virtv1alpha1.RunPolicyManual,
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, &vmr)).To(Succeed())

			By("waiting for VMs to be created")
			Eventually(func() bool {
				var vmList virtv1alpha1.VirtualMachineList
				Expect(k8sClient.List(ctx, &vmList, client.InNamespace(vmrKey.Namespace))).To(Succeed())

				matchingVMs := 0
				for _, vm := range vmList.Items {
					if vm.Labels["test-vmr"] == vmrKey.Name {
						matchingVMs++
					}
				}
				return matchingVMs == 2
			}, 30*time.Second, time.Second).Should(BeTrue())
		})

		It("should delete all VMs and remove finalizer", func() {
			By("deleting the VMReplicaSet")
			var vmr virtv1alpha1.VirtualMachineReplicaSet
			Expect(k8sClient.Get(ctx, vmrKey, &vmr)).To(Succeed())
			Expect(k8sClient.Delete(ctx, &vmr)).To(Succeed())

			By("checking that all VMs are deleted")
			Eventually(func() bool {
				var vmList virtv1alpha1.VirtualMachineList
				Expect(k8sClient.List(ctx, &vmList, client.InNamespace(vmrKey.Namespace))).To(Succeed())

				for _, vm := range vmList.Items {
					if vm.Labels["test-vmr"] == vmrKey.Name {
						return false
					}
				}
				return true
			}, 30*time.Second, time.Second).Should(BeTrue())

			By("checking that VMReplicaSet is deleted")
			Eventually(func() error {
				var vmr virtv1alpha1.VirtualMachineReplicaSet
				return k8sClient.Get(ctx, vmrKey, &vmr)
			}, 30*time.Second, time.Second).Should(MatchError(ContainSubstring("not found")))
		})
	})
})

func int32Ptr(i int32) *int32 {
	return &i
}

func containsFinalizer(obj metav1.Object, finalizer string) bool {
	for _, f := range obj.GetFinalizers() {
		if f == finalizer {
			return true
		}
	}
	return false
}

/*
Copyright 2025.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controller

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	helloworldv1 "github.com/rvolykh/helloworld-operator/api/v1"
	"github.com/rvolykh/helloworld-operator/internal/controller/fake"
)

var _ = Describe("CopyToPod Controller", func() {
	Context("When reconciling a resource", func() {
		const (
			namespace     = "default"
			containerName = "test-nginx"
		)
		var (
			ctx = context.Background()

			resourceTypeNamespacedName = types.NamespacedName{
				Name:      "test-resource",
				Namespace: namespace,
			}
			podTypeNamespacedName = types.NamespacedName{
				Name:      "test-pod",
				Namespace: namespace,
			}

			copytopod = &helloworldv1.CopyToPod{}
			pod       = &corev1.Pod{}
		)

		BeforeEach(func() {
			By("creating the dummy resource for the Kind Pod")
			err := k8sClient.Get(ctx, podTypeNamespacedName, pod)
			if err != nil && errors.IsNotFound(err) {
				pod = &corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Name:      podTypeNamespacedName.Name,
						Namespace: podTypeNamespacedName.Namespace,
					},
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{
							{
								Name:  containerName,
								Image: "nginx:latest",
							},
						},
					},
				}
				Expect(k8sClient.Create(ctx, pod)).To(Succeed())
			}

			By("creating the custom resource for the Kind CopyToPod")
			err = k8sClient.Get(ctx, resourceTypeNamespacedName, copytopod)
			if err != nil && errors.IsNotFound(err) {
				copytopod = &helloworldv1.CopyToPod{
					ObjectMeta: metav1.ObjectMeta{
						Name:      resourceTypeNamespacedName.Name,
						Namespace: resourceTypeNamespacedName.Namespace,
					},
					Spec: helloworldv1.CopyToPodSpec{
						PodName:       podTypeNamespacedName.Name,
						ContainerName: containerName,
						FileName:      "/usr/share/nginx/html/index.html",
						Content:       "Hello World",
					},
				}
				Expect(k8sClient.Create(ctx, copytopod)).To(Succeed())
			}
		})

		AfterEach(func() {
			if err := k8sClient.Get(ctx, podTypeNamespacedName, pod); err == nil {
				By("Removing finalizers")
				Eventually(func() error {
					pod.SetFinalizers(nil)
					return k8sClient.Update(ctx, pod)
				}).Should(Succeed())

				By("Cleanup the specific resource instance Pod")
				Expect(k8sClient.Delete(ctx, pod)).To(Succeed())
			}

			if err := k8sClient.Get(ctx, resourceTypeNamespacedName, copytopod); err == nil {
				By("Removing finalizers")
				Eventually(func() error {
					copytopod.SetFinalizers(nil)
					return k8sClient.Update(ctx, copytopod)
				}).Should(Succeed())

				By("Cleanup the specific resource instance CopyToPod")
				Expect(k8sClient.Delete(ctx, copytopod)).To(Succeed())
			}
		})

		It("should successfully reconcile the resource", func() {
			By("Reconciling the created resource")
			controllerReconciler := &CopyToPodReconciler{
				Client:     k8sClient,
				Scheme:     k8sClient.Scheme(),
				PodCopyCmd: &fake.FakePodCopyCmd{},
			}

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: resourceTypeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			By("Checking that the resource has been reconciled")
			Expect(k8sClient.Get(ctx, resourceTypeNamespacedName, copytopod)).To(Succeed())
			cond := meta.FindStatusCondition(copytopod.Status.Conditions, "Available")
			Expect(cond).NotTo(BeNil())
			Expect(copytopod.Status.Conditions).To(HaveLen(1))
			Expect(cond.Reason).To(Equal("Completed"))
			Expect(cond.Message).To(Equal("File copied successfully"))

			By("Checking that Pod as controlled by the CopyToPod resource")
			Expect(k8sClient.Get(ctx, podTypeNamespacedName, pod)).To(Succeed())
			refs := pod.GetOwnerReferences()
			Expect(refs).To(HaveLen(1))
			Expect(refs[0].UID).To(Equal(copytopod.GetUID()))
		})

		It("should re-attempt fail to reconcile the resource", func() {
			By("Reconciling the created resource")
			controllerReconciler := &CopyToPodReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
				PodCopyCmd: &fake.FakePodCopyCmd{
					Err: errors.NewTooManyRequestsError("Expected"),
				},
			}
			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: resourceTypeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			By("Checking that the resource has been reconciled")
			Expect(k8sClient.Get(ctx, resourceTypeNamespacedName, copytopod)).To(Succeed())
			cond := meta.FindStatusCondition(copytopod.Status.Conditions, "Available")
			Expect(cond).NotTo(BeNil())
			Expect(copytopod.Status.Conditions).To(HaveLen(1))
			Expect(cond.Reason).To(Equal("Reconciling"))
			Expect(cond.Message).To(Equal("Too many requests: Expected"))

			By("Checking that Pod is controlled by the CopyToPod resource")
			Expect(k8sClient.Get(ctx, podTypeNamespacedName, pod)).To(Succeed())
			refs := pod.GetOwnerReferences()
			Expect(refs).To(HaveLen(1))
			Expect(refs[0].UID).To(Equal(copytopod.GetUID()))
		})

		It("should handle Pod removal", func() {
			By("Reconciling the created resource")
			controllerReconciler := &CopyToPodReconciler{
				Client:     k8sClient,
				Scheme:     k8sClient.Scheme(),
				PodCopyCmd: &fake.FakePodCopyCmd{},
			}
			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: resourceTypeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			By("Deleting the Pod")
			Expect(k8sClient.Delete(ctx, pod)).To(Succeed())

			By("Reconciling the deleted resource")
			_, err = controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: resourceTypeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			By("Checking that Pod has been deleted")
			Expect(k8sClient.Get(ctx, podTypeNamespacedName, pod)).NotTo(Succeed())
		})

		It("should handle CopyToPod removal", func() {
			By("Reconciling the created resource")
			controllerReconciler := &CopyToPodReconciler{
				Client:     k8sClient,
				Scheme:     k8sClient.Scheme(),
				PodCopyCmd: &fake.FakePodCopyCmd{},
			}
			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: resourceTypeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			By("Deleting the Pod")
			Expect(k8sClient.Delete(ctx, copytopod)).To(Succeed())

			By("Reconciling the deleted resource")
			_, err = controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: resourceTypeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			By("Checking that CopyToPod has been deleted")
			Expect(k8sClient.Get(ctx, resourceTypeNamespacedName, copytopod)).NotTo(Succeed())

			By("Checking that Pod is not controlled by the CopyToPod resource")
			Expect(k8sClient.Get(ctx, podTypeNamespacedName, pod)).To(Succeed())
			refs := pod.GetOwnerReferences()
			Expect(refs).To(BeEmpty())
		})
	})
})

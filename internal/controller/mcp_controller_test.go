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
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	helloworldv1 "github.com/rvolykh/helloworld-operator/api/v1"
)

var _ = Describe("MCP Controller", func() {
	var (
		ctx                  context.Context
		controllerReconciler *MCPReconciler
		testCounter          int
	)

	BeforeEach(func() {
		ctx = context.Background()
		controllerReconciler = &MCPReconciler{
			Client:   k8sClient,
			Scheme:   k8sClient.Scheme(),
			Recorder: record.NewFakeRecorder(100),
		}
		testCounter++
	})

	Context("When reconciling an Internal MCP", func() {
		const testImage = "test/mcp-image:latest"

		var (
			resourceName       string
			typeNamespacedName types.NamespacedName
			mcp                *helloworldv1.MCP
		)

		BeforeEach(func() {
			resourceName = fmt.Sprintf("test-internal-mcp-%d", testCounter)
			typeNamespacedName = types.NamespacedName{
				Name:      resourceName,
				Namespace: "default",
			}

			// Create MCP resource with internal type
			mcpType := helloworldv1.Internal
			mcp = &helloworldv1.MCP{
				ObjectMeta: metav1.ObjectMeta{
					Name:      resourceName,
					Namespace: "default",
				},
				Spec: helloworldv1.MCPSpec{
					Type: &mcpType,
					Internal: &helloworldv1.InternalMCPSpec{
						Image: testImage,
					},
				},
			}
			Expect(k8sClient.Create(ctx, mcp)).To(Succeed())
		})

		AfterEach(func() {
			// Clean up resources
			resource := &helloworldv1.MCP{}
			if err := k8sClient.Get(ctx, typeNamespacedName, resource); err == nil {
				// Remove finalizer first to allow deletion
				resource.Finalizers = []string{}
				Expect(k8sClient.Update(ctx, resource)).To(Succeed())

				Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
				// Wait for deletion to complete
				Eventually(func() bool {
					err := k8sClient.Get(ctx, typeNamespacedName, resource)
					return errors.IsNotFound(err)
				}, time.Second*15, time.Millisecond*250).Should(BeTrue())
			}

			// Clean up any leftover deployment
			deployment := &appsv1.Deployment{}
			if err := k8sClient.Get(ctx, typeNamespacedName, deployment); err == nil {
				Expect(k8sClient.Delete(ctx, deployment)).To(Succeed())
			}

			// Clean up any leftover service
			service := &corev1.Service{}
			if err := k8sClient.Get(ctx, typeNamespacedName, service); err == nil {
				Expect(k8sClient.Delete(ctx, service)).To(Succeed())
			}
		})

		It("should create a deployment and service", func() {
			By("Reconciling the created resource")
			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			By("Checking that the finalizer is added")
			Eventually(func() bool {
				if err := k8sClient.Get(ctx, typeNamespacedName, mcp); err != nil {
					return false
				}
				for _, finalizer := range mcp.Finalizers {
					if finalizer == mcpFinalizer {
						return true
					}
				}
				return false
			}, time.Second*10, time.Millisecond*250).Should(BeTrue())

			By("Checking that a deployment is created")
			deployment := &appsv1.Deployment{}
			Eventually(func() error {
				return k8sClient.Get(ctx, typeNamespacedName, deployment)
			}, time.Second*10, time.Millisecond*250).Should(Succeed())

			// Verify deployment spec
			Expect(deployment.Spec.Replicas).To(Equal(func() *int32 { r := int32(1); return &r }()))
			Expect(deployment.Spec.Selector.MatchLabels).To(Equal(map[string]string{"app": resourceName}))
			Expect(deployment.Spec.Template.Labels).To(Equal(map[string]string{"app": resourceName}))
			Expect(deployment.Spec.Template.Spec.Containers).To(HaveLen(1))
			Expect(deployment.Spec.Template.Spec.Containers[0].Name).To(Equal(resourceName))
			Expect(deployment.Spec.Template.Spec.Containers[0].Image).To(Equal(testImage))
			Expect(deployment.Spec.Template.Spec.Containers[0].Ports[0].ContainerPort).To(Equal(int32(8080)))

			By("Checking that a service is created")
			service := &corev1.Service{}
			Eventually(func() error {
				return k8sClient.Get(ctx, typeNamespacedName, service)
			}, time.Second*10, time.Millisecond*250).Should(Succeed())

			// Verify service spec
			Expect(service.Spec.Type).To(Equal(corev1.ServiceTypeClusterIP))
			Expect(service.Spec.Selector).To(Equal(map[string]string{"app": resourceName}))
			Expect(service.Spec.Ports).To(HaveLen(1))
			Expect(service.Spec.Ports[0].Port).To(Equal(int32(8080)))
			Expect(service.Spec.Ports[0].TargetPort).To(Equal(intstr.FromInt(8080)))
		})

		It("should handle deployment updates", func() {
			By("Initial reconciliation")
			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			By("Updating the MCP image")
			Eventually(func() error {
				if err := k8sClient.Get(ctx, typeNamespacedName, mcp); err != nil {
					return err
				}
				mcp.Spec.Internal.Image = "updated/image:v2"
				return k8sClient.Update(ctx, mcp)
			}, time.Second*10, time.Millisecond*250).Should(Succeed())

			By("Reconciling after update")
			_, err = controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			By("Verifying deployment is updated")
			deployment := &appsv1.Deployment{}
			Eventually(func() string {
				if err := k8sClient.Get(ctx, typeNamespacedName, deployment); err != nil {
					return ""
				}
				if len(deployment.Spec.Template.Spec.Containers) == 0 {
					return ""
				}
				return deployment.Spec.Template.Spec.Containers[0].Image
			}, time.Second*10, time.Millisecond*250).Should(Equal("updated/image:v2"))
		})
	})

	Context("When reconciling an External MCP", func() {
		const testURL = "external-mcp.example.com" // Valid domain name without protocol

		var (
			resourceName       string
			typeNamespacedName types.NamespacedName
			mcp                *helloworldv1.MCP
		)

		BeforeEach(func() {
			resourceName = fmt.Sprintf("test-external-mcp-%d", testCounter)
			typeNamespacedName = types.NamespacedName{
				Name:      resourceName,
				Namespace: "default",
			}

			// Create MCP resource with external type
			mcpType := helloworldv1.External
			mcp = &helloworldv1.MCP{
				ObjectMeta: metav1.ObjectMeta{
					Name:      resourceName,
					Namespace: "default",
				},
				Spec: helloworldv1.MCPSpec{
					Type: &mcpType,
					External: &helloworldv1.ExternalMCPSpec{
						URL: testURL,
					},
				},
			}
			Expect(k8sClient.Create(ctx, mcp)).To(Succeed())
		})

		AfterEach(func() {
			// Clean up resources
			resource := &helloworldv1.MCP{}
			if err := k8sClient.Get(ctx, typeNamespacedName, resource); err == nil {
				// Remove finalizer first to allow deletion
				resource.Finalizers = []string{}
				Expect(k8sClient.Update(ctx, resource)).To(Succeed())

				Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
				// Wait for deletion to complete
				Eventually(func() bool {
					err := k8sClient.Get(ctx, typeNamespacedName, resource)
					return errors.IsNotFound(err)
				}, time.Second*15, time.Millisecond*250).Should(BeTrue())
			}

			// Clean up any leftover service
			service := &corev1.Service{}
			if err := k8sClient.Get(ctx, typeNamespacedName, service); err == nil {
				Expect(k8sClient.Delete(ctx, service)).To(Succeed())
			}
		})

		It("should create an external service", func() {
			By("Reconciling the created resource")
			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			By("Checking that a deployment is NOT created")
			deployment := &appsv1.Deployment{}
			Consistently(func() bool {
				err := k8sClient.Get(ctx, typeNamespacedName, deployment)
				return errors.IsNotFound(err)
			}, time.Second*2, time.Millisecond*250).Should(BeTrue())

			By("Checking that an external service is created")
			service := &corev1.Service{}
			Eventually(func() error {
				return k8sClient.Get(ctx, typeNamespacedName, service)
			}, time.Second*10, time.Millisecond*250).Should(Succeed())

			// Verify external service spec
			Expect(service.Spec.Type).To(Equal(corev1.ServiceTypeExternalName))
			Expect(service.Spec.ExternalName).To(Equal(testURL))
			Expect(service.Spec.Selector).To(BeNil())
		})
	})

	Context("When deleting an MCP", func() {
		const testImage = "test/mcp-image:latest"

		var (
			resourceName       string
			typeNamespacedName types.NamespacedName
			mcp                *helloworldv1.MCP
		)

		BeforeEach(func() {
			resourceName = fmt.Sprintf("test-deletion-mcp-%d", testCounter)
			typeNamespacedName = types.NamespacedName{
				Name:      resourceName,
				Namespace: "default",
			}

			// Create MCP resource
			mcpType := helloworldv1.Internal
			mcp = &helloworldv1.MCP{
				ObjectMeta: metav1.ObjectMeta{
					Name:      resourceName,
					Namespace: "default",
				},
				Spec: helloworldv1.MCPSpec{
					Type: &mcpType,
					Internal: &helloworldv1.InternalMCPSpec{
						Image: testImage,
					},
				},
			}
			Expect(k8sClient.Create(ctx, mcp)).To(Succeed())

			// Initial reconciliation to create resources
			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())
		})

		It("should clean up all resources when deleted", func() {
			By("Verifying resources exist before deletion")
			deployment := &appsv1.Deployment{}
			Expect(k8sClient.Get(ctx, typeNamespacedName, deployment)).To(Succeed())

			service := &corev1.Service{}
			Expect(k8sClient.Get(ctx, typeNamespacedName, service)).To(Succeed())

			By("Deleting the MCP resource")
			Expect(k8sClient.Delete(ctx, mcp)).To(Succeed())

			By("Reconciling after deletion")
			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			// The controller may return a ResourceExpired error during deletion, which is expected
			if err != nil {
				Expected := errors.IsResourceExpired(err)
				Expect(Expected).To(BeTrue(), "Expected ResourceExpired error during deletion, got: %v", err)
			}

			By("Verifying all resources are cleaned up")
			Eventually(func() bool {
				err := k8sClient.Get(ctx, typeNamespacedName, mcp)
				return errors.IsNotFound(err)
			}, time.Second*15, time.Millisecond*250).Should(BeTrue())

			Eventually(func() bool {
				err := k8sClient.Get(ctx, typeNamespacedName, deployment)
				return errors.IsNotFound(err)
			}, time.Second*15, time.Millisecond*250).Should(BeTrue())

			Eventually(func() bool {
				err := k8sClient.Get(ctx, typeNamespacedName, service)
				return errors.IsNotFound(err)
			}, time.Second*15, time.Millisecond*250).Should(BeTrue())
		})
	})

	Context("When reconciling a resource that doesn't exist", func() {
		It("should not return an error", func() {
			By("Reconciling a non-existent resource")
			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      "non-existent",
					Namespace: "default",
				},
			})
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Context("When testing status conditions", func() {
		const testImage = "test/mcp-image:latest"

		var (
			resourceName       string
			typeNamespacedName types.NamespacedName
			mcp                *helloworldv1.MCP
		)

		BeforeEach(func() {
			resourceName = fmt.Sprintf("test-status-mcp-%d", testCounter)
			typeNamespacedName = types.NamespacedName{
				Name:      resourceName,
				Namespace: "default",
			}

			// Create MCP resource with internal type
			mcpType := helloworldv1.Internal
			mcp = &helloworldv1.MCP{
				ObjectMeta: metav1.ObjectMeta{
					Name:      resourceName,
					Namespace: "default",
				},
				Spec: helloworldv1.MCPSpec{
					Type: &mcpType,
					Internal: &helloworldv1.InternalMCPSpec{
						Image: testImage,
					},
				},
			}
			Expect(k8sClient.Create(ctx, mcp)).To(Succeed())
		})

		AfterEach(func() {
			// Clean up resources
			resource := &helloworldv1.MCP{}
			if err := k8sClient.Get(ctx, typeNamespacedName, resource); err == nil {
				// Remove finalizer first to allow deletion
				resource.Finalizers = []string{}
				Expect(k8sClient.Update(ctx, resource)).To(Succeed())

				Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
				// Wait for deletion to complete
				Eventually(func() bool {
					err := k8sClient.Get(ctx, typeNamespacedName, resource)
					return errors.IsNotFound(err)
				}, time.Second*15, time.Millisecond*250).Should(BeTrue())
			}

			// Clean up any leftover deployment
			deployment := &appsv1.Deployment{}
			if err := k8sClient.Get(ctx, typeNamespacedName, deployment); err == nil {
				Expect(k8sClient.Delete(ctx, deployment)).To(Succeed())
			}

			// Clean up any leftover service
			service := &corev1.Service{}
			if err := k8sClient.Get(ctx, typeNamespacedName, service); err == nil {
				Expect(k8sClient.Delete(ctx, service)).To(Succeed())
			}
		})

		It("should set Progressing condition when deployment is being created", func() {
			By("Reconciling the created resource")
			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			By("Checking that Progressing condition is set")
			Eventually(func() bool {
				if err := k8sClient.Get(ctx, typeNamespacedName, mcp); err != nil {
					return false
				}

				// Find the progressing condition
				for _, condition := range mcp.Status.Conditions {
					if condition.Type == "Progressing" && condition.Status == metav1.ConditionTrue {
						return condition.Reason == "DeploymentStatus" &&
							condition.Message != ""
					}
				}
				return false
			}, time.Second*10, time.Millisecond*250).Should(BeTrue())

			By("Checking that MCPInfo URL is set correctly")
			Eventually(func() bool {
				if err := k8sClient.Get(ctx, typeNamespacedName, mcp); err != nil {
					return false
				}

				expectedURL := fmt.Sprintf("http://%s:8080", resourceName)
				return mcp.Status.Info != nil && mcp.Status.Info.URL == expectedURL
			}, time.Second*10, time.Millisecond*250).Should(BeTrue())
		})

		It("should transition from Progressing to Available condition", func() {
			By("Initial reconciliation")
			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			By("Verifying initial Progressing condition")
			Eventually(func() bool {
				if err := k8sClient.Get(ctx, typeNamespacedName, mcp); err != nil {
					return false
				}

				for _, condition := range mcp.Status.Conditions {
					if condition.Type == "Progressing" && condition.Status == metav1.ConditionTrue {
						return true
					}
				}
				return false
			}, time.Second*10, time.Millisecond*250).Should(BeTrue())

			By("Simulating deployment becoming available")
			deployment := &appsv1.Deployment{}
			Eventually(func() error {
				return k8sClient.Get(ctx, typeNamespacedName, deployment)
			}, time.Second*10, time.Millisecond*250).Should(Succeed())

			// Update deployment status to simulate it becoming available
			deployment.Status.Conditions = []appsv1.DeploymentCondition{
				{
					Type:               appsv1.DeploymentAvailable,
					Status:             corev1.ConditionTrue,
					LastUpdateTime:     metav1.Now(),
					LastTransitionTime: metav1.Now(),
					Reason:             "MinimumReplicasAvailable",
					Message:            "Deployment has minimum availability",
				},
			}
			deployment.Status.Replicas = 1
			deployment.Status.ReadyReplicas = 1
			deployment.Status.AvailableReplicas = 1
			Expect(k8sClient.Status().Update(ctx, deployment)).To(Succeed())

			By("Reconciling again to update status")
			_, err = controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			By("Verifying Available condition is set")
			Eventually(func() bool {
				if err := k8sClient.Get(ctx, typeNamespacedName, mcp); err != nil {
					return false
				}

				for _, condition := range mcp.Status.Conditions {
					if condition.Type == "Available" &&
						condition.Status == metav1.ConditionTrue &&
						condition.Reason == "DeploymentStatus" &&
						condition.Message == "MCP Deployment is available and ready" {
						return true
					}
				}
				return false
			}, time.Second*10, time.Millisecond*250).Should(BeTrue())
		})

		It("should set Degraded condition when deployment fails", func() {
			By("Initial reconciliation")
			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			By("Simulating deployment failure")
			deployment := &appsv1.Deployment{}
			Eventually(func() error {
				return k8sClient.Get(ctx, typeNamespacedName, deployment)
			}, time.Second*10, time.Millisecond*250).Should(Succeed())

			// Update deployment status to simulate failure
			deployment.Status.Conditions = []appsv1.DeploymentCondition{
				{
					Type:               appsv1.DeploymentReplicaFailure,
					Status:             corev1.ConditionTrue,
					LastUpdateTime:     metav1.Now(),
					LastTransitionTime: metav1.Now(),
					Reason:             "FailedCreate",
					Message:            "Failed to create replica set",
				},
			}
			deployment.Status.Replicas = 1
			deployment.Status.ReadyReplicas = 0
			deployment.Status.AvailableReplicas = 0
			Expect(k8sClient.Status().Update(ctx, deployment)).To(Succeed())

			By("Reconciling again to update status")
			_, err = controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			By("Verifying Degraded condition is set")
			Eventually(func() bool {
				if err := k8sClient.Get(ctx, typeNamespacedName, mcp); err != nil {
					return false
				}

				for _, condition := range mcp.Status.Conditions {
					if condition.Type == "Degraded" &&
						condition.Status == metav1.ConditionTrue &&
						condition.Reason == "DeploymentStatus" {
						return true
					}
				}
				return false
			}, time.Second*10, time.Millisecond*250).Should(BeTrue())
		})

		It("should handle multiple condition updates correctly", func() {
			By("Initial reconciliation")
			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			By("Verifying only one condition is True at a time")
			Eventually(func() bool {
				if err := k8sClient.Get(ctx, typeNamespacedName, mcp); err != nil {
					return false
				}

				trueConditions := 0
				for _, condition := range mcp.Status.Conditions {
					if condition.Status == metav1.ConditionTrue {
						trueConditions++
					}
				}
				return trueConditions == 1
			}, time.Second*10, time.Millisecond*250).Should(BeTrue())

			By("Simulating deployment progress")
			deployment := &appsv1.Deployment{}
			Eventually(func() error {
				return k8sClient.Get(ctx, typeNamespacedName, deployment)
			}, time.Second*10, time.Millisecond*250).Should(Succeed())

			deployment.Status.Conditions = []appsv1.DeploymentCondition{
				{
					Type:               appsv1.DeploymentProgressing,
					Status:             corev1.ConditionTrue,
					LastUpdateTime:     metav1.Now(),
					LastTransitionTime: metav1.Now(),
					Reason:             "NewReplicaSetCreated",
					Message:            "Created new replica set",
				},
			}
			deployment.Status.Replicas = 1
			Expect(k8sClient.Status().Update(ctx, deployment)).To(Succeed())

			By("Reconciling to update status")
			_, err = controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			By("Verifying Progressing condition with updated message")
			Eventually(func() bool {
				if err := k8sClient.Get(ctx, typeNamespacedName, mcp); err != nil {
					return false
				}

				for _, condition := range mcp.Status.Conditions {
					if condition.Type == "Progressing" &&
						condition.Status == metav1.ConditionTrue &&
						condition.Reason == "DeploymentStatus" &&
						condition.Message == "MCP Deployment is progressing: Created new replica set" {
						return true
					}
				}
				return false
			}, time.Second*10, time.Millisecond*250).Should(BeTrue())
		})
	})

	Context("When testing External MCP status", func() {
		const testURL = "external-mcp.example.com"

		var (
			resourceName       string
			typeNamespacedName types.NamespacedName
			mcp                *helloworldv1.MCP
		)

		BeforeEach(func() {
			resourceName = fmt.Sprintf("test-external-status-mcp-%d", testCounter)
			typeNamespacedName = types.NamespacedName{
				Name:      resourceName,
				Namespace: "default",
			}

			// Create MCP resource with external type
			mcpType := helloworldv1.External
			mcp = &helloworldv1.MCP{
				ObjectMeta: metav1.ObjectMeta{
					Name:      resourceName,
					Namespace: "default",
				},
				Spec: helloworldv1.MCPSpec{
					Type: &mcpType,
					External: &helloworldv1.ExternalMCPSpec{
						URL: testURL,
					},
				},
			}
			Expect(k8sClient.Create(ctx, mcp)).To(Succeed())
		})

		AfterEach(func() {
			// Clean up resources
			resource := &helloworldv1.MCP{}
			if err := k8sClient.Get(ctx, typeNamespacedName, resource); err == nil {
				// Remove finalizer first to allow deletion
				resource.Finalizers = []string{}
				Expect(k8sClient.Update(ctx, resource)).To(Succeed())

				Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
				// Wait for deletion to complete
				Eventually(func() bool {
					err := k8sClient.Get(ctx, typeNamespacedName, resource)
					return errors.IsNotFound(err)
				}, time.Second*15, time.Millisecond*250).Should(BeTrue())
			}

			// Clean up any leftover service
			service := &corev1.Service{}
			if err := k8sClient.Get(ctx, typeNamespacedName, service); err == nil {
				Expect(k8sClient.Delete(ctx, service)).To(Succeed())
			}
		})

		It("should set MCPInfo URL for external MCP without deployment conditions", func() {
			By("Reconciling the external MCP")
			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			By("Checking that MCPInfo URL is set correctly")
			Eventually(func() bool {
				if err := k8sClient.Get(ctx, typeNamespacedName, mcp); err != nil {
					return false
				}

				expectedURL := fmt.Sprintf("http://%s:8080", resourceName)
				return mcp.Status.Info != nil && mcp.Status.Info.URL == expectedURL
			}, time.Second*10, time.Millisecond*250).Should(BeTrue())

			By("Verifying no deployment-related conditions are set for external MCP")
			Consistently(func() bool {
				if err := k8sClient.Get(ctx, typeNamespacedName, mcp); err != nil {
					return false
				}

				// For external MCPs, we shouldn't have deployment-related conditions
				for _, condition := range mcp.Status.Conditions {
					if condition.Type == "Available" || condition.Type == "Progressing" || condition.Type == "Degraded" {
						return false
					}
				}
				return true
			}, time.Second*3, time.Millisecond*250).Should(BeTrue())
		})
	})
})

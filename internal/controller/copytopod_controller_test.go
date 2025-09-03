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
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	helloworldv1 "github.com/rvolykh/helloworld-operator/api/v1"
	"github.com/rvolykh/helloworld-operator/internal/controller/fake"
)

var _ = Describe("CopyToPod Controller", func() {
	Context("When reconciling a resource", func() {
		const namespace = "default"
		var (
			ctx = context.Background()

			resourceTypeNamespacedName = types.NamespacedName{
				Name:      "test-resource",
				Namespace: namespace,
			}

			copytopod = &helloworldv1.CopyToPod{}
		)

		BeforeEach(func() {
			By("creating the custom resource for the Kind CopyToPod")
			err := k8sClient.Get(ctx, resourceTypeNamespacedName, copytopod)
			if err != nil && errors.IsNotFound(err) {
				resource := &helloworldv1.CopyToPod{
					ObjectMeta: metav1.ObjectMeta{
						Name:      resourceTypeNamespacedName.Name,
						Namespace: resourceTypeNamespacedName.Namespace,
					},
					Spec: helloworldv1.CopyToPodSpec{
						PodName:       "test-nginx",
						ContainerName: "test-nginx",
						FileName:      "/usr/share/nginx/html/index.html",
						Content:       "Hello World",
					},
				}
				Expect(k8sClient.Create(ctx, resource)).To(Succeed())
			}
		})

		AfterEach(func() {
			resource := &helloworldv1.CopyToPod{}
			err := k8sClient.Get(ctx, resourceTypeNamespacedName, resource)
			Expect(err).NotTo(HaveOccurred())

			By("Cleanup the specific resource instance CopyToPod")
			Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
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
		})

		It("should fail to reconcile the resource", func() {
			By("Reconciling the created resource")
			controllerReconciler := &CopyToPodReconciler{
				Client:     k8sClient,
				Scheme:     k8sClient.Scheme(),
				PodCopyCmd: &fake.FakePodCopyCmd{Err: errors.NewResourceExpired("Expected")},
			}

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: resourceTypeNamespacedName,
			})
			Expect(err).To(HaveOccurred())
		})
	})
})

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
	"reflect"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	helloworldv1 "github.com/rvolykh/helloworld-operator/api/v1"
)

const (
	mcpFinalizer = "mcp.helloworld.rvolykh.github.com/finalizer"
)

// MCPReconciler reconciles a MCP object
type MCPReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=helloworld.rvolykh.github.com,resources=mcps,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=helloworld.rvolykh.github.com,resources=mcps/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=helloworld.rvolykh.github.com,resources=mcps/finalizers,verbs=update
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=services,verbs=get;list;watch;create;update;patch;delete

// SetupWithManager sets up the controller with the Manager.
func (r *MCPReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&helloworldv1.MCP{}).
		Owns(&appsv1.Deployment{}).
		Owns(&corev1.Service{}).
		Named("mcp").
		Complete(r)
}

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *MCPReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := logf.FromContext(ctx)

	cr := &helloworldv1.MCP{}
	if err := r.Get(ctx, req.NamespacedName, cr); err != nil {
		if errors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	logger = logger.WithValues(
		"name", cr.Name,
		"namespace", cr.Namespace,
	)

	if err := r.handleCR(ctx, cr); err != nil {
		if errors.IsResourceExpired(err) {
			return ctrl.Result{}, errors.NewResourceExpired("MCP Object is being deleted, stop MCP reconciliation")
		}
		logger.Error(err, "Finalizer handling failed")
		return ctrl.Result{}, err
	}

	switch *cr.Spec.Type {
	case helloworldv1.External:
		return ctrl.Result{}, r.handleService(ctx, cr)
	case helloworldv1.Internal:
		fallthrough
	default:
		if err := r.handleDeployment(ctx, cr); err != nil {
			return ctrl.Result{}, err
		}
		if err := r.handleService(ctx, cr); err != nil {
			return ctrl.Result{}, err
		}
	}

	return ctrl.Result{}, nil
}

func (r *MCPReconciler) handleDeployment(ctx context.Context, cr *helloworldv1.MCP) error {
	logger := logf.FromContext(ctx)
	logger.Info("Handling Deployment")

	var (
		namespacedName = types.NamespacedName{
			Name:      cr.Name,
			Namespace: cr.Namespace,
		}

		deployment = &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      cr.Name,
				Namespace: cr.Namespace,
			},
			Spec: appsv1.DeploymentSpec{
				Replicas: func() *int32 { r := int32(1); return &r }(),
				Selector: &metav1.LabelSelector{
					MatchLabels: map[string]string{
						"app": cr.Name,
					},
				},
				Template: corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{
						Labels: map[string]string{
							"app": cr.Name,
						},
					},
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{
							{
								Name:  cr.Name,
								Image: cr.Spec.Internal.Image,
								Ports: []corev1.ContainerPort{
									{
										ContainerPort: 8080,
									},
								},
							},
						},
					},
				},
			},
		}

		expectedSpec appsv1.DeploymentSpec
	)

	expectedSpec = *deployment.Spec.DeepCopy()

	// The MCP is being deleted
	if !controllerutil.ContainsFinalizer(cr, mcpFinalizer) {
		// stop reconciliation
		logger.Info("MCP Object is being deleted, stop Deployment reconciliation")
		return nil
	}

	if err := r.Get(ctx, namespacedName, deployment); err != nil {
		if !errors.IsNotFound(err) {
			return err
		}

		if err := controllerutil.SetControllerReference(cr, deployment, r.Scheme); err != nil {
			logger.Error(err, "Failed to set controller reference")
			return err
		}
		if err := r.Create(ctx, deployment); err != nil {
			logger.Error(err, "Failed to create Deployment")
			return err
		}
	}

	if reflect.DeepEqual(expectedSpec, deployment.Spec) {
		logger.Info("Deployment is up to date")
		return nil
	}

	// Update the deployment spec to match expected spec
	deployment.Spec = expectedSpec
	if err := r.Update(ctx, deployment); err != nil {
		logger.Error(err, "Failed to update Deployment")
		return err
	}
	logger.Info("Deployment updated")

	return nil
}

func (r *MCPReconciler) handleService(ctx context.Context, cr *helloworldv1.MCP) error {
	logger := logf.FromContext(ctx)
	logger.Info("Handling Service")

	var (
		namespacedName = types.NamespacedName{
			Name:      cr.Name,
			Namespace: cr.Namespace,
		}

		service = &corev1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:      cr.Name,
				Namespace: cr.Namespace,
			},
		}

		expectedSpec corev1.ServiceSpec
	)

	if *cr.Spec.Type == helloworldv1.External {
		service.Spec = corev1.ServiceSpec{
			Type:         corev1.ServiceTypeExternalName,
			ExternalName: cr.Spec.External.URL,
		}
	} else {
		service.Spec = corev1.ServiceSpec{
			Type: corev1.ServiceTypeClusterIP,
			Selector: map[string]string{
				"app": cr.Name,
			},
			Ports: []corev1.ServicePort{
				{
					Port:       8080,
					TargetPort: intstr.FromInt(8080),
				},
			},
		}
	}
	expectedSpec = *service.Spec.DeepCopy()

	// The MCP is being deleted
	if !controllerutil.ContainsFinalizer(cr, mcpFinalizer) {
		// stop reconciliation
		logger.Info("MCP Object is being deleted, stop Service reconciliation")
		return nil
	}

	if err := r.Get(ctx, namespacedName, service); err != nil {
		if !errors.IsNotFound(err) {
			return err
		}

		if err := controllerutil.SetControllerReference(cr, service, r.Scheme); err != nil {
			logger.Error(err, "Failed to set controller reference")
			return err
		}
		if err := r.Create(ctx, service); err != nil {
			logger.Error(err, "Failed to create Service")
			return err
		}
	}

	if reflect.DeepEqual(expectedSpec, service.Spec) {
		logger.Info("Service is up to date")
		return nil
	}

	// Update the service spec to match expected spec
	service.Spec = expectedSpec
	if err := r.Update(ctx, service); err != nil {
		logger.Error(err, "Failed to update Service")
		return err
	}
	logger.Info("Service updated")

	return nil
}

func (r *MCPReconciler) handleCR(ctx context.Context, cr *helloworldv1.MCP) error {
	logger := logf.FromContext(ctx)
	logger.Info("Handling MCP")

	// The CR is not being deleted
	if cr.DeletionTimestamp.IsZero() {
		// add the finalizer if it's not there
		if !controllerutil.ContainsFinalizer(cr, mcpFinalizer) {
			controllerutil.AddFinalizer(cr, mcpFinalizer)
			if err := r.Update(ctx, cr); err != nil {
				return err
			}
			logger.Info("CR Object finalizer is added")
		}

		return nil
	}

	// The CR is being deleted
	if controllerutil.ContainsFinalizer(cr, mcpFinalizer) {
		// Cleanup owned resources before removing finalizer
		if err := r.cleanupOwnedResources(ctx, cr); err != nil {
			logger.Error(err, "Failed to cleanup owned resources")
			return err
		}

		// Remove the finalizer after cleanup
		controllerutil.RemoveFinalizer(cr, mcpFinalizer)
		if err := r.Update(ctx, cr); err != nil {
			if !errors.IsNotFound(err) {
				return err
			}
		}
		logger.Info("CR Object finalizer is removed")

		return errors.NewResourceExpired("MCP Object is being deleted, stop MCP reconciliation")
	}

	return nil
}

func (r *MCPReconciler) cleanupOwnedResources(ctx context.Context, cr *helloworldv1.MCP) error {
	logger := logf.FromContext(ctx)
	logger.Info("Cleaning up owned resources")

	namespacedName := types.NamespacedName{
		Name:      cr.Name,
		Namespace: cr.Namespace,
	}

	// Delete deployment if it exists
	deployment := &appsv1.Deployment{}
	if err := r.Get(ctx, namespacedName, deployment); err == nil {
		logger.Info("Deleting deployment")
		if err := r.Delete(ctx, deployment); err != nil && !errors.IsNotFound(err) {
			return err
		}
	}

	// Delete service if it exists
	service := &corev1.Service{}
	if err := r.Get(ctx, namespacedName, service); err == nil {
		logger.Info("Deleting service")
		if err := r.Delete(ctx, service); err != nil && !errors.IsNotFound(err) {
			return err
		}
	}

	return nil
}

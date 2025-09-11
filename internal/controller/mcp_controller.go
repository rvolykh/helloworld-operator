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
	"reflect"

	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	helloworldv1 "github.com/rvolykh/helloworld-operator/api/v1"
)

const (
	mcpFinalizer = "mcp.helloworld.rvolykh.github.com/finalizer"

	eventReasonCreating = "Creating"
	eventReasonUpdating = "Updating"
	eventReasonDeleting = "Deleting"
)

// MCPReconciler reconciles a MCP object
type MCPReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder
}

// +kubebuilder:rbac:groups=helloworld.rvolykh.github.com,resources=mcps,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=helloworld.rvolykh.github.com,resources=mcps/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=helloworld.rvolykh.github.com,resources=mcps/finalizers,verbs=update
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=services,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=events,verbs=create;patch

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
	logger.Info("MCP reconciliation started")

	cr := &helloworldv1.MCP{}
	if err := r.Get(ctx, req.NamespacedName, cr); err != nil {
		if errors.IsNotFound(err) {
			logger.Info("MCP Object is not found, stop MCP reconciliation")
			return ctrl.Result{}, nil
		}

		logger.Error(err, "Failed to get MCP Object")
		return ctrl.Result{}, err
	}

	if err := r.handleFinalizer(ctx, logger, cr); err != nil {
		if errors.IsResourceExpired(err) {
			logger.Info("MCP Object is deleted, stop MCP reconciliation")
			return ctrl.Result{}, nil
		}

		logger.Error(err, "MCP Object finalizer handling failed")
		return ctrl.Result{}, err
	}

	switch *cr.Spec.Type {
	case helloworldv1.External:
		if err := r.handleService(ctx, logger, cr); err != nil {
			logger.Error(err, "Failed to handle MCP Service resource (externalName)")
			return ctrl.Result{}, err
		}
	case helloworldv1.Internal:
		fallthrough
	default:
		if err := r.handleDeployment(ctx, logger, cr); err != nil {
			logger.Error(err, "Failed to handle MCP Deployment resource")
			return ctrl.Result{}, err
		}

		if err := r.handleService(ctx, logger, cr); err != nil {
			logger.Error(err, "Failed to handle MCP Service resource (clusterIP)")
			return ctrl.Result{}, err
		}
	}

	if err := r.handleStatus(ctx, logger, cr); err != nil {
		logger.Error(err, "unable to update MCP status")
		return ctrl.Result{}, err
	}

	logger.Info("MCP reconciliation completed")
	return ctrl.Result{}, nil
}

func (r *MCPReconciler) handleDeployment(ctx context.Context, logger logr.Logger, cr *helloworldv1.MCP) error {
	logger.Info("Handling MCP Deployment resource")

	var (
		namespacedName = types.NamespacedName{
			Name:      cr.Name,
			Namespace: cr.Namespace,
		}

		haveDeployment = &appsv1.Deployment{}
		wantDeployment = &appsv1.Deployment{
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
										Protocol:      corev1.ProtocolTCP,
									},
								},
							},
						},
					},
				},
			},
		}
	)

	// The MCP is being deleted
	if !controllerutil.ContainsFinalizer(cr, mcpFinalizer) {
		// stop reconciliation
		logger.Info("MCP Object is being deleted, stop MCP Deployment resource reconciliation")
		return nil
	}

	if err := r.Get(ctx, namespacedName, haveDeployment); err != nil {
		if !errors.IsNotFound(err) {
			return fmt.Errorf("failed to get MCP Deployment resource: %w", err)
		}

		if err := controllerutil.SetControllerReference(cr, wantDeployment, r.Scheme); err != nil {
			return fmt.Errorf("failed to set controller reference: %w", err)
		}
		if err := r.Create(ctx, wantDeployment); err != nil {
			return fmt.Errorf("failed to create MCP Deployment resource: %w", err)
		}

		logger.Info("MCP Deployment resource created")
		r.Recorder.Eventf(cr, corev1.EventTypeNormal, eventReasonCreating,
			"MCP Deployment resource %s is created in namespace %s", wantDeployment.Name, wantDeployment.Namespace)
		return nil
	}

	isUpToDate := reflect.DeepEqual(wantDeployment.Labels, haveDeployment.Labels) &&
		*wantDeployment.Spec.Replicas == *haveDeployment.Spec.Replicas &&
		reflect.DeepEqual(wantDeployment.Spec.Selector, haveDeployment.Spec.Selector) &&
		reflect.DeepEqual(wantDeployment.Spec.Template.Labels, haveDeployment.Spec.Template.Labels) &&
		len(wantDeployment.Spec.Template.Spec.Containers) == len(haveDeployment.Spec.Template.Spec.Containers) &&
		wantDeployment.Spec.Template.Spec.Containers[0].Name == haveDeployment.Spec.Template.Spec.Containers[0].Name &&
		wantDeployment.Spec.Template.Spec.Containers[0].Image == haveDeployment.Spec.Template.Spec.Containers[0].Image &&
		reflect.DeepEqual(wantDeployment.Spec.Template.Spec.Containers[0].Ports, haveDeployment.Spec.Template.Spec.Containers[0].Ports)

	if isUpToDate {
		logger.Info("MCP Deployment resource is up-to-date")
		return nil
	}

	// Update the deployment spec to match expected spec
	if err := r.Update(ctx, wantDeployment); err != nil {
		return fmt.Errorf("failed to update MCP Deployment resource: %w", err)
	}

	logger.Info("MCP Deployment resource updated")
	r.Recorder.Eventf(cr, corev1.EventTypeNormal, eventReasonUpdating,
		"MCP Deployment resource %s is updated in namespace %s", wantDeployment.Name, wantDeployment.Namespace)
	return nil
}

func (r *MCPReconciler) handleService(ctx context.Context, logger logr.Logger, cr *helloworldv1.MCP) error {
	logger.Info("Handling MCP Service resource")

	var (
		namespacedName = types.NamespacedName{
			Name:      cr.Name,
			Namespace: cr.Namespace,
		}

		haveService = &corev1.Service{}
		wantService = &corev1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:      cr.Name,
				Namespace: cr.Namespace,
			},
		}
	)

	if *cr.Spec.Type == helloworldv1.External {
		wantService.Spec = corev1.ServiceSpec{
			Type:         corev1.ServiceTypeExternalName,
			ExternalName: cr.Spec.External.URL,
		}
	} else {
		wantService.Spec = corev1.ServiceSpec{
			Type: corev1.ServiceTypeClusterIP,
			Selector: map[string]string{
				"app": cr.Name,
			},
			Ports: []corev1.ServicePort{
				{
					Port:       8080,
					Protocol:   corev1.ProtocolTCP,
					TargetPort: intstr.FromInt(8080),
				},
			},
		}
	}

	// The MCP is being deleted
	if !controllerutil.ContainsFinalizer(cr, mcpFinalizer) {
		// stop reconciliation
		logger.Info("MCP Object is being deleted, stop MCP Service resource reconciliation")
		return nil
	}

	if err := r.Get(ctx, namespacedName, haveService); err != nil {
		if !errors.IsNotFound(err) {
			return fmt.Errorf("failed to get MCP Service resource: %w", err)
		}

		if err := controllerutil.SetControllerReference(cr, wantService, r.Scheme); err != nil {
			return fmt.Errorf("failed to set controller reference: %w", err)
		}
		if err := r.Create(ctx, wantService); err != nil {
			return fmt.Errorf("failed to create MCP Service resource: %w", err)
		}

		logger.Info("MCP Service resource created")
		r.Recorder.Eventf(cr, corev1.EventTypeNormal, eventReasonCreating,
			"MCP Service resource %s is created in namespace %s", wantService.Name, wantService.Namespace)
		return nil
	}

	isUpToDate := reflect.DeepEqual(wantService.Labels, haveService.Labels) &&
		reflect.DeepEqual(wantService.Spec.Selector, haveService.Spec.Selector) &&
		reflect.DeepEqual(wantService.Spec.Ports, haveService.Spec.Ports)

	if isUpToDate {
		logger.Info("MCP Service resource is up-to-date")
		return nil
	}

	// Update the service spec to match expected spec
	if err := r.Update(ctx, wantService); err != nil {
		return fmt.Errorf("failed to update MCP Service resource: %w", err)
	}

	logger.Info("MCP Service resource updated")
	r.Recorder.Eventf(cr, corev1.EventTypeNormal, eventReasonUpdating,
		"MCP Service resource %s is updated in namespace %s", wantService.Name, wantService.Namespace)
	return nil
}

func (r *MCPReconciler) handleFinalizer(ctx context.Context, logger logr.Logger, cr *helloworldv1.MCP) error {
	logger.Info("Handling MCP Object finalizer")

	// The CR is not being deleted
	if cr.DeletionTimestamp.IsZero() {
		// add the finalizer if it's not there
		if !controllerutil.ContainsFinalizer(cr, mcpFinalizer) {
			controllerutil.AddFinalizer(cr, mcpFinalizer)
			if err := r.Update(ctx, cr); err != nil {
				return fmt.Errorf("failed to add MCP Object finalizer: %w", err)
			}

			logger.Info("MCP Object finalizer is added")
			r.Recorder.Eventf(cr, corev1.EventTypeNormal, eventReasonCreating,
				"MCP Object finalizer is added to %s in namespace %s", cr.Name, cr.Namespace)
		}

		return nil
	}

	// The CR is being deleted
	if controllerutil.ContainsFinalizer(cr, mcpFinalizer) {
		// Cleanup owned resources before removing finalizer
		if err := r.cleanupOwnedResources(ctx, logger, cr); err != nil {
			return fmt.Errorf("failed to cleanup owned resources: %w", err)
		}

		r.Recorder.Eventf(cr, corev1.EventTypeWarning, eventReasonDeleting,
			"MCP Object %s is being deleted from the namespace %s", cr.Name, cr.Namespace)

		// Remove the finalizer after cleanup
		controllerutil.RemoveFinalizer(cr, mcpFinalizer)
		if err := r.Update(ctx, cr); err != nil {
			if !errors.IsNotFound(err) {
				return fmt.Errorf("failed to remove MCP Object finalizer: %w", err)
			}
		}
		logger.Info("MCP Object finalizer is removed")

		return errors.NewResourceExpired("MCP Object is being deleted, stop MCP reconciliation")
	}

	return nil
}

func (r *MCPReconciler) cleanupOwnedResources(ctx context.Context, logger logr.Logger, cr *helloworldv1.MCP) error {
	logger.Info("Cleaning up owned resources")

	namespacedName := types.NamespacedName{
		Name:      cr.Name,
		Namespace: cr.Namespace,
	}

	// Delete deployment if it exists
	deployment := &appsv1.Deployment{}
	if err := r.Get(ctx, namespacedName, deployment); err == nil {
		logger.Info("Deleting deployment")

		r.Recorder.Eventf(cr, corev1.EventTypeWarning, eventReasonDeleting,
			"MCP Deployment resource %s is being deleted from the namespace %s", cr.Name, cr.Namespace)

		if err := r.Delete(ctx, deployment); err != nil && !errors.IsNotFound(err) {
			return fmt.Errorf("failed to delete MCP Deployment resource: %w", err)
		}
	}

	// Delete service if it exists
	service := &corev1.Service{}
	if err := r.Get(ctx, namespacedName, service); err == nil {
		logger.Info("Deleting service")

		r.Recorder.Eventf(cr, corev1.EventTypeWarning, eventReasonDeleting,
			"MCP Service resource %s is being deleted from the namespace %s", cr.Name, cr.Namespace)

		if err := r.Delete(ctx, service); err != nil && !errors.IsNotFound(err) {
			return fmt.Errorf("failed to delete MCP Service resource: %w", err)
		}
	}

	logger.Info("Owned resources cleaned up")
	return nil
}

func (r *MCPReconciler) handleStatus(ctx context.Context, logger logr.Logger, cr *helloworldv1.MCP) error {
	logger.Info("Handling MCP Object status")

	var (
		namespacedName = types.NamespacedName{
			Name:      cr.Name,
			Namespace: cr.Namespace,
		}
		deployment = &appsv1.Deployment{}

		wantInfo = &helloworldv1.MCPInfo{
			URL: fmt.Sprintf("http://%s:8080", cr.Name),
		}
		wantCondition metav1.Condition
	)

	isUpToDate := cr.Status.Info != nil && cr.Status.Info.URL == wantInfo.URL
	if cr.Spec.Type != nil && *cr.Spec.Type == helloworldv1.Internal {
		err := r.Get(ctx, namespacedName, deployment)
		if err != nil && !errors.IsNotFound(err) {
			return fmt.Errorf("failed to fetch MCP Deployment resource: %w", err)
		}

		wantCondition = r.prepareMCPCondition(deployment)
		haveCondition := meta.FindStatusCondition(cr.Status.Conditions, wantCondition.Type)

		isUpToDate = isUpToDate && haveCondition != nil &&
			haveCondition.Status == wantCondition.Status &&
			haveCondition.Reason == wantCondition.Reason &&
			haveCondition.Message == wantCondition.Message
	}

	if isUpToDate {
		logger.Info("MCP Object status is up-to-date")
		return nil
	}

	if wantCondition.Type != "" {
		for i, condition := range cr.Status.Conditions {
			if condition.Type != wantCondition.Type {
				cr.Status.Conditions[i].Status = metav1.ConditionFalse
			}
		}
		meta.SetStatusCondition(&cr.Status.Conditions, wantCondition)
	}
	cr.Status.Info = wantInfo

	if err := r.Status().Update(ctx, cr); err != nil {
		return fmt.Errorf("failed to update MCP Object status: %w", err)
	}

	logger.Info("MCP Object status updated", "url", wantInfo.URL, "condition", wantCondition.Type)
	return nil
}

func (r *MCPReconciler) prepareMCPCondition(deployment *appsv1.Deployment) metav1.Condition {
	if deployment == nil {
		return metav1.Condition{
			Type:    helloworldv1.MCPConditionTypeProgressing,
			Status:  metav1.ConditionTrue,
			Reason:  helloworldv1.MCPConditionReasonDeploymentStatus,
			Message: "Deployment resource is not found, waiting for deployment creation",
		}
	}

	var latestDeploymentCondition *appsv1.DeploymentCondition
	for i := range deployment.Status.Conditions {
		condition := &deployment.Status.Conditions[i]
		if latestDeploymentCondition == nil || condition.LastUpdateTime.After(latestDeploymentCondition.LastUpdateTime.Time) {
			latestDeploymentCondition = condition
		}
	}

	if latestDeploymentCondition == nil {
		return metav1.Condition{
			Type:    helloworldv1.MCPConditionTypeProgressing,
			Status:  metav1.ConditionTrue,
			Reason:  helloworldv1.MCPConditionReasonDeploymentStatus,
			Message: "Deployment is created but no status conditions available yet",
		}
	}

	switch latestDeploymentCondition.Type {
	case appsv1.DeploymentAvailable:
		return metav1.Condition{
			Type:    helloworldv1.MCPConditionTypeAvailable,
			Status:  metav1.ConditionTrue,
			Reason:  helloworldv1.MCPConditionReasonDeploymentStatus,
			Message: "MCP Deployment is available and ready",
		}
	case appsv1.DeploymentProgressing:
		return metav1.Condition{
			Type:    helloworldv1.MCPConditionTypeProgressing,
			Status:  metav1.ConditionTrue,
			Reason:  helloworldv1.MCPConditionReasonDeploymentStatus,
			Message: fmt.Sprintf("MCP Deployment is progressing: %s", latestDeploymentCondition.Message),
		}
	}

	return metav1.Condition{
		Type:    helloworldv1.MCPConditionTypeDegraded,
		Status:  metav1.ConditionTrue,
		Reason:  helloworldv1.MCPConditionReasonDeploymentStatus,
		Message: fmt.Sprintf("MCP Deployment has an issue: %s", latestDeploymentCondition.Reason),
	}
}

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

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	helloworldv1 "github.com/rvolykh/helloworld-operator/api/v1"
)

const copyToPodFinalizer = "copytopod.helloworld.rvolykh.github.com/finalizer"

// CopyToPodReconciler reconciles a CopyToPod object
type CopyToPodReconciler struct {
	client.Client
	Scheme *runtime.Scheme

	PodCopyCmd PodCopyCmd
}

// +kubebuilder:rbac:groups=helloworld.rvolykh.github.com,resources=copytopods,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=helloworld.rvolykh.github.com,resources=copytopods/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=helloworld.rvolykh.github.com,resources=copytopods/finalizers,verbs=update
// +kubebuilder:rbac:groups="",resources=pod,verbs=get;list;watch;update

func (r *CopyToPodReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&helloworldv1.CopyToPod{}).
		Owns(&corev1.Pod{}).
		Named("copytopod").
		Complete(r)
}

func (r *CopyToPodReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := logf.FromContext(ctx)

	cr := &helloworldv1.CopyToPod{}
	if err := r.Get(ctx, req.NamespacedName, cr); err != nil {
		if errors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		logger.Error(err, "Failed to get CopyToPod")
		return ctrl.Result{}, err
	}

	logger = logger.WithValues(
		"podName", cr.Spec.PodName,
		"fileName", cr.Spec.FileName,
	)

	hasPod, err := r.finalizerPod(ctx, cr)
	if err != nil {
		logger.Error(err, "Pod Finalizer handling failed")
		return ctrl.Result{}, err
	}
	stopReconcile, err := r.finalizerCR(ctx, cr)
	if err != nil {
		logger.Error(err, "CR Finalizer handling failed")
		return ctrl.Result{}, err
	}
	if stopReconcile {
		logger.Info("CR Object is being deleted, stop reconciliation")
		return ctrl.Result{}, nil
	}

	copyCtx, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()

	if err := r.PodCopyCmd.CopyTo(copyCtx, *cr); err != nil {
		if hasPod {
			logger.Error(err, "Copy failed")
		} else {
			logger.Info("Copy failed, re-attempt in next reconciliation loop")
		}

		meta.SetStatusCondition(
			&cr.Status.Conditions,
			metav1.Condition{
				Type:    "Available",
				Status:  metav1.ConditionFalse,
				Reason:  "Reconciling",
				Message: err.Error(),
			},
		)
		if err := r.Status().Update(ctx, cr); err != nil {
			if errors.IsConflict(err) {
				logger.Info("Status update failed, re-attempt in next reconciliation loop")
				return ctrl.Result{RequeueAfter: 1 * time.Second}, nil
			}
			logger.Error(err, "Failed to update CopyToPod status")
			return ctrl.Result{}, err
		}

		return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
	}
	logger.Info("Copy succeeded")

	meta.SetStatusCondition(
		&cr.Status.Conditions,
		metav1.Condition{
			Type:    "Available",
			Status:  metav1.ConditionTrue,
			Reason:  "Completed",
			Message: "File copied successfully",
		},
	)
	if err := r.Status().Update(ctx, cr); err != nil {
		if errors.IsConflict(err) {
			logger.Info("Status update failed, re-attempt in next reconciliation loop")
			return ctrl.Result{RequeueAfter: 1 * time.Second}, nil
		}
		logger.Error(err, "Failed to update CopyToPod status")
		return ctrl.Result{}, err
	}
	logger.Info("Status updated")

	return ctrl.Result{}, nil
}

func (r *CopyToPodReconciler) finalizerCR(ctx context.Context, cr *helloworldv1.CopyToPod) (bool, error) {
	logger := logf.FromContext(ctx)

	// The CR is not being deleted
	if cr.DeletionTimestamp.IsZero() {
		// add the finalizer if it's not there
		if !controllerutil.ContainsFinalizer(cr, copyToPodFinalizer) {
			controllerutil.AddFinalizer(cr, copyToPodFinalizer)
			if err := r.Update(ctx, cr); err != nil {
				return false, fmt.Errorf("update for add: %w", err)
			}
			logger.Info("CR Object finalizer is added")
		}

		return false, nil
	}

	// The CR is being deleted
	if controllerutil.ContainsFinalizer(cr, copyToPodFinalizer) {
		// Remove the finalizer after cleanup
		controllerutil.RemoveFinalizer(cr, copyToPodFinalizer)
		if err := r.Update(ctx, cr); err != nil {
			if !errors.IsNotFound(err) {
				return false, fmt.Errorf("update for remove: %w", err)
			}
		}
		logger.Info("CR Object finalizer is removed")
	}

	// Stop reconciliation as the object is being deleted and finalizer removed
	return true, nil
}

func (r *CopyToPodReconciler) finalizerPod(ctx context.Context, cr *helloworldv1.CopyToPod) (bool, error) {
	logger := logf.FromContext(ctx)

	var (
		podNamespaceName = types.NamespacedName{
			Name:      cr.Spec.PodName,
			Namespace: cr.Namespace,
		}
		pod = &corev1.Pod{}
	)
	if err := r.Get(ctx, podNamespaceName, pod); err != nil {
		if errors.IsNotFound(err) {
			logger.Info("Pod Object is not found")
			return false, nil
		}
		return false, fmt.Errorf("get pod: %w", err)
	}

	// The CR or POD is being deleted
	if !cr.DeletionTimestamp.IsZero() || !pod.DeletionTimestamp.IsZero() {
		if controllerutil.ContainsFinalizer(pod, copyToPodFinalizer) {
			// stop watching Pod resource
			controllerutil.RemoveFinalizer(pod, copyToPodFinalizer)
			if err := controllerutil.RemoveControllerReference(cr, pod, r.Scheme); err != nil {
				return true, fmt.Errorf("remove controller reference: %w", err)
			}
			if err := r.Update(ctx, pod); err != nil {
				return true, fmt.Errorf("update for remove: %w", err)
			}
			logger.Info("Pod Object finalizer is removed")
		}

		return false, nil
	}

	// The POD is not being deleted
	if !controllerutil.ContainsFinalizer(pod, copyToPodFinalizer) {
		// watch Pod resource and add finializer to it
		controllerutil.AddFinalizer(pod, copyToPodFinalizer)
		if err := controllerutil.SetControllerReference(cr, pod, r.Scheme); err != nil {
			return true, fmt.Errorf("set controller reference: %w", err)
		}
		if err := r.Update(ctx, pod); err != nil {
			return true, fmt.Errorf("update for add: %w", err)
		}
		logger.Info("Pod Object finalizer is added")
	}

	return true, nil
}

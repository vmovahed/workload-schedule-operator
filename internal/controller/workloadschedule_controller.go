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
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	infrav1alpha1 "github.com/vmovahed/workload-schedule-operator/api/v1alpha1"
)

const (
	// FinalizerName is the finalizer for WorkloadSchedule resources
	FinalizerName = "workloadschedule.infra.illumin.com/finalizer"

	// WorldTimeAPIURL is the base URL for the World Time API
	WorldTimeAPIURL = "https://worldtimeapi.org/api/timezone"

	// RequeueInterval is the default requeue interval for reconciliation
	RequeueInterval = 60 * time.Second

	// ConditionTypeReady is the condition type for ready status
	ConditionTypeReady = "Ready"

	// ConditionTypeSynced is the condition type for synced status
	ConditionTypeSynced = "Synced"
)

// WorldTimeResponse represents the response from worldtimeapi.org
type WorldTimeResponse struct {
	Datetime    string `json:"datetime"`
	Timezone    string `json:"timezone"`
	UtcDatetime string `json:"utc_datetime"`
	UtcOffset   string `json:"utc_offset"`
	DayOfWeek   int    `json:"day_of_week"`
	DayOfYear   int    `json:"day_of_year"`
	WeekNumber  int    `json:"week_number"`
}

// WorkloadScheduleReconciler reconciles a WorkloadSchedule object
type WorkloadScheduleReconciler struct {
	client.Client
	Scheme     *runtime.Scheme
	HTTPClient *http.Client
}

// +kubebuilder:rbac:groups=infra.illumin.com,resources=workloadschedules,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=infra.illumin.com,resources=workloadschedules/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=infra.illumin.com,resources=workloadschedules/finalizers,verbs=update
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;update;patch
// +kubebuilder:rbac:groups="",resources=namespaces,verbs=get;list;watch;create

// Reconcile is the main reconciliation loop for WorkloadSchedule resources
func (r *WorkloadScheduleReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)
	log.Info("Reconciling WorkloadSchedule", "name", req.Name, "namespace", req.Namespace)

	// Fetch the WorkloadSchedule instance
	workloadSchedule := &infrav1alpha1.WorkloadSchedule{}
	if err := r.Get(ctx, req.NamespacedName, workloadSchedule); err != nil {
		if apierrors.IsNotFound(err) {
			log.Info("WorkloadSchedule resource not found. Ignoring since object must be deleted")
			return ctrl.Result{}, nil
		}
		log.Error(err, "Failed to get WorkloadSchedule")
		return ctrl.Result{}, err
	}

	// Handle finalizer
	if workloadSchedule.ObjectMeta.DeletionTimestamp.IsZero() {
		// Object is not being deleted, add finalizer if not present
		if !controllerutil.ContainsFinalizer(workloadSchedule, FinalizerName) {
			controllerutil.AddFinalizer(workloadSchedule, FinalizerName)
			if err := r.Update(ctx, workloadSchedule); err != nil {
				log.Error(err, "Failed to add finalizer")
				return ctrl.Result{}, err
			}
			return ctrl.Result{Requeue: true}, nil
		}
	} else {
		// Object is being deleted
		if controllerutil.ContainsFinalizer(workloadSchedule, FinalizerName) {
			// Run cleanup logic
			if err := r.cleanupResources(ctx, workloadSchedule); err != nil {
				log.Error(err, "Failed to cleanup resources")
				return ctrl.Result{}, err
			}

			// Remove finalizer
			controllerutil.RemoveFinalizer(workloadSchedule, FinalizerName)
			if err := r.Update(ctx, workloadSchedule); err != nil {
				log.Error(err, "Failed to remove finalizer")
				return ctrl.Result{}, err
			}
		}
		return ctrl.Result{}, nil
	}

	// Ensure target namespace exists
	if err := r.ensureNamespace(ctx, workloadSchedule.Spec.TargetNamespace); err != nil {
		log.Error(err, "Failed to ensure namespace exists", "namespace", workloadSchedule.Spec.TargetNamespace)
		r.setCondition(workloadSchedule, ConditionTypeReady, metav1.ConditionFalse, "NamespaceError", err.Error())
		if statusErr := r.Status().Update(ctx, workloadSchedule); statusErr != nil {
			log.Error(statusErr, "Failed to update status")
		}
		return ctrl.Result{RequeueAfter: RequeueInterval}, err
	}

	// Get current time from World Time API
	currentTime, err := r.getCurrentTime(ctx, workloadSchedule.Spec.Timezone)
	if err != nil {
		log.Error(err, "Failed to get current time from World Time API", "timezone", workloadSchedule.Spec.Timezone)
		r.setCondition(workloadSchedule, ConditionTypeSynced, metav1.ConditionFalse, "TimeAPIError", err.Error())
		if statusErr := r.Status().Update(ctx, workloadSchedule); statusErr != nil {
			log.Error(statusErr, "Failed to update status")
		}
		return ctrl.Result{RequeueAfter: RequeueInterval}, err
	}

	// Determine if within active window
	withinActiveWindow := r.isWithinActiveWindow(currentTime, workloadSchedule.Spec.StartHour, workloadSchedule.Spec.EndHour)
	log.Info("Time check", "currentTime", currentTime.Format(time.RFC3339), "hour", currentTime.Hour(),
		"startHour", workloadSchedule.Spec.StartHour, "endHour", workloadSchedule.Spec.EndHour,
		"withinActiveWindow", withinActiveWindow)

	// Scale the deployment
	desiredReplicas := int32(0)
	if withinActiveWindow {
		desiredReplicas = workloadSchedule.Spec.ReplicasWhenActive
	}

	scaleAction, currentReplicas, err := r.scaleDeployment(ctx, workloadSchedule.Spec.TargetNamespace,
		workloadSchedule.Spec.TargetDeployment, desiredReplicas)
	if err != nil {
		log.Error(err, "Failed to scale deployment")
		r.setCondition(workloadSchedule, ConditionTypeReady, metav1.ConditionFalse, "ScaleError", err.Error())
		if statusErr := r.Status().Update(ctx, workloadSchedule); statusErr != nil {
			log.Error(statusErr, "Failed to update status")
		}
		return ctrl.Result{RequeueAfter: RequeueInterval}, err
	}

	// Update status
	now := metav1.Now()
	workloadSchedule.Status.CurrentLocalTime = currentTime.Format(time.RFC3339)
	workloadSchedule.Status.WithinActiveWindow = withinActiveWindow
	workloadSchedule.Status.LastScaleAction = scaleAction
	workloadSchedule.Status.LastSyncTime = &now
	workloadSchedule.Status.CurrentReplicas = currentReplicas

	r.setCondition(workloadSchedule, ConditionTypeReady, metav1.ConditionTrue, "Reconciled", "Successfully reconciled")
	r.setCondition(workloadSchedule, ConditionTypeSynced, metav1.ConditionTrue, "Synced", "Successfully synced with World Time API")

	if err := r.Status().Update(ctx, workloadSchedule); err != nil {
		log.Error(err, "Failed to update WorkloadSchedule status")
		return ctrl.Result{}, err
	}

	log.Info("Successfully reconciled WorkloadSchedule", "scaleAction", scaleAction, "replicas", currentReplicas)
	return ctrl.Result{RequeueAfter: RequeueInterval}, nil
}

// getCurrentTime fetches the current time from worldtimeapi.org for the given timezone
func (r *WorkloadScheduleReconciler) getCurrentTime(ctx context.Context, timezone string) (time.Time, error) {
	url := fmt.Sprintf("%s/%s", WorldTimeAPIURL, timezone)

	httpClient := r.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 10 * time.Second}
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return time.Time{}, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return time.Time{}, fmt.Errorf("failed to call World Time API: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return time.Time{}, fmt.Errorf("World Time API returned status %d", resp.StatusCode)
	}

	var timeResp WorldTimeResponse
	if err := json.NewDecoder(resp.Body).Decode(&timeResp); err != nil {
		return time.Time{}, fmt.Errorf("failed to decode World Time API response: %w", err)
	}

	// Parse the datetime string
	parsedTime, err := time.Parse(time.RFC3339, timeResp.Datetime)
	if err != nil {
		return time.Time{}, fmt.Errorf("failed to parse datetime: %w", err)
	}

	return parsedTime, nil
}

// isWithinActiveWindow checks if the current time is within the active window [startHour, endHour)
func (r *WorkloadScheduleReconciler) isWithinActiveWindow(currentTime time.Time, startHour, endHour int) bool {
	currentHour := currentTime.Hour()
	return currentHour >= startHour && currentHour < endHour
}

// scaleDeployment scales the target deployment to the desired number of replicas
func (r *WorkloadScheduleReconciler) scaleDeployment(ctx context.Context, namespace, deploymentName string, desiredReplicas int32) (string, int32, error) {
	log := logf.FromContext(ctx)

	deployment := &appsv1.Deployment{}
	if err := r.Get(ctx, types.NamespacedName{Namespace: namespace, Name: deploymentName}, deployment); err != nil {
		if apierrors.IsNotFound(err) {
			return "deployment not found", 0, fmt.Errorf("deployment %s/%s not found", namespace, deploymentName)
		}
		return "error", 0, fmt.Errorf("failed to get deployment: %w", err)
	}

	currentReplicas := int32(0)
	if deployment.Spec.Replicas != nil {
		currentReplicas = *deployment.Spec.Replicas
	}

	// Check if scaling is needed
	if currentReplicas == desiredReplicas {
		return fmt.Sprintf("no change needed (replicas=%d)", desiredReplicas), desiredReplicas, nil
	}

	// Scale the deployment
	log.Info("Scaling deployment", "namespace", namespace, "deployment", deploymentName,
		"from", currentReplicas, "to", desiredReplicas)

	deployment.Spec.Replicas = &desiredReplicas
	if err := r.Update(ctx, deployment); err != nil {
		return "scale failed", currentReplicas, fmt.Errorf("failed to scale deployment: %w", err)
	}

	return fmt.Sprintf("scaled from %d to %d", currentReplicas, desiredReplicas), desiredReplicas, nil
}

// ensureNamespace creates the namespace if it doesn't exist
func (r *WorkloadScheduleReconciler) ensureNamespace(ctx context.Context, namespace string) error {
	ns := &corev1.Namespace{}
	err := r.Get(ctx, types.NamespacedName{Name: namespace}, ns)
	if err == nil {
		return nil // Namespace exists
	}

	if !apierrors.IsNotFound(err) {
		return fmt.Errorf("failed to check namespace: %w", err)
	}

	// Create namespace
	ns = &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: namespace,
		},
	}

	if err := r.Create(ctx, ns); err != nil && !apierrors.IsAlreadyExists(err) {
		return fmt.Errorf("failed to create namespace: %w", err)
	}

	return nil
}

// cleanupResources performs cleanup when the WorkloadSchedule is deleted
func (r *WorkloadScheduleReconciler) cleanupResources(ctx context.Context, workloadSchedule *infrav1alpha1.WorkloadSchedule) error {
	log := logf.FromContext(ctx)
	log.Info("Cleaning up resources for WorkloadSchedule", "name", workloadSchedule.Name)

	// Optionally scale the deployment back to a default value (e.g., 1) on deletion
	// For now, we just log the cleanup
	log.Info("Cleanup completed")
	return nil
}

// setCondition sets a condition on the WorkloadSchedule status
func (r *WorkloadScheduleReconciler) setCondition(ws *infrav1alpha1.WorkloadSchedule, conditionType string, status metav1.ConditionStatus, reason, message string) {
	meta.SetStatusCondition(&ws.Status.Conditions, metav1.Condition{
		Type:               conditionType,
		Status:             status,
		Reason:             reason,
		Message:            message,
		LastTransitionTime: metav1.Now(),
	})
}

// SetupWithManager sets up the controller with the Manager.
func (r *WorkloadScheduleReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&infrav1alpha1.WorkloadSchedule{}).
		Named("workloadschedule").
		Complete(r)
}

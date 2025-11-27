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

package v1

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"

	infrav1alpha1 "github.com/vmovahed/workload-schedule-operator/api/v1alpha1"
)

const (
	// ActiveLabel is the label injected into Pods
	ActiveLabel = "schedule.illumin.com/active"

	// ActiveEnvVar is the environment variable injected into containers
	ActiveEnvVar = "WORKLOAD_SCHEDULE_ACTIVE"
)

// log is for logging in this package.
var podlog = logf.Log.WithName("pod-webhook")

// PodCustomDefaulter struct is responsible for setting default values on Pods
type PodCustomDefaulter struct {
	Client client.Client
}

var _ webhook.CustomDefaulter = &PodCustomDefaulter{}

// SetupPodWebhookWithManager registers the webhook for Pod in the manager.
func SetupPodWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).For(&corev1.Pod{}).
		WithDefaulter(&PodCustomDefaulter{Client: mgr.GetClient()}).
		Complete()
}

// +kubebuilder:webhook:path=/mutate--v1-pod,mutating=true,failurePolicy=ignore,sideEffects=None,groups="",resources=pods,verbs=create,versions=v1,name=mpod-v1.kb.io,admissionReviewVersions=v1

// Default implements webhook.CustomDefaulter so a webhook will be registered for the Kind Pod.
func (d *PodCustomDefaulter) Default(ctx context.Context, obj runtime.Object) error {
	pod, ok := obj.(*corev1.Pod)
	if !ok {
		return fmt.Errorf("expected a Pod object but got %T", obj)
	}

	podlog.Info("Mutating Pod", "name", pod.GetName(), "namespace", pod.GetNamespace())

	// Get the namespace of the pod
	namespace := pod.GetNamespace()
	if namespace == "" {
		// Pod namespace might be empty during admission, try to get from request context
		podlog.Info("Pod namespace is empty, skipping mutation")
		return nil
	}

	// Find WorkloadSchedule that targets this namespace
	workloadSchedules := &infrav1alpha1.WorkloadScheduleList{}
	if err := d.Client.List(ctx, workloadSchedules); err != nil {
		podlog.Error(err, "Failed to list WorkloadSchedules")
		// Don't block pod creation on error
		return nil
	}

	// Check if any WorkloadSchedule targets this namespace
	var matchingSchedule *infrav1alpha1.WorkloadSchedule
	for i := range workloadSchedules.Items {
		ws := &workloadSchedules.Items[i]
		if ws.Spec.TargetNamespace == namespace {
			matchingSchedule = ws
			break
		}
	}

	if matchingSchedule == nil {
		podlog.Info("No WorkloadSchedule found for namespace, skipping mutation", "namespace", namespace)
		return nil
	}

	podlog.Info("Found matching WorkloadSchedule", "name", matchingSchedule.Name, "namespace", namespace)

	// Determine the active status from the WorkloadSchedule status
	isActive := matchingSchedule.Status.WithinActiveWindow
	activeValue := "false"
	if isActive {
		activeValue = "true"
	}

	// Inject label
	if pod.Labels == nil {
		pod.Labels = make(map[string]string)
	}
	pod.Labels[ActiveLabel] = activeValue

	// Inject environment variable into all containers
	envVar := corev1.EnvVar{
		Name:  ActiveEnvVar,
		Value: activeValue,
	}

	for i := range pod.Spec.Containers {
		// Check if env var already exists
		found := false
		for j := range pod.Spec.Containers[i].Env {
			if pod.Spec.Containers[i].Env[j].Name == ActiveEnvVar {
				pod.Spec.Containers[i].Env[j].Value = activeValue
				found = true
				break
			}
		}
		if !found {
			pod.Spec.Containers[i].Env = append(pod.Spec.Containers[i].Env, envVar)
		}
	}

	// Also inject into init containers
	for i := range pod.Spec.InitContainers {
		found := false
		for j := range pod.Spec.InitContainers[i].Env {
			if pod.Spec.InitContainers[i].Env[j].Name == ActiveEnvVar {
				pod.Spec.InitContainers[i].Env[j].Value = activeValue
				found = true
				break
			}
		}
		if !found {
			pod.Spec.InitContainers[i].Env = append(pod.Spec.InitContainers[i].Env, envVar)
		}
	}

	podlog.Info("Successfully mutated Pod", "name", pod.GetName(), "namespace", namespace, "active", activeValue)
	return nil
}

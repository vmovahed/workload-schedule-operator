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

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// WorkloadScheduleSpec defines the desired state of WorkloadSchedule
type WorkloadScheduleSpec struct {
	// Timezone specifies the timezone to use for scheduling (e.g., "America/Toronto")
	// This timezone will be used to query the worldtimeapi.org API
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Timezone string `json:"timezone"`

	// StartHour is the hour (0-23) when the active window begins (inclusive)
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=23
	StartHour int `json:"startHour"`

	// EndHour is the hour (0-23) when the active window ends (exclusive)
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=24
	EndHour int `json:"endHour"`

	// TargetNamespace is the namespace where the target deployment resides
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	TargetNamespace string `json:"targetNamespace"`

	// TargetDeployment is the name of the deployment to scale
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	TargetDeployment string `json:"targetDeployment"`

	// ReplicasWhenActive is the number of replicas when within the active window
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Minimum=1
	ReplicasWhenActive int32 `json:"replicasWhenActive"`
}

// WorkloadScheduleStatus defines the observed state of WorkloadSchedule
type WorkloadScheduleStatus struct {
	// CurrentLocalTime is the current local time in the specified timezone
	// +optional
	CurrentLocalTime string `json:"currentLocalTime,omitempty"`

	// WithinActiveWindow indicates whether the current time is within the active window
	// +optional
	WithinActiveWindow bool `json:"withinActiveWindow"`

	// LastScaleAction describes the last scaling action taken
	// +optional
	LastScaleAction string `json:"lastScaleAction,omitempty"`

	// LastSyncTime is the timestamp of the last successful reconciliation
	// +optional
	LastSyncTime *metav1.Time `json:"lastSyncTime,omitempty"`

	// CurrentReplicas is the current number of replicas of the target deployment
	// +optional
	CurrentReplicas int32 `json:"currentReplicas"`

	// Conditions represent the current state of the WorkloadSchedule resource
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Timezone",type=string,JSONPath=`.spec.timezone`
// +kubebuilder:printcolumn:name="Active",type=boolean,JSONPath=`.status.withinActiveWindow`
// +kubebuilder:printcolumn:name="Replicas",type=integer,JSONPath=`.status.currentReplicas`
// +kubebuilder:printcolumn:name="Last Sync",type=date,JSONPath=`.status.lastSyncTime`

// WorkloadSchedule is the Schema for the workloadschedules API
type WorkloadSchedule struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   WorkloadScheduleSpec   `json:"spec,omitempty"`
	Status WorkloadScheduleStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// WorkloadScheduleList contains a list of WorkloadSchedule
type WorkloadScheduleList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []WorkloadSchedule `json:"items"`
}

func init() {
	SchemeBuilder.Register(&WorkloadSchedule{}, &WorkloadScheduleList{})
}

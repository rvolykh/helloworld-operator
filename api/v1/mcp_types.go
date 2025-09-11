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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type DeplomentMCPType string

const (
	Internal DeplomentMCPType = "internal"
	External DeplomentMCPType = "external"
)

const (
	// MCPConditionTypeAvailable indicates that the MCP is fully functional
	MCPConditionTypeAvailable = "Available"
	// MCPConditionTypeProgressing indicates that the MCP is being created or updated
	MCPConditionTypeProgressing = "Progressing"
	// MCPConditionTypeDegraded indicates that the MCP failed to reach or maintain its desired state
	MCPConditionTypeDegraded = "Degraded"

	// MCPConditionReasonDeploymentStatus indicates current status of owned deployment resource
	MCPConditionReasonDeploymentStatus = "DeploymentStatus"
)

type InternalMCPSpec struct {
	// +kubebuilder:title:=Docker image of the MCP to deploy
	// +kubebuilder:validation:Required
	Image string `json:"image"`
}

type ExternalMCPSpec struct {
	// +kubebuilder:title:=URL of the deployed MCP
	// +kubebuilder:validation:Required
	URL string `json:"url"`
}

// MCPSpec defines the desired state of MCP
type MCPSpec struct {
	// +kubebuilder:title:=Whether the MCP has to be deployed internally or it is already deployed externally
	// +kubebuilder:validation:Optional
	// +kubebuilder:default:=internal
	// +kubebuilder:validation:Enum=internal;external
	Type *DeplomentMCPType `json:"type,omitempty"`

	// +kubebuilder:title:=Internal MCP spec
	// +kubebuilder:validation:Optional
	Internal *InternalMCPSpec `json:"internal,omitempty"`

	// +kubebuilder:title:=External MCP spec
	// +kubebuilder:validation:Optional
	External *ExternalMCPSpec `json:"external,omitempty"`
}

type MCPInfo struct {
	// +kubebuilder:title:=URL of the deployed MCP
	URL string `json:"url"`
}

// MCPStatus defines the observed state of MCP.
type MCPStatus struct {
	// conditions represent the current state of the MCP resource.
	// Each condition has a unique type and reflects the status of a specific aspect of the resource.
	//
	// Standard condition types include:
	// - "Available": the resource is fully functional
	// - "Progressing": the resource is being created or updated
	// - "Degraded": the resource failed to reach or maintain its desired state
	//
	// The status of each condition is one of True, False, or Unknown.
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// +kubebuilder:title:=Info about the deployed MCP
	// +kubebuilder:validation:Optional
	Info *MCPInfo `json:"info,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// MCP is the Schema for the mcps API
type MCP struct {
	metav1.TypeMeta `json:",inline"`

	// metadata is a standard object metadata
	// +optional
	metav1.ObjectMeta `json:"metadata,omitempty,omitzero"`

	// spec defines the desired state of MCP
	// +required
	Spec MCPSpec `json:"spec"`

	// status defines the observed state of MCP
	// +optional
	Status MCPStatus `json:"status,omitempty,omitzero"`
}

// +kubebuilder:object:root=true

// MCPList contains a list of MCP
type MCPList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []MCP `json:"items"`
}

func init() {
	SchemeBuilder.Register(&MCP{}, &MCPList{})
}

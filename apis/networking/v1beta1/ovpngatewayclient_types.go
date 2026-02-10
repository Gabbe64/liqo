// Copyright 2019-2026 The Liqo Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package v1beta1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// OvpnGatewayClientResource the name of the ovpngatewayclient resources.
var OvpnGatewayClientResource = "ovpngatewayclients"

// OvpnGatewayClientKind is the kind name used to register the OvpnGatewayClient CRD.
var OvpnGatewayClientKind = "OvpnGatewayClient"

// OvpnGatewayClientGroupResource is group resource used to register these objects.
var OvpnGatewayClientGroupResource = schema.GroupResource{Group: GroupVersion.Group, Resource: OvpnGatewayClientResource}

// OvpnGatewayClientGroupVersionResource is groupResourceVersion used to register these objects.
var OvpnGatewayClientGroupVersionResource = GroupVersion.WithResource(OvpnGatewayClientResource)

// OvpnGatewayClientSpec defines the desired state of OvpnGatewayClient.
type OvpnGatewayClientSpec struct {
	// Deployment specifies the deployment template for the client.
	Deployment DeploymentTemplate `json:"deployment"`
	// Metrics specifies the metrics configuration for the client.
	Metrics *Metrics `json:"metrics,omitempty"`
	// SecretRef specifies the reference to the secret containing the openvpn configuration.
	// Leave it empty to let the operator create a new secret.
	SecretRef corev1.LocalObjectReference `json:"secretRef,omitempty"`
}

// OvpnGatewayClientStatus defines the observed state of OvpnGatewayClient.
type OvpnGatewayClientStatus struct {
	// SecretRef specifies the reference to the secret.
	SecretRef *corev1.ObjectReference `json:"secretRef,omitempty"`
	// InternalEndpoint specifies the endpoint for the internal network.
	InternalEndpoint *InternalGatewayEndpoint `json:"internalEndpoint,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:categories=liqo,shortName=ovpnc;oc
// +kubebuilder:subresource:status

// OvpnGatewayClient defines a openvpn gateway client that needs to point to a remote openvpn gateway server.
type OvpnGatewayClient struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   OvpnGatewayClientSpec   `json:"spec,omitempty"`
	Status OvpnGatewayClientStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// OvpnGatewayClientList contains a list of OvpnGatewayClient.
type OvpnGatewayClientList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []OvpnGatewayClient `json:"items"`
}

func init() {
	SchemeBuilder.Register(&OvpnGatewayClient{}, &OvpnGatewayClientList{})
}

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

// OvpnGatewayServerResource the name of the ovpngatewayserver resources.
var OvpnGatewayServerResource = "ovpngatewayservers"

// OvpnGatewayServerKind specifies the kind of the ovpngatewayserver resources.
var OvpnGatewayServerKind = "OvpnGatewayServer"

// OvpnGatewayServerGroupResource specifies the group and the resource of the ovpngatewayserver resources.
var OvpnGatewayServerGroupResource = schema.GroupResource{Group: GroupVersion.Group, Resource: OvpnGatewayServerResource}

// OvpnGatewayServerGroupVersionResource specifies the group, the version and the resource of the ovpngatewayserver resources.
var OvpnGatewayServerGroupVersionResource = GroupVersion.WithResource(OvpnGatewayServerResource)

// OvpnGatewayServerSpec defines the desired state of OvpnGatewayServer.
type OvpnGatewayServerSpec struct {
	// Service specifies the service template for the server.
	Service ServiceTemplate `json:"service"`
	// Deployment specifies the deployment template for the server.
	Deployment DeploymentTemplate `json:"deployment"`
	// Metrics specifies the metrics configuration for the server.
	Metrics *Metrics `json:"metrics,omitempty"`
	// SecretRef specifies the reference to the secret containing the openvpn configuration.
	// Leave it empty to let the operator create a new secret.
	SecretRef corev1.LocalObjectReference `json:"secretRef,omitempty"`
}

// OvpnGatewayServerStatus defines the observed state of OvpnGatewayServer.
type OvpnGatewayServerStatus struct {
	// SecretRef specifies the reference to the secret.
	SecretRef *corev1.ObjectReference `json:"secretRef,omitempty"`
	// Endpoint specifies the endpoint of the server.
	Endpoint *EndpointStatus `json:"endpoint,omitempty"`
	// InternalEndpoint specifies the endpoint for the internal network.
	InternalEndpoint *InternalGatewayEndpoint `json:"internalEndpoint,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:categories=liqo,shortName=ovpns;os
// +kubebuilder:subresource:status

// OvpnGatewayServer defines a openvpn gateway server that will accept connections from remote openvpn gateway clients.
type OvpnGatewayServer struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   OvpnGatewayServerSpec   `json:"spec,omitempty"`
	Status OvpnGatewayServerStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// OvpnGatewayServerList contains a list of OvpnGatewayServer.
type OvpnGatewayServerList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []OvpnGatewayServer `json:"items"`
}

func init() {
	SchemeBuilder.Register(&OvpnGatewayServer{}, &OvpnGatewayServerList{})
}

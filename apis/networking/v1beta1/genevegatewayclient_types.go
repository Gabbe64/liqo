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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// GeneveGatewayClientResource the name of the genevegatewayclient resources.
var GeneveGatewayClientResource = "genevegatewayclients"

// GeneveGatewayClientKind specifies the kind of the genevegatewayclient resources.
var GeneveGatewayClientKind = "GeneveGatewayClient"

// GeneveGatewayClientGroupResource specifies the group and the resource of the genevegatewayclient resources.
var GeneveGatewayClientGroupResource = schema.GroupResource{Group: GroupVersion.Group, Resource: GeneveGatewayClientResource}

// GeneveGatewayClientGroupVersionResource specifies the group, the version and the resource of the genevegatewayclient resources.
var GeneveGatewayClientGroupVersionResource = GroupVersion.WithResource(GeneveGatewayClientResource)

// GeneveGatewayClientSpec defines the desired state of GeneveGatewayClient.
//
// Unlike the WireGuard client, the Geneve client is not a pure initiator: the
// server replies to a configured endpoint rather than to the observed source,
// so the client must be reachable and the Service is mandatory.
type GeneveGatewayClientSpec struct {
	// Service specifies the service template for the client. The server needs to
	// reach the client at a known endpoint, so it is required.
	Service ServiceTemplate `json:"service"`
	// Deployment specifies the deployment template for the client.
	Deployment DeploymentTemplate `json:"deployment"`
	// Metrics specifies the metrics configuration for the client.
	Metrics *Metrics `json:"metrics,omitempty"`
}

// GeneveGatewayClientStatus defines the observed state of GeneveGatewayClient.
type GeneveGatewayClientStatus struct {
	// Endpoint specifies the endpoint at which the client is reachable.
	Endpoint *EndpointStatus `json:"endpoint,omitempty"`
	// InternalEndpoint specifies the endpoint for the internal network.
	InternalEndpoint *InternalGatewayEndpoint `json:"internalEndpoint,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:categories=liqo,shortName=ggc;genevegc
// +kubebuilder:subresource:status

// GeneveGatewayClient defines a Geneve gateway client that connects to a remote Geneve gateway server.
type GeneveGatewayClient struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   GeneveGatewayClientSpec   `json:"spec,omitempty"`
	Status GeneveGatewayClientStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// GeneveGatewayClientList contains a list of GeneveGatewayClient.
type GeneveGatewayClientList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []GeneveGatewayClient `json:"items"`
}

func init() {
	SchemeBuilder.Register(&GeneveGatewayClient{}, &GeneveGatewayClientList{})
}

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

// GeneveGatewayServerResource the name of the genevegatewayserver resources.
var GeneveGatewayServerResource = "genevegatewayservers"

// GeneveGatewayServerKind specifies the kind of the genevegatewayserver resources.
var GeneveGatewayServerKind = "GeneveGatewayServer"

// GeneveGatewayServerGroupResource specifies the group and the resource of the genevegatewayserver resources.
var GeneveGatewayServerGroupResource = schema.GroupResource{Group: GroupVersion.Group, Resource: GeneveGatewayServerResource}

// GeneveGatewayServerGroupVersionResource specifies the group, the version and the resource of the genevegatewayserver resources.
var GeneveGatewayServerGroupVersionResource = GroupVersion.WithResource(GeneveGatewayServerResource)

// GeneveGatewayServerSpec defines the desired state of GeneveGatewayServer.
type GeneveGatewayServerSpec struct {
	// Service specifies the service template for the server.
	Service ServiceTemplate `json:"service"`
	// Deployment specifies the deployment template for the server.
	Deployment DeploymentTemplate `json:"deployment"`
	// Metrics specifies the metrics configuration for the server.
	Metrics *Metrics `json:"metrics,omitempty"`
}

// GeneveGatewayServerStatus defines the observed state of GeneveGatewayServer.
type GeneveGatewayServerStatus struct {
	// Endpoint specifies the endpoint of the server.
	Endpoint *EndpointStatus `json:"endpoint,omitempty"`
	// InternalEndpoint specifies the endpoint for the internal network.
	InternalEndpoint *InternalGatewayEndpoint `json:"internalEndpoint,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:categories=liqo,shortName=ggs;genevegs
// +kubebuilder:subresource:status

// GeneveGatewayServer defines a Geneve gateway server.
//
// The Geneve tunnel has a single mode: both gateways are configured with each
// other's endpoint, and the outer UDP source port is hashed per inner flow so
// that RSS and ECMP can spread the tunnel traffic. The kernel Geneve driver
// offers no way to pin that source port, so both gateways must be mutually
// reachable and the server always exposes a Service.
type GeneveGatewayServer struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   GeneveGatewayServerSpec   `json:"spec,omitempty"`
	Status GeneveGatewayServerStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// GeneveGatewayServerList contains a list of GeneveGatewayServer.
type GeneveGatewayServerList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []GeneveGatewayServer `json:"items"`
}

func init() {
	SchemeBuilder.Register(&GeneveGatewayServer{}, &GeneveGatewayServerList{})
}

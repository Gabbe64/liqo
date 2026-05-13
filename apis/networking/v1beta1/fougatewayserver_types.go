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

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// FouGatewayServerResource the name of the fougatewayserver resources.
var FouGatewayServerResource = "fougatewayservers"

// FouGatewayServerKind specifies the kind of the fougatewayserver resources.
var FouGatewayServerKind = "FouGatewayServer"

// FouGatewayServerGroupResource specifies the group and the resource of the fougatewayserver resources.
var FouGatewayServerGroupResource = schema.GroupResource{Group: GroupVersion.Group, Resource: FouGatewayServerResource}

// FouGatewayServerGroupVersionResource specifies the group, the version and the resource of the fougatewayserver resources.
var FouGatewayServerGroupVersionResource = GroupVersion.WithResource(FouGatewayServerResource)

// FouGatewayServerSpec defines the desired state of FouGatewayServer.
type FouGatewayServerSpec struct {
	// Service specifies the service template for the server.
	Service ServiceTemplate `json:"service"`
	// Deployment specifies the deployment template for the server.
	Deployment DeploymentTemplate `json:"deployment"`
	// Metrics specifies the metrics configuration for the server.
	Metrics *Metrics `json:"metrics,omitempty"`
}

// FouGatewayServerStatus defines the observed state of FouGatewayServer.
type FouGatewayServerStatus struct {
	// Endpoint specifies the endpoint of the server.
	Endpoint *EndpointStatus `json:"endpoint,omitempty"`
	// InternalEndpoint specifies the endpoint for the internal network.
	InternalEndpoint *InternalGatewayEndpoint `json:"internalEndpoint,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:categories=liqo,shortName=fgs;fougs
// +kubebuilder:subresource:status

// FouGatewayServer defines a FOU gateway server that will accept connections from remote FOU gateway clients.
type FouGatewayServer struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   FouGatewayServerSpec   `json:"spec,omitempty"`
	Status FouGatewayServerStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// FouGatewayServerList contains a list of FouGatewayServer.
type FouGatewayServerList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []FouGatewayServer `json:"items"`
}

func init() {
	SchemeBuilder.Register(&FouGatewayServer{}, &FouGatewayServerList{})
}

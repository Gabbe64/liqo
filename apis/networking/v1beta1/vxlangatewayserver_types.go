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

// VxlanGatewayServerResource the name of the vxlangatewayserver resources.
var VxlanGatewayServerResource = "vxlangatewayservers"

// VxlanGatewayServerKind specifies the kind of the vxlangatewayserver resources.
var VxlanGatewayServerKind = "VxlanGatewayServer"

// VxlanGatewayServerGroupResource specifies the group and the resource of the vxlangatewayserver resources.
var VxlanGatewayServerGroupResource = schema.GroupResource{Group: GroupVersion.Group, Resource: VxlanGatewayServerResource}

// VxlanGatewayServerGroupVersionResource specifies the group, the version and the resource of the vxlangatewayserver resources.
var VxlanGatewayServerGroupVersionResource = GroupVersion.WithResource(VxlanGatewayServerResource)

// VxlanGatewayServerSpec defines the desired state of VxlanGatewayServer.
type VxlanGatewayServerSpec struct {
	// Service specifies the service template for the server.
	Service ServiceTemplate `json:"service"`
	// Deployment specifies the deployment template for the server.
	Deployment DeploymentTemplate `json:"deployment"`
	// Metrics specifies the metrics configuration for the server.
	Metrics *Metrics `json:"metrics,omitempty"`
}

// VxlanGatewayServerStatus defines the observed state of VxlanGatewayServer.
type VxlanGatewayServerStatus struct {
	// Endpoint specifies the endpoint of the server.
	Endpoint *EndpointStatus `json:"endpoint,omitempty"`
	// InternalEndpoint specifies the endpoint for the internal network.
	InternalEndpoint *InternalGatewayEndpoint `json:"internalEndpoint,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:categories=liqo,shortName=vgs;vxlangs
// +kubebuilder:subresource:status

// VxlanGatewayServer defines a VXLAN gateway server that will accept connections from remote VXLAN gateway clients.
type VxlanGatewayServer struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   VxlanGatewayServerSpec   `json:"spec,omitempty"`
	Status VxlanGatewayServerStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// VxlanGatewayServerList contains a list of VxlanGatewayServer.
type VxlanGatewayServerList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []VxlanGatewayServer `json:"items"`
}

func init() {
	SchemeBuilder.Register(&VxlanGatewayServer{}, &VxlanGatewayServerList{})
}

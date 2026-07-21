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

// VxlanGatewayClientResource the name of the vxlangatewayclient resources.
var VxlanGatewayClientResource = "vxlangatewayclients"

// VxlanGatewayClientKind specifies the kind of the vxlangatewayclient resources.
var VxlanGatewayClientKind = "VxlanGatewayClient"

// VxlanGatewayClientGroupResource specifies the group and the resource of the vxlangatewayclient resources.
var VxlanGatewayClientGroupResource = schema.GroupResource{Group: GroupVersion.Group, Resource: VxlanGatewayClientResource}

// VxlanGatewayClientGroupVersionResource specifies the group, the version and the resource of the vxlangatewayclient resources.
var VxlanGatewayClientGroupVersionResource = GroupVersion.WithResource(VxlanGatewayClientResource)

// VxlanGatewayClientSpec defines the desired state of VxlanGatewayClient.
//
// The client is a pure initiator: it needs no Service and no inbound
// reachability. The server learns the client endpoint from the data plane.
type VxlanGatewayClientSpec struct {
	// Deployment specifies the deployment template for the client.
	Deployment DeploymentTemplate `json:"deployment"`
	// Metrics specifies the metrics configuration for the client.
	Metrics *Metrics `json:"metrics,omitempty"`
}

// VxlanGatewayClientStatus defines the observed state of VxlanGatewayClient.
type VxlanGatewayClientStatus struct {
	// InternalEndpoint specifies the endpoint for the internal network.
	InternalEndpoint *InternalGatewayEndpoint `json:"internalEndpoint,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:categories=liqo,shortName=vgc;vxlangc
// +kubebuilder:subresource:status

// VxlanGatewayClient defines a VXLAN gateway client that connects to a remote VXLAN gateway server.
type VxlanGatewayClient struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   VxlanGatewayClientSpec   `json:"spec,omitempty"`
	Status VxlanGatewayClientStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// VxlanGatewayClientList contains a list of VxlanGatewayClient.
type VxlanGatewayClientList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []VxlanGatewayClient `json:"items"`
}

func init() {
	SchemeBuilder.Register(&VxlanGatewayClient{}, &VxlanGatewayClientList{})
}

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

// FouGatewayClientResource the name of the fougatewayclient resources.
var FouGatewayClientResource = "fougatewayclients"

// FouGatewayClientKind is the kind name used to register the FouGatewayClient CRD.
var FouGatewayClientKind = "FouGatewayClient"

// FouGatewayClientGroupResource is group resource used to register these objects.
var FouGatewayClientGroupResource = schema.GroupResource{Group: GroupVersion.Group, Resource: FouGatewayClientResource}

// FouGatewayClientGroupVersionResource is groupResourceVersion used to register these objects.
var FouGatewayClientGroupVersionResource = GroupVersion.WithResource(FouGatewayClientResource)

// FouGatewayClientSpec defines the desired state of FouGatewayClient.
type FouGatewayClientSpec struct {
	// Service specifies the service template for the client.
	Service ServiceTemplate `json:"service"`
	// Deployment specifies the deployment template for the client.
	Deployment DeploymentTemplate `json:"deployment"`
	// Metrics specifies the metrics configuration for the client.
	Metrics *Metrics `json:"metrics,omitempty"`
}

// FouGatewayClientStatus defines the observed state of FouGatewayClient.
type FouGatewayClientStatus struct {
	// Endpoint specifies the external endpoint of this client as seen from the remote cluster.
	// The remote FOU gateway server should be configured with this endpoint for return-path routing.
	Endpoint *EndpointStatus `json:"endpoint,omitempty"`
	// InternalEndpoint specifies the endpoint for the internal network.
	InternalEndpoint *InternalGatewayEndpoint `json:"internalEndpoint,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:categories=liqo,shortName=fgc;fougc
// +kubebuilder:subresource:status

// FouGatewayClient defines a FOU gateway client that needs to point to a remote FOU gateway server.
type FouGatewayClient struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   FouGatewayClientSpec   `json:"spec,omitempty"`
	Status FouGatewayClientStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// FouGatewayClientList contains a list of FouGatewayClient.
type FouGatewayClientList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []FouGatewayClient `json:"items"`
}

func init() {
	SchemeBuilder.Register(&FouGatewayClient{}, &FouGatewayClientList{})
}

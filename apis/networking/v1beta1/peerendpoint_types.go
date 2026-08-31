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

// PeerEndpointResource the name of the peerendpoint resources.
var PeerEndpointResource = "peerendpoints"

// PeerEndpointKind is the kind name used to register the PeerEndpoint CRD.
var PeerEndpointKind = "PeerEndpoint"

// PeerEndpointGroupResource is group resource used to register these objects.
var PeerEndpointGroupResource = schema.GroupResource{Group: GroupVersion.Group, Resource: PeerEndpointResource}

// PeerEndpointGroupVersionResource is groupResourceVersion used to register these objects.
var PeerEndpointGroupVersionResource = GroupVersion.WithResource(PeerEndpointResource)

// PeerEndpointSpec defines the desired state of PeerEndpoint.
type PeerEndpointSpec struct {
	// Endpoint specifies the endpoint at which the remote gateway is reachable.
	Endpoint EndpointStatus `json:"endpoint"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:categories=liqo,shortName=pe;peerep

// PeerEndpoint carries the endpoint of the remote gateway to the local gateway
// runtime. It is required by tunnel technologies that cannot discover the peer
// endpoint from the data plane, i.e. those configuring both sides statically
// (Geneve) so that the outer source port can be hashed and RSS/ECMP can spread
// the tunnel traffic.
//
// It is watched by the gateway runtime, so the endpoint can be updated without
// restarting the gateway pod.
type PeerEndpoint struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec PeerEndpointSpec `json:"spec,omitempty"`
}

// +kubebuilder:object:root=true

// PeerEndpointList contains a list of PeerEndpoint.
type PeerEndpointList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []PeerEndpoint `json:"items"`
}

func init() {
	SchemeBuilder.Register(&PeerEndpoint{}, &PeerEndpointList{})
}

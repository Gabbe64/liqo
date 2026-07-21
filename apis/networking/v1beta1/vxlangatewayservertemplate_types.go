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
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// VxlanGatewayServerTemplateResource the name of the vxlangatewayservertemplate resources.
var VxlanGatewayServerTemplateResource = "vxlangatewayservertemplates"

// VxlanGatewayServerTemplateKind is the kind name used to register the VxlanGatewayServerTemplate CRD.
var VxlanGatewayServerTemplateKind = "VxlanGatewayServerTemplate"

// VxlanGatewayServerTemplateGroupResource is group resource used to register these objects.
var VxlanGatewayServerTemplateGroupResource = schema.GroupResource{Group: GroupVersion.Group, Resource: VxlanGatewayServerTemplateResource}

// VxlanGatewayServerTemplateGroupVersionResource is groupResourceVersion used to register these objects.
var VxlanGatewayServerTemplateGroupVersionResource = GroupVersion.WithResource(VxlanGatewayServerTemplateResource)

// VxlanGatewayServerTemplateSpec defines the desired state of VxlanGatewayServerTemplate.
type VxlanGatewayServerTemplateSpec struct {
	// ObjectKind specifies the kind of the object.
	ObjectKind metav1.TypeMeta `json:"objectKind,omitempty"`
	// Template specifies the template of the server.
	// +kubebuilder:pruning:PreserveUnknownFields
	Template unstructured.Unstructured `json:"template,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:categories=liqo,shortName=vgst;vxlangst

// VxlanGatewayServerTemplate contains a template for a VXLAN gateway server.
type VxlanGatewayServerTemplate struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec VxlanGatewayServerTemplateSpec `json:"spec,omitempty"`
}

// +kubebuilder:object:root=true

// VxlanGatewayServerTemplateList contains a list of VxlanGatewayServerTemplate.
type VxlanGatewayServerTemplateList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []VxlanGatewayServerTemplate `json:"items"`
}

func init() {
	SchemeBuilder.Register(&VxlanGatewayServerTemplate{}, &VxlanGatewayServerTemplateList{})
}

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

// VxlanGatewayClientTemplateResource the name of the vxlangatewayclienttemplate resources.
var VxlanGatewayClientTemplateResource = "vxlangatewayclienttemplates"

// VxlanGatewayClientTemplateKind is the kind name used to register the VxlanGatewayClientTemplate CRD.
var VxlanGatewayClientTemplateKind = "VxlanGatewayClientTemplate"

// VxlanGatewayClientTemplateGroupResource is group resource used to register these objects.
var VxlanGatewayClientTemplateGroupResource = schema.GroupResource{Group: GroupVersion.Group, Resource: VxlanGatewayClientTemplateResource}

// VxlanGatewayClientTemplateGroupVersionResource is groupResourceVersion used to register these objects.
var VxlanGatewayClientTemplateGroupVersionResource = GroupVersion.WithResource(VxlanGatewayClientTemplateResource)

// VxlanGatewayClientTemplateSpec defines the desired state of VxlanGatewayClientTemplate.
type VxlanGatewayClientTemplateSpec struct {
	// ObjectKind specifies the kind of the object.
	ObjectKind metav1.TypeMeta `json:"objectKind,omitempty"`
	// Template specifies the template of the client.
	// +kubebuilder:pruning:PreserveUnknownFields
	Template unstructured.Unstructured `json:"template,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:categories=liqo,shortName=vgct;vxlangct

// VxlanGatewayClientTemplate contains a template for a VXLAN gateway client.
type VxlanGatewayClientTemplate struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec VxlanGatewayClientTemplateSpec `json:"spec,omitempty"`
}

// +kubebuilder:object:root=true

// VxlanGatewayClientTemplateList contains a list of VxlanGatewayClientTemplate.
type VxlanGatewayClientTemplateList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []VxlanGatewayClientTemplate `json:"items"`
}

func init() {
	SchemeBuilder.Register(&VxlanGatewayClientTemplate{}, &VxlanGatewayClientTemplateList{})
}

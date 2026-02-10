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

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// OvpnGatewayServerTemplateResource the name of the ovpngatewayservertemplate resources.
var OvpnGatewayServerTemplateResource = "ovpngatewayservertemplates"

// OvpnGatewayServerTemplateKind is the kind name used to register the OvpnGatewayServerTemplate CRD.
var OvpnGatewayServerTemplateKind = "OvpnGatewayServerTemplate"

// OvpnGatewayServerTemplateGroupResource is group resource used to register these objects.
var OvpnGatewayServerTemplateGroupResource = schema.GroupResource{Group: GroupVersion.Group, Resource: OvpnGatewayServerTemplateResource}

// OvpnGatewayServerTemplateGroupVersionResource is groupResourceVersion used to register these objects.
var OvpnGatewayServerTemplateGroupVersionResource = GroupVersion.WithResource(OvpnGatewayServerTemplateResource)

// OvpnGatewayServerTemplateSpec defines the desired state of OvpnGatewayServerTemplate.
type OvpnGatewayServerTemplateSpec struct {
	// ObjectKind specifies the kind of the object.
	ObjectKind metav1.TypeMeta `json:"objectKind,omitempty"`
	// Template specifies the template of the server.
	// +kubebuilder:pruning:PreserveUnknownFields
	Template unstructured.Unstructured `json:"template,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:categories=liqo,shortName=ovpnst;ost

// OvpnGatewayServerTemplate contains a template for a openvpn gateway server.
type OvpnGatewayServerTemplate struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec OvpnGatewayServerTemplateSpec `json:"spec,omitempty"`
}

// +kubebuilder:object:root=true

// OvpnGatewayServerTemplateList contains a list of OvpnGatewayServerTemplate.
type OvpnGatewayServerTemplateList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []OvpnGatewayServerTemplate `json:"items"`
}

func init() {
	SchemeBuilder.Register(&OvpnGatewayServerTemplate{}, &OvpnGatewayServerTemplateList{})
}

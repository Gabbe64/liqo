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

// GeneveGatewayServerTemplateResource the name of the genevegatewayservertemplate resources.
var GeneveGatewayServerTemplateResource = "genevegatewayservertemplates"

// GeneveGatewayServerTemplateKind is the kind name used to register the GeneveGatewayServerTemplate CRD.
var GeneveGatewayServerTemplateKind = "GeneveGatewayServerTemplate"

// GeneveGatewayServerTemplateGroupResource is group resource used to register these objects.
var GeneveGatewayServerTemplateGroupResource = schema.GroupResource{Group: GroupVersion.Group, Resource: GeneveGatewayServerTemplateResource}

// GeneveGatewayServerTemplateGroupVersionResource is groupResourceVersion used to register these objects.
var GeneveGatewayServerTemplateGroupVersionResource = GroupVersion.WithResource(GeneveGatewayServerTemplateResource)

// GeneveGatewayServerTemplateSpec defines the desired state of GeneveGatewayServerTemplate.
type GeneveGatewayServerTemplateSpec struct {
	// ObjectKind specifies the kind of the object.
	ObjectKind metav1.TypeMeta `json:"objectKind,omitempty"`
	// Template specifies the template of the server.
	// +kubebuilder:pruning:PreserveUnknownFields
	Template unstructured.Unstructured `json:"template,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:categories=liqo,shortName=ggst;geneveggst

// GeneveGatewayServerTemplate contains a template for a Geneve gateway server.
type GeneveGatewayServerTemplate struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec GeneveGatewayServerTemplateSpec `json:"spec,omitempty"`
}

// +kubebuilder:object:root=true

// GeneveGatewayServerTemplateList contains a list of GeneveGatewayServerTemplate.
type GeneveGatewayServerTemplateList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []GeneveGatewayServerTemplate `json:"items"`
}

func init() {
	SchemeBuilder.Register(&GeneveGatewayServerTemplate{}, &GeneveGatewayServerTemplateList{})
}

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

// GeneveGatewayClientTemplateResource the name of the genevegatewayclienttemplate resources.
var GeneveGatewayClientTemplateResource = "genevegatewayclienttemplates"

// GeneveGatewayClientTemplateKind is the kind name used to register the GeneveGatewayClientTemplate CRD.
var GeneveGatewayClientTemplateKind = "GeneveGatewayClientTemplate"

// GeneveGatewayClientTemplateGroupResource is group resource used to register these objects.
var GeneveGatewayClientTemplateGroupResource = schema.GroupResource{Group: GroupVersion.Group, Resource: GeneveGatewayClientTemplateResource}

// GeneveGatewayClientTemplateGroupVersionResource is groupResourceVersion used to register these objects.
var GeneveGatewayClientTemplateGroupVersionResource = GroupVersion.WithResource(GeneveGatewayClientTemplateResource)

// GeneveGatewayClientTemplateSpec defines the desired state of GeneveGatewayClientTemplate.
type GeneveGatewayClientTemplateSpec struct {
	// ObjectKind specifies the kind of the object.
	ObjectKind metav1.TypeMeta `json:"objectKind,omitempty"`
	// Template specifies the template of the client.
	// +kubebuilder:pruning:PreserveUnknownFields
	Template unstructured.Unstructured `json:"template,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:categories=liqo,shortName=ggct;geneveggct

// GeneveGatewayClientTemplate contains a template for a Geneve gateway client.
type GeneveGatewayClientTemplate struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec GeneveGatewayClientTemplateSpec `json:"spec,omitempty"`
}

// +kubebuilder:object:root=true

// GeneveGatewayClientTemplateList contains a list of GeneveGatewayClientTemplate.
type GeneveGatewayClientTemplateList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []GeneveGatewayClientTemplate `json:"items"`
}

func init() {
	SchemeBuilder.Register(&GeneveGatewayClientTemplate{}, &GeneveGatewayClientTemplateList{})
}

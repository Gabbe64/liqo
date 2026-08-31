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

package network

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"

	networkingv1beta1 "github.com/liqotech/liqo/apis/networking/v1beta1"
)

func TestNetwork(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "liqoctl network test suite")
}

func server(kind string, port int32) *networkingv1beta1.GatewayServer {
	gw := &networkingv1beta1.GatewayServer{}
	gw.Spec.ServerTemplateRef = corev1.ObjectReference{Kind: kind}
	if port != 0 {
		gw.Status.Endpoint = &networkingv1beta1.EndpointStatus{Port: port}
	}
	return gw
}

func gwclient(kind string, port int32) *networkingv1beta1.GatewayClient {
	gw := &networkingv1beta1.GatewayClient{}
	gw.Spec.ClientTemplateRef = corev1.ObjectReference{Kind: kind}
	if port != 0 {
		gw.Status.Endpoint = &networkingv1beta1.EndpointStatus{Port: port}
	}
	return gw
}

const (
	geneveServerKind = "GeneveGatewayServerTemplate"
	geneveClientKind = "GeneveGatewayClientTemplate"
	wgServerKind     = "WgGatewayServerTemplate"
	wgClientKind     = "WgGatewayClientTemplate"
)

var _ = Describe("peeringTunnelKind", func() {
	It("recognises a Geneve pair", func() {
		kind, err := peeringTunnelKind(server(geneveServerKind, 0), gwclient(geneveClientKind, 0))
		Expect(err).NotTo(HaveOccurred())
		Expect(kind).To(Equal(tunnelGeneve))
	})

	It("recognises a WireGuard pair", func() {
		kind, err := peeringTunnelKind(server(wgServerKind, 0), gwclient(wgClientKind, 0))
		Expect(err).NotTo(HaveOccurred())
		Expect(kind).To(Equal(tunnelWireGuard))
	})

	It("treats an unknown template kind as WireGuard, so custom templates keep working", func() {
		kind, err := peeringTunnelKind(server("MyCustomServerTemplate", 0), gwclient("MyCustomClientTemplate", 0))
		Expect(err).NotTo(HaveOccurred())
		Expect(kind).To(Equal(tunnelWireGuard))
	})

	It("rejects a mixed pair, which cannot interoperate", func() {
		_, err := peeringTunnelKind(server(geneveServerKind, 0), gwclient(wgClientKind, 0))
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("same tunnel technology"))
	})
})

var _ = Describe("checkGenevePorts", func() {
	It("accepts equal ports", func() {
		Expect(checkGenevePorts(server(geneveServerKind, 31840), gwclient(geneveClientKind, 31840))).To(Succeed())
	})

	It("rejects unequal ports, which would black-hole one direction", func() {
		err := checkGenevePorts(server(geneveServerKind, 51840), gwclient(geneveClientKind, 31840))
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("--gw-server-service-port 31840"))
	})

	It("stays quiet while an endpoint has not been published yet", func() {
		Expect(checkGenevePorts(server(geneveServerKind, 0), gwclient(geneveClientKind, 31840))).To(Succeed())
		Expect(checkGenevePorts(server(geneveServerKind, 31840), gwclient(geneveClientKind, 0))).To(Succeed())
	})
})

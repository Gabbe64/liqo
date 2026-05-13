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

package route

import (
	"context"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	liqov1beta1 "github.com/liqotech/liqo/apis/core/v1beta1"
	networkingv1beta1 "github.com/liqotech/liqo/apis/networking/v1beta1"
	"github.com/liqotech/liqo/pkg/consts"
	"github.com/liqotech/liqo/pkg/gateway"
	"github.com/liqotech/liqo/pkg/gateway/tunnel"
	cidrutils "github.com/liqotech/liqo/pkg/utils/cidr"
	"github.com/liqotech/liqo/pkg/utils/getters"
	"github.com/liqotech/liqo/pkg/utils/resource"
)

// GenerateRouteConfigurationName generates the name of the RouteConfiguration object.
func GenerateRouteConfigurationName(cfg *networkingv1beta1.Configuration) string {
	return fmt.Sprintf("%s-gw-ext", cfg.Name)
}

// GetRemoteClusterID returns the remote cluster ID of the Configuration.
func GetRemoteClusterID(cfg *networkingv1beta1.Configuration) (liqov1beta1.ClusterID, error) {
	if cfg.GetLabels() == nil {
		return "", fmt.Errorf("configuration %s/%s has no labels", cfg.Namespace, cfg.Name)
	}
	remoteID, ok := cfg.GetLabels()[consts.RemoteClusterID]
	if !ok {
		return "", fmt.Errorf("configuration %s/%s has no remote cluster ID label", cfg.Namespace, cfg.Name)
	}
	return liqov1beta1.ClusterID(remoteID), nil
}

// enforceRouteConfigurationPresence creates or updates a RouteConfiguration object.
func enforceRouteConfigurationPresence(ctx context.Context, cl client.Client, scheme *runtime.Scheme,
	cfg *networkingv1beta1.Configuration) error {
	remoteClusterID, err := GetRemoteClusterID(cfg)
	if err != nil {
		return err
	}

	gwserver, gwclient, err := getters.GetGatewaysByClusterID(ctx, cl, remoteClusterID)
	if err != nil {
		return err
	}

	mode, err := gatewayMode(gwserver, gwclient, remoteClusterID)
	if err != nil {
		return err
	}
	// If the Gateway is not already present, we are not able to understand if it will be a server or a client
	if mode == "" {
		return nil
	}

	remoteInterfaceIP, err := tunnel.GetRemoteInterfaceIP(mode)
	if err != nil {
		return err
	}

	isFoU := isFoUGateway(gwserver, gwclient)

	routecfg := &networkingv1beta1.RouteConfiguration{
		ObjectMeta: metav1.ObjectMeta{
			Name:      GenerateRouteConfigurationName(cfg),
			Namespace: cfg.Namespace,
		},
	}

	internalNodes, err := getters.ListInternalNodesByLabels(ctx, cl, labels.Everything())
	if err != nil {
		return err
	}

	_, err = resource.CreateOrUpdate(ctx, cl, routecfg,
		forgeMutateRouteConfiguration(cfg, routecfg, scheme, remoteClusterID, remoteInterfaceIP, isFoU, internalNodes))
	return err
}

// forgeMutateRouteConfiguration mutates a RouteConfiguration object.
// When isFoU is true (FoU/IPIP tunnel), routes target the tunnel device directly.
// For WireGuard tunnels, routes use the gateway IP.
func forgeMutateRouteConfiguration(cfg *networkingv1beta1.Configuration,
	routecfg *networkingv1beta1.RouteConfiguration, scheme *runtime.Scheme,
	remoteClusterID liqov1beta1.ClusterID,
	remoteInterfaceIP string,
	isFoU bool,
	internalNodes *networkingv1beta1.InternalNodeList) func() error {
	return func() error {
		var err error

		if err = controllerutil.SetOwnerReference(cfg, routecfg, scheme); err != nil {
			return err
		}

		routecfg.ObjectMeta.Labels = gateway.ForgeRouteExternalTargetLabels(string(remoteClusterID))

		routecfg.Spec = networkingv1beta1.RouteConfigurationSpec{
			Table: networkingv1beta1.Table{
				Name: cfg.Name,
			},
		}

		for i := range internalNodes.Items {
			podRoute := forgeExternalRoute(cfg.Spec.Remote.CIDR.Pod, remoteInterfaceIP, isFoU)
			extRoute := forgeExternalRoute(cfg.Spec.Remote.CIDR.External, remoteInterfaceIP, isFoU)

			// The Iif is always the node's Geneve interface: traffic from worker nodes
			// reaches the gateway via Geneve regardless of the tunnel type (WireGuard or FoU).
			// Only the egress device/gateway IP in the route differs between tunnel types.
			iif := &internalNodes.Items[i].Spec.Interface.Gateway.Name

			routecfg.Spec.Table.Rules = append(routecfg.Spec.Table.Rules,
				[]networkingv1beta1.Rule{
					{
						Iif:    iif,
						Dst:    cidrutils.GetPrimary(cfg.Spec.Remote.CIDR.Pod),
						Routes: []networkingv1beta1.Route{podRoute},
					},
					{
						Iif:    iif,
						Dst:    cidrutils.GetPrimary(cfg.Spec.Remote.CIDR.External),
						Routes: []networkingv1beta1.Route{extRoute},
					},
				}...)
		}
		return nil
	}
}

// forgeExternalRoute creates a route for a remote CIDR.
// For FoU/IPIP tunnels (isFoU true), the route targets the tunnel device directly —
// the device's fixed Remote address and FoU encap settings handle egress
// For WireGuard tunnels, it uses the traditional gateway IP (169.254.18.x).
func forgeExternalRoute(cidrs []networkingv1beta1.CIDR,
	remoteInterfaceIP string,
	isFoU bool,
) networkingv1beta1.Route {
	if isFoU {
		return networkingv1beta1.Route{
			Dst: cidrutils.GetPrimary(cidrs),
			Dev: ptr.To(tunnel.TunnelInterfaceName),
		}
	}
	return networkingv1beta1.Route{
		Dst: cidrutils.GetPrimary(cidrs),
		Gw:  ptr.To(networkingv1beta1.IP(remoteInterfaceIP)),
	}
}

// gatewayMode returns the mode of the Gateway from the pre-fetched objects.
func gatewayMode(gwserver *networkingv1beta1.GatewayServer, gwclient *networkingv1beta1.GatewayClient,
	remoteClusterID liqov1beta1.ClusterID) (gateway.Mode, error) {
	switch {
	case gwclient == nil && gwserver == nil:
		return "", nil
	case gwclient != nil && gwserver != nil:
		return "", fmt.Errorf("multiple Gateways found for cluster %s", remoteClusterID)
	case gwclient == nil && gwserver != nil:
		return gateway.ModeServer, nil
	case gwclient != nil && gwserver == nil:
		return gateway.ModeClient, nil
	}
	return "", fmt.Errorf("unable to determine Gateway mode for cluster %s", remoteClusterID)
}

// isFoUGateway returns true when the gateway pair uses FoU tunnelling.
func isFoUGateway(gwserver *networkingv1beta1.GatewayServer, gwclient *networkingv1beta1.GatewayClient) bool {
	if gwserver != nil && gwserver.Spec.ServerTemplateRef.Kind == networkingv1beta1.FouGatewayServerTemplateKind {
		return true
	}
	if gwclient != nil && gwclient.Spec.ClientTemplateRef.Kind == networkingv1beta1.FouGatewayClientTemplateKind {
		return true
	}
	return false
}

// GetGatewayMode returns the mode of the Gateway related to the Configuration.
func GetGatewayMode(ctx context.Context, cl client.Client, remoteClusterID liqov1beta1.ClusterID) (gateway.Mode, error) {
	gwserver, gwclient, err := getters.GetGatewaysByClusterID(ctx, cl, remoteClusterID)
	if err != nil {
		return "", err
	}
	return gatewayMode(gwserver, gwclient, remoteClusterID)
}

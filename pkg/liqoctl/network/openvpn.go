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
	"context"
	"fmt"

	networkingv1beta1 "github.com/liqotech/liqo/apis/networking/v1beta1"
	"github.com/liqotech/liqo/pkg/liqo-controller-manager/networking/forge"
	"github.com/liqotech/liqo/pkg/liqoctl/rest/openvpn"
)

const (
	TunnelingProtocolWireguard = "wireguard"
	TunnelingProtocolOpenVPN   = "openvpn"

	DefaultOpenVPNServerGatewayType  = "networking.liqo.io/v1beta1/ovpngatewayservertemplates"
	DefaultOpenVPNClientGatewayType  = "networking.liqo.io/v1beta1/ovpngatewayclienttemplates"
	DefaultOpenVPNServerTemplateName = "openvpn-gwserver"
	DefaultOpenVPNClientTemplateName = "openvpn-gwclient"
	DefaultOpenVPNServerSecretName   = "openvpn-server"
	DefaultOpenVPNClientSecretName   = "openvpn-client"
)

func (o *Options) usingOpenVPN() bool {
	return o != nil && o.TunnelingProtocol != nil && o.TunnelingProtocol.Value == TunnelingProtocolOpenVPN
}

func (o *Options) applyTunnelingProtocolDefaults() error {
	if o.TunnelingProtocol == nil || o.TunnelingProtocol.Value == "" ||
		o.TunnelingProtocol.Value == TunnelingProtocolWireguard {
		return nil
	}

	switch o.TunnelingProtocol.Value {
	case TunnelingProtocolOpenVPN:
		if o.ServerGatewayType == "" || o.ServerGatewayType == forge.DefaultGwServerType {
			o.ServerGatewayType = DefaultOpenVPNServerGatewayType
		}
		if o.ClientGatewayType == "" || o.ClientGatewayType == forge.DefaultGwClientType {
			o.ClientGatewayType = DefaultOpenVPNClientGatewayType
		}
		if o.ServerTemplateName == "" || o.ServerTemplateName == forge.DefaultGwServerTemplateName {
			o.ServerTemplateName = DefaultOpenVPNServerTemplateName
		}
		if o.ClientTemplateName == "" || o.ClientTemplateName == forge.DefaultGwClientTemplateName {
			o.ClientTemplateName = DefaultOpenVPNClientTemplateName
		}
		return nil
	default:
		return fmt.Errorf("unsupported tunneling protocol %q", o.TunnelingProtocol.Value)
	}
}

func (o *Options) ensureOpenVPNSecrets(ctx context.Context, cluster1, cluster2 *Cluster) error {
	if cluster1 == nil || cluster2 == nil {
		return fmt.Errorf("openvpn secret setup requires initialized clusters")
	}

	clientTenantNs, err := cluster1.localNamespaceManager.GetNamespace(ctx, cluster1.remoteClusterID)
	if err != nil {
		return fmt.Errorf("unable to resolve local tenant namespace: %w", err)
	}
	if cluster1.localNetworkNamespace != clientTenantNs.Name {
		return fmt.Errorf("openvpn requires the local tenant namespace %q, but %q was selected", clientTenantNs.Name, cluster1.localNetworkNamespace)
	}

	serverTenantNs, err := cluster2.localNamespaceManager.GetNamespace(ctx, cluster2.remoteClusterID)
	if err != nil {
		return fmt.Errorf("unable to resolve remote tenant namespace: %w", err)
	}
	if cluster2.localNetworkNamespace != serverTenantNs.Name {
		return fmt.Errorf("openvpn requires the remote tenant namespace %q, but %q was selected", serverTenantNs.Name, cluster2.localNetworkNamespace)
	}

	serverNamespace := serverTenantNs.Name
	clientNamespace := clientTenantNs.Name
	return openvpn.EnsureGatewaySecrets(
		ctx,
		cluster2.local.KubeClient,
		cluster1.local.KubeClient,
		serverNamespace,
		clientNamespace,
		DefaultOpenVPNServerSecretName,
		DefaultOpenVPNClientSecretName,
	)
}

// Sets the owner reference on the secret used by OpenVPN gateway to enable garbage collection when the owning gateway resources are deleted.
func (o *Options) setOpenVPNSecretOwnerReferences(ctx context.Context, cluster1, cluster2 *Cluster,
	gwServer *networkingv1beta1.GatewayServer, gwClient *networkingv1beta1.GatewayClient) error {
	if gwServer == nil || gwClient == nil {
		return fmt.Errorf("openvpn secret owner reference setup requires gateway resources")
	}

	if err := openvpn.SetSecretOwnerReference(
		ctx,
		cluster2.local.KubeClient,
		cluster2.local.CRClient.Scheme(),
		gwServer.Namespace,
		DefaultOpenVPNServerSecretName,
		gwServer,
	); err != nil {
		return fmt.Errorf("failed to set owner reference on server secret: %w", err)
	}

	if err := openvpn.SetSecretOwnerReference(
		ctx,
		cluster1.local.KubeClient,
		cluster1.local.CRClient.Scheme(),
		gwClient.Namespace,
		DefaultOpenVPNClientSecretName,
		gwClient,
	); err != nil {
		return fmt.Errorf("failed to set owner reference on client secret: %w", err)
	}

	return nil
}

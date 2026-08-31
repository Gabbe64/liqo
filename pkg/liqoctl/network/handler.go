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
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/utils/ptr"

	liqov1beta1 "github.com/liqotech/liqo/apis/core/v1beta1"
	networkingv1beta1 "github.com/liqotech/liqo/apis/networking/v1beta1"
	"github.com/liqotech/liqo/pkg/liqo-controller-manager/networking/forge"
	"github.com/liqotech/liqo/pkg/liqo-controller-manager/networking/getters"
	"github.com/liqotech/liqo/pkg/liqoctl/factory"
	argsutils "github.com/liqotech/liqo/pkg/utils/args"
)

// Options encapsulates the arguments of the network command.
type Options struct {
	LocalFactory  *factory.Factory
	RemoteFactory *factory.Factory

	Timeout        time.Duration
	Wait           bool
	SkipValidation bool

	ServerGatewayType           string
	ServerTemplateName          string
	ServerTemplateNamespace     string
	ServerServiceType           *argsutils.StringEnum
	ServerServicePort           int32
	ServerServiceNodePort       int32
	ServerServiceLoadBalancerIP string

	ClientGatewayType       string
	ClientTemplateName      string
	ClientTemplateNamespace string
	// ClientConnectAddress is the address used by the client to connect to the gateway server. When this value is specified
	// liqoctl ignores the values of server and port written in the GatewayServer status.
	ClientConnectAddress string
	// ClientConnectPort is the port used by the client to connect to the gateway server. When this value is specified
	// liqoctl ignores the values of server and port written in the GatewayServer status.
	ClientConnectPort int32

	MTU                int
	DisableSharingKeys bool
}

// NewOptions returns a new Options struct.
func NewOptions(localFactory *factory.Factory) *Options {
	return &Options{
		LocalFactory: localFactory,
		ServerServiceType: argsutils.NewEnum(
			[]string{string(corev1.ServiceTypeLoadBalancer), string(corev1.ServiceTypeNodePort), string(corev1.ServiceTypeClusterIP)},
			string(forge.DefaultGwServerServiceType)),
	}
}

// RunReset reset the liqo networking between two clusters.
// If the clusters are still connected through the gateways, it deletes them before removing network Configurations.
func (o *Options) RunReset(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, o.Timeout)
	defer cancel()

	// Create and initialize cluster 1.
	cluster1, err := NewCluster(ctx, o.LocalFactory, o.RemoteFactory, false)
	if err != nil {
		return err
	}

	// Create and initialize cluster 2.
	cluster2, err := NewCluster(ctx, o.RemoteFactory, o.LocalFactory, false)
	if err != nil {
		return err
	}

	// Run disconnect command to remove gateways.
	if err := o.RunDisconnect(ctx, cluster1, cluster2); err != nil {
		return err
	}

	// Delete Configuration on cluster 1
	if err := cluster1.DeleteConfiguration(ctx, cluster2.localClusterID, cluster1.localNetworkNamespace); err != nil {
		return err
	}

	// Delete Configuration on cluster 2
	return cluster2.DeleteConfiguration(ctx, cluster1.localClusterID, cluster2.localNetworkNamespace)
}

// RunConnect connect two clusters using liqo networking.
func (o *Options) RunConnect(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, o.Timeout)
	defer cancel()

	if o.ServerTemplateNamespace == "" {
		o.ServerTemplateNamespace = o.RemoteFactory.LiqoNamespace
	}

	if o.ClientTemplateNamespace == "" {
		o.ClientTemplateNamespace = o.LocalFactory.LiqoNamespace
	}

	// Create and initialize cluster 1.
	cluster1, err := NewCluster(ctx, o.LocalFactory, o.RemoteFactory, true)
	if err != nil {
		return err
	}

	// Create and initialize cluster 2.
	cluster2, err := NewCluster(ctx, o.RemoteFactory, o.LocalFactory, true)
	if err != nil {
		return err
	}
	// Exchange network configurations between the clusters
	if err := o.initNetworkConfigs(ctx, cluster1, cluster2); err != nil {
		return err
	}

	// Connect the two clusters
	if !o.SkipValidation {
		// Check if the Templates exists and is valid on cluster 2
		if err := cluster2.CheckTemplateGwServer(ctx, o); err != nil {
			return err
		}

		// Check if the Templates exists and is valid on cluster 1
		if err := cluster1.CheckTemplateGwClient(ctx, o); err != nil {
			return err
		}
	}

	// Check if the Networking is initialized on cluster 1
	if err := cluster1.CheckNetworkInitialized(ctx, cluster2.localClusterID); err != nil {
		return err
	}

	// Check if the Networking is initialized on cluster 2
	if err := cluster2.CheckNetworkInitialized(ctx, cluster1.localClusterID); err != nil {
		return err
	}

	// Check if the reverse Networking is already established on cluster 1
	if established, err := cluster1.CheckAlreadyEstablishedForGwServer(ctx); err != nil {
		return err
	} else if established {
		return nil
	}

	// Check if the reverse Networking is already established on cluster 2
	if established, err := cluster2.CheckAlreadyEstablishedForGwClient(ctx); err != nil {
		return err
	} else if established {
		return nil
	}

	// Create gateway server on cluster 2
	gwServer, err := cluster2.EnsureGatewayServer(ctx, o.newGatewayServerForgeOptions(o.RemoteFactory.KubeClient, cluster1.localClusterID))
	if err != nil {
		return err
	}

	// Wait for the gateway pod to be ready
	if err := cluster2.waiter.ForGatewayPodReady(ctx, gwServer); err != nil {
		return err
	}

	// Wait for the endpoint status of the gateway server to be set
	if err := cluster2.waiter.ForGatewayServerStatusEndpoint(ctx, gwServer); err != nil {
		return err
	}

	// Create gateway client on cluster 1

	// By default address and port used by the GatewayClient are the ones written in the endpoint field of the status of the GatewayServer,
	// unless address or port are manually overwritten
	endpoint := o.overrideServerEndpoint(gwServer.Status.Endpoint)

	gwClient, err := cluster1.EnsureGatewayClient(ctx,
		o.newGatewayClientForgeOptions(o.LocalFactory.KubeClient, cluster2.localClusterID, endpoint))
	if err != nil {
		return err
	}

	// Wait for the gateway pod to be ready
	if err := cluster1.waiter.ForGatewayPodReady(ctx, gwClient); err != nil {
		return err
	}

	// Plaintext gateways do not use WireGuard keys; the key-sharing step is
	// replaced by a tunnel-specific one. Fail early on mixed template kinds.
	tunnel, err := peeringTunnelKind(gwServer, gwClient)
	if err != nil {
		return err
	}

	if err := o.exchangePeerInfo(ctx, cluster1, cluster2, gwServer, gwClient, tunnel); err != nil {
		return err
	}

	if o.Wait {
		// Wait for Connections on both cluster to be created.
		conn2, err := cluster2.waiter.ForConnection(ctx, gwServer.Namespace, cluster1.localClusterID)
		if err != nil {
			return err
		}
		conn1, err := cluster1.waiter.ForConnection(ctx, gwClient.Namespace, cluster2.localClusterID)
		if err != nil {
			return err
		}

		// Wait for Connections on both cluster cluster to be established
		if err := cluster1.waiter.ForConnectionEstablished(ctx, conn1); err != nil {
			return err
		}
		if err := cluster2.waiter.ForConnectionEstablished(ctx, conn2); err != nil {
			return err
		}
	}

	return nil
}

// overrideServerEndpoint applies the user-provided address and port overrides to
// the endpoint advertised by the gateway server.
func (o *Options) overrideServerEndpoint(endpoint *networkingv1beta1.EndpointStatus) *networkingv1beta1.EndpointStatus {
	if o.ClientConnectAddress != "" {
		endpoint.Addresses = []string{o.ClientConnectAddress}
	}
	if o.ClientConnectPort != 0 {
		endpoint.Port = o.ClientConnectPort
	}
	return endpoint
}

// exchangePeerInfo performs the tunnel-specific step that completes the peering
// once both gateways are running.
//
// WireGuard exchanges public keys. Geneve instead hands the client endpoint over
// to the server: the kernel gives no way to pin the outer source port, so the
// server can never infer its peer from received traffic and must be told where
// to transmit.
func (o *Options) exchangePeerInfo(ctx context.Context, cluster1, cluster2 *Cluster,
	gwServer *networkingv1beta1.GatewayServer, gwClient *networkingv1beta1.GatewayClient, tunnel tunnelKind) error {
	if tunnel == tunnelWireGuard {
		if o.DisableSharingKeys {
			return nil
		}
		return shareWireGuardKeys(ctx, cluster1, cluster2, gwServer, gwClient)
	}

	// Geneve: publish the client endpoint so that the server can transmit to it.
	if err := cluster1.waiter.ForGatewayClientStatusEndpoint(ctx, gwClient); err != nil {
		return err
	}
	if err := checkGenevePorts(gwServer, gwClient); err != nil {
		return err
	}
	return cluster2.EnsurePeerEndpoint(ctx, cluster1.localClusterID, gwClient.Status.Endpoint, gwServer)
}

// checkGenevePorts rejects a Geneve peering whose two gateways are published on
// different ports.
//
// A Geneve device uses a single UDP port for both listening and transmitting,
// and the kernel refuses to change it on a live device, so each side transmits
// to the peer on its own port. Unequal ports therefore black-hole one direction
// while everything still reports as created — which is worth catching here,
// where both endpoints are known and the message can name them.
func checkGenevePorts(gwServer *networkingv1beta1.GatewayServer, gwClient *networkingv1beta1.GatewayClient) error {
	if gwServer.Status.Endpoint == nil || gwClient.Status.Endpoint == nil {
		return nil
	}
	serverPort, clientPort := gwServer.Status.Endpoint.Port, gwClient.Status.Endpoint.Port
	if serverPort == clientPort {
		return nil
	}
	return fmt.Errorf(
		"the Geneve gateways are published on different ports (server %d, client %d), so one direction of the "+
			"tunnel would be black-holed: a Geneve device uses one UDP port for both listening and transmitting. "+
			"Re-run with \"--gw-server-service-port %d --gw-server-service-nodeport %d\", or align "+
			"networking.gatewayTemplates.geneve.client.service.port with the server port",
		serverPort, clientPort, clientPort, clientPort)
}

// tunnelKind identifies the tunnel technology a gateway pair is built on.
type tunnelKind int

const (
	// tunnelWireGuard is the default, encrypted tunnel. It is also the fallback
	// for any template kind that is not recognised, since a custom template is
	// assumed to follow the default WireGuard contract.
	tunnelWireGuard tunnelKind = iota
	// tunnelGeneve is the plaintext Geneve tunnel.
	tunnelGeneve
)

// peeringTunnelKind tells which tunnel technology the gateway pair uses. It
// returns an error when the two sides disagree, since mixed tunnel technologies
// cannot interoperate.
func peeringTunnelKind(gwServer *networkingv1beta1.GatewayServer,
	gwClient *networkingv1beta1.GatewayClient) (tunnelKind, error) {
	serverKind, clientKind := gwServer.Spec.ServerTemplateRef.Kind, gwClient.Spec.ClientTemplateRef.Kind

	server, client := tunnelWireGuard, tunnelWireGuard
	if serverKind == networkingv1beta1.GeneveGatewayServerTemplateKind {
		server = tunnelGeneve
	}
	if clientKind == networkingv1beta1.GeneveGatewayClientTemplateKind {
		client = tunnelGeneve
	}

	if server != client {
		return server, fmt.Errorf("mismatched gateway templates: server %q and client %q must use the same tunnel technology",
			serverKind, clientKind)
	}
	return server, nil
}

// shareWireGuardKeys exchanges the WireGuard public keys between the two clusters.
func shareWireGuardKeys(ctx context.Context, cluster1, cluster2 *Cluster,
	gwServer *networkingv1beta1.GatewayServer, gwClient *networkingv1beta1.GatewayClient) error {
	// Wait for gateway server to set secret reference (containing the server public key) in the status
	if err := cluster2.waiter.ForGatewayServerSecretRef(ctx, gwServer); err != nil {
		return err
	}
	keyServer, err := getters.ExtractKeyFromSecretRef(ctx, cluster2.local.CRClient, gwServer.Status.SecretRef)
	if err != nil {
		return err
	}

	// Create PublicKey of gateway server on cluster 1
	if err := cluster1.EnsurePublicKey(ctx, cluster2.localClusterID, keyServer, gwClient); err != nil {
		return err
	}

	// Wait for gateway client to set secret reference (containing the client public key) in the status
	if err := cluster1.waiter.ForGatewayClientSecretRef(ctx, gwClient); err != nil {
		return err
	}
	keyClient, err := getters.ExtractKeyFromSecretRef(ctx, cluster1.local.CRClient, gwClient.Status.SecretRef)
	if err != nil {
		return err
	}

	// Create PublicKey of gateway client on cluster 2
	return cluster2.EnsurePublicKey(ctx, cluster1.localClusterID, keyClient, gwServer)
}

// RunDisconnect disconnects two clusters.
// It deletes the gateways (if present) on both clusters.
// Cluster1 and Cluster2 are optional, if not provided they will be created and initialized.
func (o *Options) RunDisconnect(ctx context.Context, cluster1, cluster2 *Cluster) error {
	var err error
	ctx, cancel := context.WithTimeout(ctx, o.Timeout)
	defer cancel()

	if cluster1 == nil {
		// Create and initialize cluster 1.
		cluster1, err = NewCluster(ctx, o.LocalFactory, o.RemoteFactory, false)
		if err != nil {
			return err
		}
	}

	if cluster2 == nil {
		// Create and initialize cluster 2.
		cluster2, err = NewCluster(ctx, o.RemoteFactory, o.LocalFactory, false)
		if err != nil {
			return err
		}
	}

	// Delete gateway client on cluster 1
	if err := cluster1.DeleteGatewayClient(ctx, cluster2.localClusterID); err != nil {
		return err
	}

	// Delete gateway client on cluster 2
	if err := cluster2.DeleteGatewayClient(ctx, cluster1.localClusterID); err != nil {
		return err
	}

	// Delete gateway server on cluster 1
	if err := cluster1.DeleteGatewayServer(ctx, cluster2.localClusterID); err != nil {
		return err
	}

	// Delete gateway server on cluster 2
	return cluster2.DeleteGatewayServer(ctx, cluster1.localClusterID)
}

func (o *Options) initNetworkConfigs(ctx context.Context, cluster1, cluster2 *Cluster) error {
	// Forges the local Configuration of cluster 1 to be applied on remote clusters.
	if err := cluster1.SetLocalConfiguration(ctx); err != nil {
		return err
	}

	// Forges the local Configuration of cluster 2 to be applied on remote clusters.
	if err := cluster2.SetLocalConfiguration(ctx); err != nil {
		return err
	}

	// Setup Configurations in cluster 1.
	if err := cluster1.SetupConfiguration(ctx, cluster2.networkConfiguration); err != nil {
		return err
	}

	// Setup Configurations in cluster 2.
	if err := cluster2.SetupConfiguration(ctx, cluster1.networkConfiguration); err != nil {
		return err
	}

	// Wait for cluster 1 to be ready.
	if err := cluster1.waiter.ForConfiguration(ctx, cluster2.localClusterID, cluster1.localNetworkNamespace); err != nil {
		return err
	}

	// Wait for cluster 2 to be ready.
	if err := cluster2.waiter.ForConfiguration(ctx, cluster1.localClusterID, cluster2.localNetworkNamespace); err != nil {
		return err
	}

	return nil
}

func (o *Options) newGatewayServerForgeOptions(kubeClient kubernetes.Interface, remoteClusterID liqov1beta1.ClusterID) *forge.GwServerOptions {
	return &forge.GwServerOptions{
		KubeClient:        kubeClient,
		RemoteClusterID:   remoteClusterID,
		GatewayType:       o.ServerGatewayType,
		TemplateName:      o.ServerTemplateName,
		TemplateNamespace: o.ServerTemplateNamespace,
		ServiceType:       corev1.ServiceType(o.ServerServiceType.Value),
		MTU:               o.MTU,
		Port:              o.ServerServicePort,
		NodePort:          ptr.To(o.ServerServiceNodePort),
		LoadBalancerIP:    ptr.To(o.ServerServiceLoadBalancerIP),
	}
}

func (o *Options) newGatewayClientForgeOptions(kubeClient kubernetes.Interface, remoteClusterID liqov1beta1.ClusterID,
	serverEndpoint *networkingv1beta1.EndpointStatus) *forge.GwClientOptions {
	return &forge.GwClientOptions{
		KubeClient:        kubeClient,
		RemoteClusterID:   remoteClusterID,
		GatewayType:       o.ClientGatewayType,
		TemplateName:      o.ClientTemplateName,
		TemplateNamespace: o.ClientTemplateNamespace,
		MTU:               o.MTU,
		Addresses:         serverEndpoint.Addresses,
		Port:              serverEndpoint.Port,
		Protocol:          string(*serverEndpoint.Protocol),
	}
}

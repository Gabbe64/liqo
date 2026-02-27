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

package openvpn

import (
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/liqotech/liqo/pkg/gateway"
	"github.com/liqotech/liqo/pkg/liqo-controller-manager/networking/forge"
)

// FlagName is the type for the name of the flags.
type FlagName string

func (fn FlagName) String() string {
	return string(fn)
}

const (
	// FlagNameMTU is the MTU for the OpenVPN interface.
	FlagNameMTU FlagName = "mtu"
	// FlagNameDevice is the OpenVPN device name.
	FlagNameDevice FlagName = "dev"
	// FlagNameDeviceType is the OpenVPN device type.
	FlagNameDeviceType FlagName = "dev-type"
	// FlagNameIfconfigLocalIP is the local IP for --ifconfig.
	FlagNameIfconfigLocalIP FlagName = "ifconfig-local-ip"
	// FlagNameIfconfigRemoteIP is the remote IP for --ifconfig.
	FlagNameIfconfigRemoteIP FlagName = "ifconfig-remote-ip"
	// FlagNameListenPort is the listen port for the OpenVPN interface.
	FlagNameListenPort FlagName = "listen-port"
	// FlagNameInterfaceIP is the IP of the OpenVPN interface.
	FlagNameInterfaceIP FlagName = "interface-ip"
	// FlagNameEndpointAddress is the address of the endpoint for the OpenVPN interface.
	FlagNameEndpointAddress FlagName = "endpoint-address"
	// FlagNameEndpointPort is the port of the endpoint for the OpenVPN interface.
	FlagNameEndpointPort FlagName = "endpoint-port"
	// FlagNamePort is the port for the OpenVPN interface (alias for listen/endpoint port).
	FlagNamePort FlagName = "port"
	// FlagNameRemote is the remote endpoint address (alias for endpoint-address).
	FlagNameRemote FlagName = "remote"
	// FlagNameConfigDir is the directory where the OpenVPN configuration is stored.
	FlagNameConfigDir FlagName = "config-dir"
	// FlagNameCAFile is the CA certificate path.
	FlagNameCAFile FlagName = "ca"
	// FlagNameCertFile is the client/server certificate path.
	FlagNameCertFile FlagName = "cert"
	// FlagNameKeyFile is the client/server key path.
	FlagNameKeyFile FlagName = "key"
	// FlagNameDHFile is the DH parameters file path.
	FlagNameDHFile FlagName = "dh"
	// FlagNameTLSAuthKeyFile is the tls-auth key path.
	FlagNameTLSAuthKeyFile FlagName = "tls-auth"
	// FlagNameTLSAuthDirection is the tls-auth direction: 1 for client and 0 for server.
	FlagNameTLSAuthDirection FlagName = "tls-auth-direction"
	// FlagNameTLSClient enables tls-client.
	FlagNameTLSClient FlagName = "tls-client"
	// FlagNameTLSServer enables tls-server.
	FlagNameTLSServer FlagName = "tls-server"
	// FlagNameCipher is the cipher.
	FlagNameCipher FlagName = "cipher"
	// FlagNameAuth is the auth digest.
	FlagNameAuth FlagName = "auth"
	// FlagNameProto is the transport protocol.
	FlagNameProto FlagName = "proto"
	// FlagNameKeepalivePing is the keepalive ping interval.
	FlagNameKeepalivePing FlagName = "keepalive-ping"
	// FlagNameKeepaliveTimeout is the keepalive timeout.
	FlagNameKeepaliveTimeout FlagName = "keepalive-timeout"
	// FlagNameMaxClients sets the maximum number of clients.
	FlagNameMaxClients FlagName = "max-clients"
	// FlagNameManagementAddress is the OpenVPN management bind address.
	FlagNameManagementAddress FlagName = "management-address"
	// FlagNameManagementPort is the OpenVPN management port.
	FlagNameManagementPort FlagName = "management-port"
	// FlagNameExtraOpts contains additional OpenVPN options to append to the config file.
	FlagNameExtraOpts FlagName = "extra-opts"
)

// ClientRequiredFlags contains the list of the mandatory flags for the client mode.
var ClientRequiredFlags = []FlagName{
	FlagNameIfconfigLocalIP,
	FlagNameIfconfigRemoteIP,
	FlagNameEndpointAddress,
}

// ServerRequiredFlags contains the list of the mandatory flags for the server mode.
var ServerRequiredFlags = []FlagName{
	FlagNameIfconfigLocalIP,
	FlagNameIfconfigRemoteIP,
}

// InitFlags initializes the flags for the OpenVPN tunnel.
func InitFlags(flagset *pflag.FlagSet, opts *Options) {
	flagset.IntVar(&opts.MTU, FlagNameMTU.String(), forge.DefaultMTU, "MTU for the interface")
	flagset.StringVar(&opts.IfconfigLocalIP, FlagNameIfconfigLocalIP.String(), opts.IfconfigLocalIP, "OpenVPN ifconfig local IP")
	flagset.StringVar(&opts.IfconfigRemoteIP, FlagNameIfconfigRemoteIP.String(), opts.IfconfigRemoteIP, "OpenVPN ifconfig remote IP")
	flagset.IntVar(&opts.ListenPort, FlagNameListenPort.String(), forge.DefaultGwServerPort, "Listen port (server only)")
	flagset.StringVar(&opts.InterfaceIP, FlagNameInterfaceIP.String(), "", "Interface IP address")
	flagset.StringVar(&opts.EndpointAddress, FlagNameEndpointAddress.String(), "", "Endpoint address (client only)")
	flagset.IntVar(&opts.EndpointPort, FlagNameEndpointPort.String(), forge.DefaultGwServerPort, "Endpoint port (client only)")
	flagset.IntVar(&opts.Port, FlagNamePort.String(), 0, "Port (alias for listen/endpoint port)")
	flagset.StringVar(&opts.EndpointAddress, FlagNameRemote.String(), opts.EndpointAddress, "Remote endpoint address (client only)")
	flagset.StringVar(&opts.ConfigDir, FlagNameConfigDir.String(), DefaultConfigDir, "Directory where the OpenVPN configuration is stored")
	flagset.StringVar(&opts.Auth, FlagNameAuth.String(), opts.Auth, "Auth digest")
	flagset.StringVar(&opts.Proto, FlagNameProto.String(), opts.Proto, "Protocol (udp/tcp)")
	flagset.IntVar(&opts.KeepalivePing, FlagNameKeepalivePing.String(), opts.KeepalivePing, "Keepalive ping interval")
	flagset.IntVar(&opts.KeepaliveTimeout, FlagNameKeepaliveTimeout.String(), opts.KeepaliveTimeout, "Keepalive timeout")
	flagset.IntVar(&opts.MaxClients, FlagNameMaxClients.String(), opts.MaxClients, "Maximum number of clients")
	flagset.StringVar(&opts.ManagementAddress, FlagNameManagementAddress.String(), opts.ManagementAddress, "OpenVPN management bind address")
	flagset.IntVar(&opts.ManagementPort, FlagNameManagementPort.String(), opts.ManagementPort, "OpenVPN management port")
	flagset.StringVar(&opts.ExtraOpts, FlagNameExtraOpts.String(), opts.ExtraOpts, "Additional OpenVPN options appended to the config file (newline-separated)")
}

// MarkFlagsRequired marks the flags as required.
func MarkFlagsRequired(cmd *cobra.Command, opts *Options) error {
	if opts.GwOptions.Mode == gateway.ModeClient {
		for _, flag := range ClientRequiredFlags {
			if err := cmd.MarkFlagRequired(flag.String()); err != nil {
				return err
			}
		}
		return nil
	}
	for _, flag := range ServerRequiredFlags {
		if err := cmd.MarkFlagRequired(flag.String()); err != nil {
			return err
		}
	}
	return nil
}

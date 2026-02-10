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
	"fmt"

	"github.com/liqotech/liqo/pkg/gateway"
	"github.com/liqotech/liqo/pkg/liqo-controller-manager/networking/forge"
)

const (
	// DefaultListenPort is the default port where the OpenVPN server listens.
	DefaultListenPort = 1194
	// DefaultConfigDir is the default directory where OpenVPN configuration is stored.
	DefaultConfigDir = "/etc/openvpn"
	// DefaultKeysDir is the default directory where OpenVPN keys/certs are stored.
	DefaultKeysDir = "/etc/openvpn/keys"
	// DefaultManagementAddress is the default OpenVPN management bind address.
	DefaultManagementAddress = "127.0.0.1"
	// DefaultManagementPort is the default OpenVPN management port.
	DefaultManagementPort = 5555

	// TODO: change the value or apply dynamic MTU calculation.
	DefaultMTU = 1340
)

// Options contains the options for the OpenVPN interface.
type Options struct {
	GwOptions *gateway.Options

	MTU               int
	Device            string
	DeviceType        string
	IfconfigLocalIP   string
	IfconfigRemoteIP  string
	InterfaceIP       string
	ListenPort        int
	EndpointAddress   string
	EndpointPort      int
	Port              int
	ConfigDir         string
	CAFile            string
	CertFile          string
	KeyFile           string
	DHFile            string
	TLSAuthKeyFile    string
	TLSAuthDirection  int
	TLSClient         bool
	TLSServer         bool
	Cipher            string
	Auth              string
	Proto             string
	KeepalivePing     int
	KeepaliveTimeout  int
	MaxClients        int
	ManagementAddress string
	ManagementPort    int
}

// NewOptions returns a new Options struct.
func NewOptions(options *gateway.Options) *Options {
	return &Options{
		GwOptions:         options,
		MTU:               DefaultMTU,
		Device:            "liqo-tunnel",
		DeviceType:        "tun",
		ListenPort:        DefaultListenPort,
		EndpointPort:      forge.DefaultGwServerPort,
		ConfigDir:         DefaultConfigDir,
		CAFile:            DefaultKeysDir + "/ca.crt",
		CertFile:          DefaultKeysDir + "/client.crt",
		KeyFile:           DefaultKeysDir + "/client.key",
		DHFile:            "",
		TLSAuthKeyFile:    DefaultKeysDir + "/ta.key",
		TLSAuthDirection:  1,
		TLSClient:         false,
		TLSServer:         false,
		Cipher:            "AES-256-CBC",
		Auth:              "SHA256",
		Proto:             "udp",
		KeepalivePing:     10,
		KeepaliveTimeout:  120,
		MaxClients:        0,
		ManagementAddress: DefaultManagementAddress,
		ManagementPort:    DefaultManagementPort,
	}
}

// NormalizeOptions applies aliases and validates required fields based on the gateway mode.
func NormalizeOptions(opts *Options) error {
	if opts == nil {
		return nil
	}

	// Default paths for client mode, note that they are set at Options creation.
	defaultClientCert := DefaultKeysDir + "/client.crt"
	defaultClientKey := DefaultKeysDir + "/client.key"

	// Default paths for server mode.
	defaultServerCert := DefaultKeysDir + "/server.crt"
	defaultServerKey := DefaultKeysDir + "/server.key"
	defaultServerDH := DefaultKeysDir + "/dh.pem"

	if opts.Port != 0 {
		if opts.GwOptions != nil && opts.GwOptions.Mode == gateway.ModeServer {
			opts.ListenPort = opts.Port
		} else {
			opts.EndpointPort = opts.Port
		}
	}

	switch opts.GwOptions.Mode {
    case gateway.ModeServer:
        if opts.CertFile == "" || opts.CertFile == defaultClientCert {
            opts.CertFile = defaultServerCert
        }
        if opts.KeyFile == "" || opts.KeyFile == defaultClientKey {
            opts.KeyFile = defaultServerKey
        }
        if opts.DHFile == "" {
			opts.DHFile = defaultServerDH
		}
	case gateway.ModeClient:
		// Default client values are set at Options creation.
        if opts.EndpointAddress == "" {
            return fmt.Errorf("endpoint address is required (use --endpoint-address or --remote)")
        }
        if opts.EndpointPort == 0 {
            return fmt.Errorf("endpoint port is required (use --endpoint-port or --port)")
        }
	default:
		return fmt.Errorf("invalid gateway mode %q", opts.GwOptions.Mode)
    }

	return nil
}

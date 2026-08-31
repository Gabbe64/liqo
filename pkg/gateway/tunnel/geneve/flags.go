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

package geneve

import (
	"time"

	"github.com/spf13/pflag"

	"github.com/liqotech/liqo/pkg/liqo-controller-manager/networking/forge"
)

// FlagName is the type for the name of the flags.
type FlagName string

func (fn FlagName) String() string {
	return string(fn)
}

const (
	// FlagNameMTU is the MTU for the Geneve tunnel interface.
	FlagNameMTU FlagName = "mtu"

	// FlagNameVNI is the Geneve Virtual Network Identifier.
	FlagNameVNI FlagName = "vni"

	// FlagNamePort is the local UDP port the tunnel binds for reception.
	FlagNamePort FlagName = "port"

	// FlagNameRemoteAddress is the address (IP or DNS name) of the remote tunnel endpoint.
	FlagNameRemoteAddress FlagName = "remote-address"

	// FlagNameRemotePort is the UDP port of the remote tunnel endpoint.
	FlagNameRemotePort FlagName = "remote-port"

	// FlagNameL3 selects the inner payload type of the tunnel.
	FlagNameL3 FlagName = "l3"

	// FlagNameDNSCheckInterval is the interval between DNS re-resolutions of the remote address.
	FlagNameDNSCheckInterval FlagName = "dns-check-interval"
)

const (
	// DefaultVNI is the default Geneve Virtual Network Identifier.
	DefaultVNI = 100

	// DefaultPort is the default local UDP port. It deliberately does not default
	// to the IANA Geneve port (6081), nor to the port the internal fabric uses for
	// its own Geneve tunnels, so that the external tunnel never shares a port with
	// an overlay running on the same node.
	DefaultPort = int(forge.DefaultGwServerPort)
)

// InitFlags initializes the flags for the Geneve tunnel.
func InitFlags(flagset *pflag.FlagSet, opts *Options) {
	flagset.IntVar(&opts.MTU, FlagNameMTU.String(), forge.DefaultMTU, "MTU for the Geneve tunnel interface")
	flagset.IntVar(&opts.VNI, FlagNameVNI.String(), DefaultVNI, "Geneve Virtual Network Identifier (must match on both sides)")
	flagset.IntVar(&opts.Port, FlagNamePort.String(), DefaultPort, "Local UDP port the tunnel binds for reception")
	flagset.StringVar(&opts.RemoteAddress, FlagNameRemoteAddress.String(), "",
		"Address (IP or DNS name) of the remote tunnel endpoint (required in client mode)")
	flagset.IntVar(&opts.RemotePort, FlagNameRemotePort.String(), 0,
		"UDP port of the remote tunnel endpoint (required in client mode)")
	flagset.BoolVar(&opts.L3, FlagNameL3.String(), true,
		"Carry L3 payloads (inner_proto_inherit), dropping the inner Ethernet header. Requires kernel >= 5.19 "+
			"and must match on both sides")
	flagset.DurationVar(&opts.DNSCheckInterval, FlagNameDNSCheckInterval.String(), 5*time.Minute,
		"Interval between DNS re-resolutions of the remote endpoint address")
}

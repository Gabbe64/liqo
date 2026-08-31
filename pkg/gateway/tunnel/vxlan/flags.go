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

package vxlan

import (
	"fmt"
	"time"

	"github.com/spf13/pflag"
)

// FlagName is the type for the name of the flags.
type FlagName string

func (fn FlagName) String() string {
	return string(fn)
}

const (
	// FlagNameMTU is the MTU for the VXLAN tunnel interface.
	FlagNameMTU FlagName = "mtu"

	// FlagNameVNI is the VXLAN Network Identifier.
	FlagNameVNI FlagName = "vni"

	// FlagNamePort is the local UDP port (bind and pinned source port).
	FlagNamePort FlagName = "port"

	// FlagNameRemoteAddress is the address (IP or DNS name) of the remote tunnel endpoint.
	FlagNameRemoteAddress FlagName = "remote-address"

	// FlagNameRemotePort is the UDP port of the remote tunnel endpoint.
	FlagNameRemotePort FlagName = "remote-port"

	// FlagNameTunnelMode selects the endpoint-discovery strategy.
	FlagNameTunnelMode FlagName = "tunnel-mode"

	// FlagNameLearningCooldown is the minimum interval between two endpoint updates.
	FlagNameLearningCooldown FlagName = "learning-cooldown"

	// FlagNameDNSCheckInterval is the interval between DNS re-resolutions of the remote address.
	FlagNameDNSCheckInterval FlagName = "dns-check-interval"
)

const (
	// DefaultMTU is the default MTU for the VXLAN tunnel interface:
	// 1500 minus the VXLAN-over-IPv4 overhead (20 outer IP + 8 UDP + 8 VXLAN + 14 inner Ethernet).
	DefaultMTU = 1450

	// DefaultVNI is the default VXLAN Network Identifier.
	DefaultVNI = 100

	// DefaultPort is the default local UDP port (IANA VXLAN port).
	DefaultPort = 4789
)

// InitFlags initializes the flags for the VXLAN tunnel.
func InitFlags(flagset *pflag.FlagSet, opts *Options) {
	flagset.IntVar(&opts.MTU, FlagNameMTU.String(), DefaultMTU, "MTU for the VXLAN tunnel interface")
	flagset.IntVar(&opts.VNI, FlagNameVNI.String(), DefaultVNI, "VXLAN Network Identifier (must match on both sides)")
	flagset.IntVar(&opts.Port, FlagNamePort.String(), DefaultPort,
		"Local UDP port: bound for reception and pinned as source port for transmission")
	flagset.StringVar(&opts.RemoteAddress, FlagNameRemoteAddress.String(), "",
		"Address (IP or DNS name) of the remote tunnel endpoint (required in client mode)")
	flagset.IntVar(&opts.RemotePort, FlagNameRemotePort.String(), 0,
		"UDP port of the remote tunnel endpoint (required in client mode)")
	flagset.StringVar((*string)(&opts.TunnelMode), FlagNameTunnelMode.String(), string(TunnelModeNATTraversal),
		fmt.Sprintf("Endpoint-discovery strategy, one of %v. %q learns the peer endpoint from received traffic "+
			"(works behind NAT, pins the source port); %q uses configured endpoints on both sides (requires mutual "+
			"reachability, allows source-port hashing so RSS/ECMP work)",
			TunnelModes, TunnelModeNATTraversal, TunnelModeStatic))
	flagset.DurationVar(&opts.LearningCooldown, FlagNameLearningCooldown.String(), 3*time.Second,
		"Minimum interval between two learned endpoint updates")
	flagset.DurationVar(&opts.DNSCheckInterval, FlagNameDNSCheckInterval.String(), 5*time.Minute,
		"Interval between DNS re-resolutions of the remote endpoint address (client mode)")
}

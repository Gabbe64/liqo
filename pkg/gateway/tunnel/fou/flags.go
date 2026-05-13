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

package fou

import (
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// FlagName is the type for the name of the flags.
type FlagName string

func (fn FlagName) String() string {
	return string(fn)
}

const (
	// FlagNameMTU is the MTU for the FOU tunnel interface.
	FlagNameMTU FlagName = "mtu"

	// DefaultMTU is the default MTU for the FOU tunnel interface.
	DefaultMTU = 1480

	// FlagNameLocalPort is the UDP port on which this side listens for incoming FoU packets.
	FlagNameLocalPort FlagName = "listen-port"

	// FlagNameRemotePort is the UDP encapsulation destination port on the remote side.
	FlagNameRemotePort FlagName = "remote-port"

	// FlagNameRemoteAddress is the IP address of the remote tunnel endpoint.
	FlagNameRemoteAddress FlagName = "remote-address"
)

// RequiredFlags contains the list of the mandatory flags, required for both client and server modes.
var RequiredFlags = []FlagName{
	FlagNameLocalPort,
	FlagNameRemotePort,
	FlagNameRemoteAddress,
}

// InitFlags initializes the flags for the FOU tunnel.
func InitFlags(flagset *pflag.FlagSet, opts *Options) {
	flagset.IntVar(&opts.MTU, FlagNameMTU.String(), DefaultMTU, "MTU for the FOU tunnel interface")
	flagset.IntVar(&opts.LocalPort, FlagNameLocalPort.String(), 0, "UDP port to listen on for incoming FoU packets on this side")
	flagset.IntVar(&opts.RemotePort, FlagNameRemotePort.String(), 0, "UDP destination port for FOU encapsulation on the remote side")
	flagset.StringVar(&opts.RemoteAddress, FlagNameRemoteAddress.String(), "", "IP address of the remote tunnel endpoint (peer's LoadBalancer IP)")
}

// MarkFlagsRequired marks flags as required.
func MarkFlagsRequired(cmd *cobra.Command, _ *Options) error {
	for _, flag := range RequiredFlags {
		if err := cmd.MarkFlagRequired(flag.String()); err != nil {
			return err
		}
	}
	return nil
}

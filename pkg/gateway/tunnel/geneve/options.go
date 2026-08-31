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
	"fmt"
	"time"

	"github.com/liqotech/liqo/pkg/gateway"
)

// Options contains the options for the Geneve tunnel.
type Options struct {
	GwOptions *gateway.Options

	// MTU for the tunnel interface.
	MTU int

	// VNI is the Geneve Virtual Network Identifier. It must match on both sides.
	VNI int

	// Port is the local UDP port the tunnel binds for reception. The outer source
	// port used for transmission is not this one: the kernel always hashes it per
	// inner flow, which is what lets RSS and ECMP spread the tunnel.
	Port int

	// RemoteAddress is the address (IP or DNS name) of the peer endpoint.
	// Required in client mode; in server mode it is supplied later by the
	// PeerEndpoint resource.
	RemoteAddress string

	// RemotePort is the UDP port of the peer endpoint. A Geneve device transmits
	// to the port it was created with, so this is not applied to the device: it is
	// cross-checked against Port, since the two must be equal for the peering to
	// work at all. See ValidateOptions.
	RemotePort int

	// L3 carries inner payloads without an Ethernet header (inner_proto_inherit).
	// It must match on both sides: the kernel refuses to change it on an existing
	// device, and a mismatched pair silently fails to decapsulate.
	L3 bool

	// DNSCheckInterval is the interval between two DNS resolutions of the remote
	// endpoint address, when it is a DNS name.
	DNSCheckInterval time.Duration
}

// NewOptions returns a new Options struct.
func NewOptions(opts *gateway.Options) *Options {
	return &Options{
		GwOptions: opts,
	}
}

// ValidateOptions checks the consistency of the options with the gateway mode.
func ValidateOptions(opts *Options) error {
	if opts.VNI < 0 || opts.VNI >= 1<<24 {
		return fmt.Errorf("invalid VNI %d: must fit in 24 bits", opts.VNI)
	}
	if opts.Port <= 0 || opts.Port > 65535 {
		return fmt.Errorf("invalid port %d", opts.Port)
	}
	if opts.MTU <= 0 {
		return fmt.Errorf("invalid MTU %d", opts.MTU)
	}

	// The client is configured with the server's endpoint at creation time, since
	// the server is up first. The server learns the client's endpoint from the
	// PeerEndpoint resource, which arrives once the client has published it, so it
	// must be able to start without one.
	if opts.GwOptions.Mode == gateway.ModeClient {
		if opts.RemoteAddress == "" {
			return fmt.Errorf("flag --%s is required in client mode", FlagNameRemoteAddress)
		}
		if opts.RemotePort <= 0 || opts.RemotePort > 65535 {
			return fmt.Errorf("flag --%s must be a valid port in client mode, got %d",
				FlagNameRemotePort, opts.RemotePort)
		}

		// A Geneve device has a single UDP port, used both to listen and as the
		// transmit destination, and the kernel refuses to change it on a live
		// device. A peer reachable on a different port therefore black-holes the
		// return path instead of failing visibly, so the mismatch is rejected here.
		if opts.RemotePort != opts.Port {
			return fmt.Errorf(
				"peer endpoint port (--%s=%d) must equal the local tunnel port (--%s=%d): a Geneve device "+
					"uses one UDP port for both listening and transmitting, so the peer cannot be reached "+
					"through a translated port; align the gateway server's service port and node port with it",
				FlagNameRemotePort, opts.RemotePort, FlagNamePort, opts.Port)
		}
	}

	return nil
}

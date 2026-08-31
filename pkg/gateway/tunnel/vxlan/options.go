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

	"github.com/liqotech/liqo/pkg/gateway"
)

// TunnelMode selects how the peer endpoint is determined and, as a direct
// consequence, whether the outer UDP source port may be hashed.
type TunnelMode string

const (
	// TunnelModeNATTraversal makes the server learn the peer endpoint from the
	// outer source of received packets, so the tunnel works when the client is
	// behind NAT/PAT and has no inbound reachability. It requires the outer UDP
	// source port to be pinned, which prevents RSS/ECMP from spreading the
	// tunnel across queues and paths.
	TunnelModeNATTraversal TunnelMode = "nat-traversal"

	// TunnelModeStatic configures both peers with each other's endpoint, so each
	// side replies to a known port instead of the observed source. This frees the
	// outer UDP source port to be hashed per inner flow (RSS/ECMP work), at the
	// cost of requiring both gateways to be mutually reachable.
	TunnelModeStatic TunnelMode = "static"
)

// TunnelModes lists the supported tunnel modes.
var TunnelModes = []TunnelMode{TunnelModeNATTraversal, TunnelModeStatic}

// Options contains the options for the VXLAN tunnel.
type Options struct {
	GwOptions *gateway.Options

	// MTU for the tunnel interface.
	MTU int

	// VNI is the VXLAN Network Identifier. It must match on both sides of the tunnel.
	VNI int

	// Port is the local UDP port the tunnel binds for reception. In nat-traversal
	// mode it is also pinned as the outer source port for transmission.
	Port int

	// RemoteAddress is the address (IP or DNS name) of the peer endpoint.
	// Always required in client mode; in server mode it is optional (the peer
	// endpoint is provided by the learner or by the PeerEndpoint resource).
	RemoteAddress string

	// RemotePort is the UDP port of the peer endpoint. See RemoteAddress.
	RemotePort int

	// TunnelMode selects the endpoint-discovery strategy. See TunnelMode.
	TunnelMode TunnelMode

	// LearningCooldown is the minimum interval between two learned endpoint
	// updates, bounding the flap rate. Only used in nat-traversal mode.
	LearningCooldown time.Duration

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
	switch opts.TunnelMode {
	case TunnelModeNATTraversal, TunnelModeStatic:
	default:
		return fmt.Errorf("invalid tunnel mode %q: must be one of %v", opts.TunnelMode, TunnelModes)
	}

	if opts.VNI < 0 || opts.VNI >= 1<<24 {
		return fmt.Errorf("invalid VNI %d: must fit in 24 bits", opts.VNI)
	}
	// 65535 is excluded: pinning the source port needs an upper-exclusive [P, P+1) range.
	if opts.Port <= 0 || opts.Port >= 65535 {
		return fmt.Errorf("invalid port %d", opts.Port)
	}

	// The client always knows where the server is: it is the side that initiates.
	// The server needs no peer endpoint at startup — it is supplied later by the
	// learner (nat-traversal) or by the PeerEndpoint resource (static), which is
	// what decouples the tunnel from gateway startup ordering.
	if opts.GwOptions.Mode == gateway.ModeClient {
		if opts.RemoteAddress == "" {
			return fmt.Errorf("flag --%s is required in client mode", FlagNameRemoteAddress)
		}
		if opts.RemotePort <= 0 || opts.RemotePort > 65535 {
			return fmt.Errorf("flag --%s is required in client mode", FlagNameRemotePort)
		}
	}

	return nil
}

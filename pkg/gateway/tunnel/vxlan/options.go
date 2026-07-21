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

// Options contains the options for the VXLAN tunnel.
type Options struct {
	GwOptions *gateway.Options

	// MTU for the tunnel interface.
	MTU int

	// VNI is the VXLAN Network Identifier. It must match on both sides of the tunnel.
	VNI int

	// Port is the local UDP port: the VXLAN socket binds it for reception and the
	// source port range is pinned to it for transmission. The pin is load-bearing:
	// replies must leave from the same 5-tuple that received traffic, so that NAT
	// and LoadBalancer conntrack mappings are traversed in reverse.
	Port int

	// RemoteAddress is the address (IP or DNS name) of the gateway server endpoint.
	// Only meaningful in client mode.
	RemoteAddress string

	// RemotePort is the UDP port of the gateway server endpoint.
	// Only meaningful in client mode.
	RemotePort int

	// EndpointLearning enables learning the client endpoint from the outer source
	// of received VXLAN packets. Only meaningful in server mode.
	EndpointLearning bool

	// LearningCooldown is the minimum interval between two endpoint updates,
	// bounding the flap rate of the learned endpoint.
	LearningCooldown time.Duration

	// DNSCheckInterval is the interval between two DNS resolutions of the remote
	// endpoint address, when it is a DNS name. Only meaningful in client mode.
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
	// 65535 is excluded: the source-port pin needs an upper-exclusive [P, P+1) range.
	if opts.Port <= 0 || opts.Port >= 65535 {
		return fmt.Errorf("invalid port %d", opts.Port)
	}
	switch opts.GwOptions.Mode {
	case gateway.ModeClient:
		if opts.RemoteAddress == "" {
			return fmt.Errorf("flag --%s is required in client mode", FlagNameRemoteAddress)
		}
		if opts.RemotePort <= 0 || opts.RemotePort > 65535 {
			return fmt.Errorf("flag --%s is required in client mode", FlagNameRemotePort)
		}
	case gateway.ModeServer:
		// The server needs neither a remote address nor a remote port: the client
		// endpoint is learned from the data plane when EndpointLearning is enabled,
		// or set statically via the remote address/port flags otherwise.
		if !opts.EndpointLearning && (opts.RemoteAddress == "" || opts.RemotePort <= 0) {
			return fmt.Errorf("with --%s=false, --%s and --%s are required in server mode",
				FlagNameEndpointLearning, FlagNameRemoteAddress, FlagNameRemotePort)
		}
	}
	return nil
}

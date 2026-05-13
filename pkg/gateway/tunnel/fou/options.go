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

import "github.com/liqotech/liqo/pkg/gateway"

// Options contains the options for the FOU tunnel.
type Options struct {
	GwOptions *gateway.Options

	// MTU for the tunnel interface.
	MTU int

	// LocalPort is the UDP port this side listens on for incoming FoU packets.
	LocalPort int

	// RemotePort is the UDP encapsulation destination port on the remote side.
	RemotePort int

	// RemoteAddress is the IP address of the remote tunnel endpoint (peer's LoadBalancer IP).
	// Used as the fixed Remote address on the IPIP device so the kernel adds the outer IP + FoU
	// UDP header on egress without needing lwtunnel per-route encapsulation.
	RemoteAddress string
}

// NewOptions returns a new Options struct.
func NewOptions(opts *gateway.Options) *Options {
	return &Options{
		GwOptions: opts,
	}
}

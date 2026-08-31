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

// Package geneve contains the logic to configure the Geneve tunnel runtime
// used for plaintext cluster-to-cluster (external network) connectivity.
//
// The Geneve tunnel exists to make aggregated multi-cluster traffic spread
// across queues and paths. The kernel derives the outer UDP source port from a
// hash of the inner flow (`udp_flow_src_port` over `skb_get_hash`), so every
// inner 5-tuple presents as a distinct outer flow and both ECMP in the fabric
// and RSS on the receiving NIC can spread the tunnel.
//
// Unlike VXLAN, the Geneve driver offers no way to pin that source port: it is
// always hashed. That removes any possibility of a NAT-traversal mode, in which
// the server would have to reply to the observed source from the port it listens
// on. Both gateways are therefore configured with each other's endpoint and must
// be mutually reachable, with no NAT between them.
//
// The tunnel carries L3 payloads (`inner_proto_inherit`), so there is no inner
// Ethernet header, no MAC address to assign, and no neighbor entry to install:
// the device is addressed with the usual link-local /30 and the peer resolves
// through the connected route.
package geneve

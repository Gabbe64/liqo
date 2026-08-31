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
//
// Two properties of the kernel driver constrain how the gateway may be exposed,
// and both differ from VXLAN:
//
//   - On receive, `geneve_lookup` matches a packet on (VNI, outer source
//     address) against the device's configured remote. Any SNAT on the inbound
//     path therefore makes packets match no device and be silently dropped, so
//     the gateway Services use `externalTrafficPolicy: Local`. VXLAN has no such
//     constraint: it looks up by VNI alone and accepts any source.
//
//   - A device has a single UDP port, used both for listening and as the
//     transmit destination, and the kernel refuses to change it once the device
//     exists. The peer must therefore be reachable on exactly that port, which
//     is why the Services pin their node port to it. VXLAN instead carries a
//     per-destination port in its FDB entry (NDA_PORT) and can reach a peer
//     through a translated port.
package geneve

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
	"context"
	"fmt"
	"net"

	"github.com/vishvananda/netlink"
	"k8s.io/klog/v2"

	"github.com/liqotech/liqo/pkg/gateway/tunnel"
)

// InitGeneveLink creates (or reuses) the Geneve tunnel interface.
//
// The device receives every packet arriving on its UDP port with the matching
// VNI, and transmits to the address held in its Remote attribute. Because the
// tunnel carries L3 payloads there is no inner Ethernet header, so unlike the
// VXLAN runtime there is no MAC to assign, no ARP to disable and no neighbor
// entry to install: the peer's link-local address resolves through the
// connected route of the /30 assigned below.
//
// An existing device with matching parameters is reused rather than recreated,
// so that routes installed in custom tables survive a container restart.
// It returns the interface index of the tunnel device.
func InitGeneveLink(ctx context.Context, opts *Options) (int, error) {
	link, err := ensureGeneveDevice(opts)
	if err != nil {
		return 0, err
	}

	// Assign the link-local IP to the tunnel so that connection checking
	// (ping 169.254.18.x) continues to work.
	interfaceIP := tunnel.GetInterfaceIP(opts.GwOptions.Mode)
	if interfaceIP != "" {
		if err := ensureAddress(link, interfaceIP); err != nil {
			return 0, fmt.Errorf("cannot assign IP %s to tunnel interface %q: %w",
				interfaceIP, tunnel.TunnelInterfaceName, err)
		}
	}

	if err := netlink.LinkSetUp(link); err != nil {
		return 0, fmt.Errorf("cannot bring up tunnel interface %q: %w", tunnel.TunnelInterfaceName, err)
	}

	// The peer endpoint has two providers: configured at startup for the client,
	// which is created once the server has published its endpoint; and the
	// PeerEndpoint watcher for the server, whose peer is not known until the
	// client comes up. The latter is what decouples the tunnel from gateway
	// startup ordering.
	if opts.RemoteAddress != "" {
		remoteIP, err := ResolveRemoteIP(ctx, opts.RemoteAddress)
		if err != nil {
			return 0, err
		}
		if err := EnsureRemote(opts, link.Attrs().Index, remoteIP); err != nil {
			return 0, err
		}
	}

	klog.Infof("Geneve tunnel interface %q ready (vni=%d, port=%d, role=%s, l3=%t, mtu=%d, ip=%s)",
		tunnel.TunnelInterfaceName, opts.VNI, opts.Port, opts.GwOptions.Mode, opts.L3, opts.MTU, interfaceIP)
	return link.Attrs().Index, nil
}

// ensureGeneveDevice returns the tunnel device, creating it if absent and
// recreating it only when the existing device has incompatible parameters.
func ensureGeneveDevice(opts *Options) (netlink.Link, error) {
	if existing, err := tunnel.GetLink(tunnel.TunnelInterfaceName); err == nil {
		if gn, ok := existing.(*netlink.Geneve); ok && geneveParamsMatch(gn, opts) {
			klog.Infof("Reusing existing Geneve interface %q", tunnel.TunnelInterfaceName)
			if existing.Attrs().MTU != opts.MTU {
				if err := netlink.LinkSetMTU(existing, opts.MTU); err != nil {
					return nil, fmt.Errorf("cannot set MTU on interface %q: %w", tunnel.TunnelInterfaceName, err)
				}
			}
			return existing, nil
		}
		klog.Infof("Interface %q exists with different parameters, recreating it", tunnel.TunnelInterfaceName)
		if err := netlink.LinkDel(existing); err != nil {
			return nil, fmt.Errorf("cannot delete existing tunnel interface %q: %w", tunnel.TunnelInterfaceName, err)
		}
	}

	// Remote is deliberately left unset: the kernel accepts a Geneve device with
	// no peer (iproute2 refuses it, but that check is userspace only), and the
	// server does not know its peer until the PeerEndpoint arrives. Until then the
	// device simply has nowhere to transmit, which is the safe default.
	if err := netlink.LinkAdd(forgeGeneveLink(opts, nil)); err != nil {
		return nil, fmt.Errorf("cannot create Geneve interface %q: %w", tunnel.TunnelInterfaceName, err)
	}

	return tunnel.GetLink(tunnel.TunnelInterfaceName)
}

// forgeGeneveLink builds the desired device definition.
//
// Note that the outer UDP checksum cannot be requested here: the netlink library
// never serializes IFLA_GENEVE_UDP_CSUM, so the device is created with
// checksums disabled. That is legal for IPv4 and saves the computation, but it
// differs from the VXLAN runtime, which enables them.
func forgeGeneveLink(opts *Options, remote net.IP) *netlink.Geneve {
	return &netlink.Geneve{
		LinkAttrs: netlink.LinkAttrs{
			Name: tunnel.TunnelInterfaceName,
			MTU:  opts.MTU,
		},
		ID:     uint32(opts.VNI),  //nolint:gosec // validated in ValidateOptions
		Dport:  uint16(opts.Port), //nolint:gosec // validated in ValidateOptions
		Remote: remote,
		// Set the don't-fragment bit on the outer header. Without it an MTU
		// shortfall silently fragments the tunnel instead of failing: only the
		// first fragment carries the UDP header, so ECMP would spread the
		// fragments of one packet across different paths.
		Df:                netlink.GENEVE_DF_SET,
		InnerProtoInherit: opts.L3,
	}
}

// geneveParamsMatch tells whether the existing device can be reused as is.
// Reuse matters: recreating the device invalidates every route that references
// its index, so a tunnel-container restart would silently drop the dataplane
// until something re-triggers the route controllers.
//
// Only the parameters the kernel refuses to change are compared, and only those
// the netlink library actually parses back. Df is excluded because
// parseGeneveData ignores IFLA_GENEVE_DF, so it always deserializes as unset;
// EnsureRemote re-asserts it on every update instead. The MTU is excluded too:
// it is adjusted in place.
func geneveParamsMatch(gn *netlink.Geneve, opts *Options) bool {
	return gn.ID == uint32(opts.VNI) && //nolint:gosec // validated in ValidateOptions
		gn.Dport == uint16(opts.Port) && //nolint:gosec // validated in ValidateOptions
		gn.InnerProtoInherit == opts.L3
}

// EnsureRemote points the tunnel at the given peer address, without recreating
// the device, so that routes referencing its index survive an endpoint change.
//
// Two attributes must be withheld from the request. The kernel rejects a
// changelink carrying IFLA_GENEVE_PORT or IFLA_GENEVE_INNER_PROTO_INHERIT
// outright — not only when the value differs — and the resulting EOPNOTSUPP
// names none of them. Both are preserved by the kernel when absent.
//
// Df, conversely, must be re-asserted every time: the library always serializes
// it but never parses it back, so a request built from the current device state
// would carry Df=unset and silently clear the don't-fragment bit.
func EnsureRemote(opts *Options, linkIndex int, remote net.IP) error {
	remote4 := remote.To4()
	if remote4 == nil {
		return fmt.Errorf("peer endpoint %q is not an IPv4 address", remote)
	}

	link, err := netlink.LinkByIndex(linkIndex)
	if err != nil {
		return fmt.Errorf("cannot get tunnel interface by index %d: %w", linkIndex, err)
	}
	gn, ok := link.(*netlink.Geneve)
	if !ok {
		return fmt.Errorf("interface %q is not a Geneve device", link.Attrs().Name)
	}
	if gn.Remote.Equal(remote4) {
		return nil
	}

	upd := forgeGeneveLink(opts, remote4)
	upd.Dport = 0
	upd.InnerProtoInherit = false

	if err := netlink.LinkModify(upd); err != nil {
		return fmt.Errorf("cannot set tunnel peer to %s: %w", remote4, err)
	}

	klog.Infof("Geneve tunnel peer set to %s (was %s)", remote4, gn.Remote)
	endpointUpdates.Inc()
	endpointLastUpdate.SetToCurrentTime()
	return nil
}

// ensureAddress adds the given address to the link if not already present.
func ensureAddress(link netlink.Link, ip string) error {
	addr, err := netlink.ParseAddr(ip)
	if err != nil {
		return err
	}
	existing, err := netlink.AddrList(link, netlink.FAMILY_V4)
	if err != nil {
		return fmt.Errorf("cannot list addresses: %w", err)
	}
	for i := range existing {
		if existing[i].IPNet != nil && existing[i].IP.Equal(addr.IP) {
			return nil
		}
	}
	return netlink.AddrAdd(link, addr)
}

// ResolveRemoteIP returns the IPv4 address for the given endpoint address,
// resolving it through DNS when it is not a literal IP (e.g., the hostname
// of a cloud LoadBalancer).
func ResolveRemoteIP(ctx context.Context, address string) (net.IP, error) {
	if ip := net.ParseIP(address); ip != nil {
		if ip4 := ip.To4(); ip4 != nil {
			return ip4, nil
		}
		return nil, fmt.Errorf("remote address %q is not an IPv4 address", address)
	}

	ips, err := net.DefaultResolver.LookupIPAddr(ctx, address)
	if err != nil {
		return nil, fmt.Errorf("cannot resolve remote address %q: %w", address, err)
	}
	for i := range ips {
		if ip4 := ips[i].IP.To4(); ip4 != nil {
			return ip4, nil
		}
	}
	return nil, fmt.Errorf("remote address %q resolved to no IPv4 address", address)
}

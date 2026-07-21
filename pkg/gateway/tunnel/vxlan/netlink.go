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
	"context"
	"fmt"
	"net"

	"github.com/vishvananda/netlink"
	"k8s.io/klog/v2"

	"github.com/liqotech/liqo/pkg/gateway"
	"github.com/liqotech/liqo/pkg/gateway/tunnel"
)

// serverMAC and clientMAC are the deterministic, locally-administered MAC
// addresses assigned to the tunnel device depending on the gateway mode. Fixed
// MACs allow fully static neighbor entries: inner frames travel as proper
// unicast (a unicast IP datagram inside a link-layer broadcast frame would be
// silently discarded by the receiver, per RFC 1122 section 3.3.6), while the
// egress destination still resolves through the all-zeros FDB entry, since
// unknown unicast MACs fall back to it.
var (
	serverMAC = net.HardwareAddr{0x02, 0x6c, 0x71, 0x6f, 0x00, 0x01}
	clientMAC = net.HardwareAddr{0x02, 0x6c, 0x71, 0x6f, 0x00, 0x02}
)

// localMAC returns the MAC address of the local tunnel device.
func localMAC(mode gateway.Mode) net.HardwareAddr {
	if mode == gateway.ModeServer {
		return serverMAC
	}
	return clientMAC
}

// peerMAC returns the MAC address of the peer tunnel device.
func peerMAC(mode gateway.Mode) net.HardwareAddr {
	if mode == gateway.ModeServer {
		return clientMAC
	}
	return serverMAC
}

// InitVxlanLink creates (or reuses) the VXLAN tunnel interface.
//
// The device receives every packet arriving on its UDP port with the matching
// VNI, regardless of the outer source address, and transmits to the all-zeros
// FDB entry (the "default destination"). ARP is disabled: the device MAC is
// fixed per role and the peer is installed as a permanent neighbor entry, so
// no dynamic neighbor state is ever needed.
//
// An existing device with matching parameters is reused rather than recreated,
// so that routes installed in custom tables survive a container restart.
// It returns the interface index of the tunnel device.
func InitVxlanLink(ctx context.Context, opts *Options) (int, error) {
	link, err := ensureVxlanDevice(opts)
	if err != nil {
		return 0, err
	}

	if err := netlink.LinkSetHardwareAddr(link, localMAC(opts.GwOptions.Mode)); err != nil {
		return 0, fmt.Errorf("cannot set MAC address on interface %q: %w", tunnel.TunnelInterfaceName, err)
	}

	if err := netlink.LinkSetARPOff(link); err != nil {
		return 0, fmt.Errorf("cannot disable ARP on interface %q: %w", tunnel.TunnelInterfaceName, err)
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

	// Install the permanent neighbor entry for the peer interface address, so
	// that inner frames are emitted as unicast without any ARP resolution.
	if err := ensurePeerNeighbor(link, opts.GwOptions.Mode); err != nil {
		return 0, err
	}

	// The client (and the server in static mode) points the default destination
	// at the configured remote endpoint. In learning mode the server leaves it
	// unset: it is installed as soon as the first client packet is observed.
	if opts.GwOptions.Mode == gateway.ModeClient || !opts.EndpointLearning {
		remoteIP, err := ResolveRemoteIP(ctx, opts.RemoteAddress)
		if err != nil {
			return 0, err
		}
		if err := EnsureFdbEntry(link.Attrs().Index, remoteIP, uint16(opts.RemotePort)); err != nil { //nolint:gosec // port fits in uint16
			return 0, fmt.Errorf("cannot set FDB default destination to %s:%d: %w", remoteIP, opts.RemotePort, err)
		}
	}

	klog.Infof("VXLAN tunnel interface %q ready (vni=%d, port=%d, mode=%s, ip=%s)",
		tunnel.TunnelInterfaceName, opts.VNI, opts.Port, opts.GwOptions.Mode, interfaceIP)
	return link.Attrs().Index, nil
}

// ensureVxlanDevice returns the tunnel device, creating it if absent and
// recreating it only when the existing device has incompatible parameters.
func ensureVxlanDevice(opts *Options) (netlink.Link, error) {
	if existing, err := tunnel.GetLink(tunnel.TunnelInterfaceName); err == nil {
		if vx, ok := existing.(*netlink.Vxlan); ok && vxlanParamsMatch(vx, opts) {
			klog.Infof("Reusing existing VXLAN interface %q", tunnel.TunnelInterfaceName)
			return existing, nil
		}
		klog.Infof("Interface %q exists with different parameters, recreating it", tunnel.TunnelInterfaceName)
		if err := netlink.LinkDel(existing); err != nil {
			return nil, fmt.Errorf("cannot delete existing tunnel interface %q: %w", tunnel.TunnelInterfaceName, err)
		}
	}

	link := &netlink.Vxlan{
		LinkAttrs: netlink.LinkAttrs{
			Name: tunnel.TunnelInterfaceName,
			MTU:  opts.MTU,
		},
		VxlanId: opts.VNI,
		Port:    opts.Port,
		// The kernel source-port range is upper-exclusive, and an empty range
		// (low == high) silently falls back to the ephemeral port range: [P, P+1)
		// is the way to pin the source port to P. The pin is load-bearing: all
		// tunnel traffic must share one outer 5-tuple so that the peer observes a
		// single stable endpoint and replies traverse NAT/conntrack in reverse.
		PortLow:  opts.Port,
		PortHigh: opts.Port + 1,
		Learning: false,
		UDPCSum:  true,
	}

	if err := netlink.LinkAdd(link); err != nil {
		return nil, fmt.Errorf("cannot create VXLAN interface %q: %w", tunnel.TunnelInterfaceName, err)
	}

	created, err := tunnel.GetLink(tunnel.TunnelInterfaceName)
	if err != nil {
		return nil, fmt.Errorf("cannot get tunnel interface after creation: %w", err)
	}
	return created, nil
}

// vxlanParamsMatch tells whether the existing device can be reused as is.
// The MTU is excluded: it is adjusted in place.
func vxlanParamsMatch(vx *netlink.Vxlan, opts *Options) bool {
	return vx.VxlanId == opts.VNI &&
		vx.Port == opts.Port &&
		vx.PortLow == opts.Port &&
		vx.PortHigh == opts.Port+1 &&
		!vx.Learning
}

// ensurePeerNeighbor installs a permanent neighbor entry mapping the peer's
// link-local interface address to its deterministic MAC.
func ensurePeerNeighbor(link netlink.Link, mode gateway.Mode) error {
	remoteIP, err := tunnel.GetRemoteInterfaceIP(mode)
	if err != nil {
		return fmt.Errorf("cannot get remote interface IP: %w", err)
	}
	if err := netlink.NeighSet(&netlink.Neigh{
		LinkIndex:    link.Attrs().Index,
		IP:           net.ParseIP(remoteIP),
		HardwareAddr: peerMAC(mode),
		State:        netlink.NUD_PERMANENT,
	}); err != nil {
		return fmt.Errorf("cannot install neighbor entry for peer %s: %w", remoteIP, err)
	}
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

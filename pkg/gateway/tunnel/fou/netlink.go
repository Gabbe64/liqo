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
	"context"
	"errors"
	"fmt"
	"net"

	"github.com/vishvananda/netlink"
	"golang.org/x/sys/unix"
	"k8s.io/klog/v2"

	"github.com/liqotech/liqo/pkg/gateway/tunnel"
)

// InboundIPIPTunnelName is the name given to the kernel-auto-created tunl0 wildcard
// IPIP fallback interface. When any IPIP tunnel is created, the kernel automatically
// creates tunl0 as a catch-all device (Remote=0.0.0.0 / Local=0.0.0.0) that
// decapsulates IPIP packets from any source. Liqo renames it to this name so that
// RouteConfiguration rules can reference it by a stable, recognisable name.
const InboundIPIPTunnelName = "liqo-tunnel-in"

// The tunnel device is created in fixed-remote mode: Remote is set to the peer's LoadBalancer
// IP so the kernel's ip_tunnel egress path adds the outer IP header and the FoU UDP header
// (via the TUNNEL_ENCAP_FOU hook) without requiring per-route lwtunnel encapsulation.
// Inbound asymmetry is handled automatically: packets arriving from the remote node IP
// (rather than the LB IP) are decapsulated by the FoU listener and then fall through to
// the tunl0 wildcard IPIP device (renamed to InboundIPIPTunnelName) which accepts
// any-source IPIP.
func InitFouLink(_ context.Context, opts *Options) error {
	// 1. Register the FOU receive port with the kernel (idempotent – ignore EEXIST).
	if err := ensureFouPort(opts.LocalPort); err != nil {
		return fmt.Errorf("cannot configure FOU receive port %d: %w", opts.LocalPort, err)
	}

	// 2. Delete the egress IPIP-over-FOU tunnel if it already exists so it can be
	// recreated with the current options (remote address, ports, MTU may have changed).
	if lnk, err := tunnel.GetLink(tunnel.TunnelInterfaceName); err == nil {
		klog.Infof("FOU tunnel interface %q already exists, deleting for re-creation", tunnel.TunnelInterfaceName)
		if delErr := netlink.LinkDel(lnk); delErr != nil {
			return fmt.Errorf("cannot delete existing tunnel interface %q: %w", tunnel.TunnelInterfaceName, delErr)
		}
	}

	remote := net.ParseIP(opts.RemoteAddress)
	if remote == nil {
		return fmt.Errorf("invalid remote address %q for FOU tunnel", opts.RemoteAddress)
	}

	link := &netlink.Iptun{
		LinkAttrs: netlink.LinkAttrs{
			MTU:  opts.MTU,
			Name: tunnel.TunnelInterfaceName,
		},
		Remote:     remote,
		EncapType:  uint16(netlink.FOU),
		EncapDport: uint16(opts.RemotePort), //nolint:gosec // port fits in uint16
	}

	if err := netlink.LinkAdd(link); err != nil {
		return fmt.Errorf("cannot create IPIP-over-FOU interface %q: %w", tunnel.TunnelInterfaceName, err)
	}

	lnk, err := tunnel.GetLink(tunnel.TunnelInterfaceName)
	if err != nil {
		return fmt.Errorf("cannot get tunnel interface after creation: %w", err)
	}

	// Assign the link-local IP to the tunnel so that connection checking
	// (ping 169.254.18.x) continues to work.
	interfaceIP := tunnel.GetInterfaceIP(opts.GwOptions.Mode)
	if interfaceIP != "" {
		if err := tunnel.AddAddress(lnk, interfaceIP); err != nil {
			return fmt.Errorf("cannot assign IP %s to tunnel interface %q: %w",
				interfaceIP, tunnel.TunnelInterfaceName, err)
		}
	}

	if err := netlink.LinkSetUp(lnk); err != nil {
		return fmt.Errorf("cannot bring up tunnel interface %q: %w", tunnel.TunnelInterfaceName, err)
	}

	// Rename the kernel-auto-created tunl0 wildcard IPIP device to InboundIPIPTunnelName
	// and bring it up so RouteConfiguration rules can reference it by a stable name.
	if err := ensureInboundInterface(); err != nil {
		return fmt.Errorf("cannot set up inbound IPIP interface %q: %w", InboundIPIPTunnelName, err)
	}

	klog.Infof("FOU tunnel interface %q created (remote=%s, local-port=%d, remote-port=%d, ip=%s)",
		tunnel.TunnelInterfaceName, opts.RemoteAddress, opts.LocalPort, opts.RemotePort, interfaceIP)
	return nil
}

// FindIPIPFallbackTunnel finds the kernel-auto-created IPIP fallback (catch-all) interface.
// When the IPIP module is loaded, the kernel always creates one interface with both
// Local and Remote set to 0.0.0.0 that decapsulates IPIP packets from any source.
// This is typically named "tunl0" but we identify it by its properties rather than
// its name, since it may have been renamed by a previous run.
func FindIPIPFallbackTunnel() (netlink.Link, error) {
	links, err := netlink.LinkList()
	if err != nil {
		return nil, fmt.Errorf("cannot list network interfaces: %w", err)
	}
	anyIP := net.IPv4zero
	for i := range links {
		iptun, ok := links[i].(*netlink.Iptun)
		if !ok {
			continue
		}
		localIsAny := iptun.Local == nil || iptun.Local.Equal(anyIP)
		remoteIsAny := iptun.Remote == nil || iptun.Remote.Equal(anyIP)
		if localIsAny && remoteIsAny {
			return links[i], nil
		}
	}
	return nil, fmt.Errorf("no IPIP fallback interface found, make sure to load the ipip kernel module before initializing the tunnel.")
}

// ensureInboundInterface renames the kernel-auto-created tunl0 wildcard IPIP interface
// to InboundIPIPTunnelName and brings it up. tunl0 is always present after any IPIP
// tunnel is created (the kernel creates it automatically as a catch-all fallback device).
// If the interface has already been renamed in a previous call, this is a no-op.
func ensureInboundInterface() error {
	// If already renamed, just make sure it is up.
	if lnk, err := tunnel.GetLink(InboundIPIPTunnelName); err == nil {
		if upErr := netlink.LinkSetUp(lnk); upErr != nil {
			return fmt.Errorf("cannot bring up inbound IPIP interface %q: %w", InboundIPIPTunnelName, upErr)
		}
		return nil
	}

	fallback, err := FindIPIPFallbackTunnel()
	if err != nil {
		return fmt.Errorf("cannot find IPIP fallback tunnel to rename: %w", err)
	}

	if err := netlink.LinkSetDown(fallback); err != nil {
		return fmt.Errorf("cannot bring down IPIP fallback interface before rename: %w", err)
	}
	if err := netlink.LinkSetName(fallback, InboundIPIPTunnelName); err != nil {
		return fmt.Errorf("cannot rename IPIP fallback interface to %q: %w", InboundIPIPTunnelName, err)
	}

	// Re-fetch after rename so the handle is up to date.
	lnk, err := tunnel.GetLink(InboundIPIPTunnelName)
	if err != nil {
		return fmt.Errorf("cannot get interface after rename: %w", err)
	}

	if err := netlink.LinkSetUp(lnk); err != nil {
		return fmt.Errorf("cannot bring up inbound IPIP interface %q: %w", InboundIPIPTunnelName, err)
	}

	return nil
}

// ensureFouPort registers a FOU UDP receive port with the kernel.
// It is safe to call when the port is already registered.
func ensureFouPort(port int) error {
	cfg := netlink.Fou{
		Family:    unix.AF_INET,
		Port:      port,
		Protocol:  unix.IPPROTO_IPIP,
		EncapType: netlink.FOU_ENCAP_DIRECT,
	}
	err := netlink.FouAdd(cfg)
	if err == nil {
		klog.Infof("FOU receive port %d registered", port)
		return nil
	}
	// EEXIST: port is already registered in the FoU table — nothing to do.
	if errors.Is(err, unix.EEXIST) {
		klog.V(4).Infof("FOU receive port %d already registered", port)
		return nil
	}
	// EADDRINUSE: on older kernels the FoU subsystem returns this instead of EEXIST
	// when the internal UDP socket for this port is already bound by a stale previous
	// registration (e.g. the pod crashed before CleanupFouPort could run, or the
	// container network namespace was reused). Delete the stale entry and re-register
	// so we own a fresh, correctly-configured FoU listener.
	if errors.Is(err, unix.EADDRINUSE) {
		klog.Infof("FOU receive port %d has a stale registration; cleaning up and re-registering", port)
		if delErr := netlink.FouDel(netlink.Fou{Family: unix.AF_INET, Port: port}); delErr != nil {
			return fmt.Errorf("cannot clean up stale FOU registration for port %d: %w", port, delErr)
		}
		if retryErr := netlink.FouAdd(cfg); retryErr != nil {
			return fmt.Errorf("cannot re-register FOU port %d after cleanup: %w", port, retryErr)
		}
		klog.Infof("FOU receive port %d re-registered after stale cleanup", port)
		return nil
	}
	return err
}

// CleanupFouPort deregisters the FOU UDP receive port from the kernel.
// It should be called when the FOU runtime shuts down so the port is not
// left registered after the container exits.
func CleanupFouPort(port int) {
	err := netlink.FouDel(netlink.Fou{
		Family: unix.AF_INET,
		Port:   port,
	})
	if err == nil {
		klog.Infof("FOU receive port %d deregistered", port)
		return
	}
	klog.Warningf("Failed to deregister FOU receive port %d: %v", port, err)
}

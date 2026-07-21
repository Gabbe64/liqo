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
	"bytes"
	"encoding/binary"
	"fmt"
	"net"

	"github.com/vishvananda/netlink"
	"github.com/vishvananda/netlink/nl"
	"golang.org/x/sys/unix"
	"k8s.io/klog/v2"
)

// zeroMAC is the all-zeros FDB entry: the VXLAN "default destination" used for
// every frame whose inner destination MAC has no specific entry (including the
// broadcast frames emitted by the ARP-less tunnel device).
var zeroMAC = net.HardwareAddr{0, 0, 0, 0, 0, 0}

// FdbDestination is one destination of the all-zeros-MAC FDB entry.
type FdbDestination struct {
	IP   net.IP
	Port uint16
}

func (d FdbDestination) String() string {
	return fmt.Sprintf("%s:%d", d.IP, d.Port)
}

// EnsureFdbEntry makes dst:port the only destination of the all-zeros-MAC FDB
// entry of the given VXLAN device: it appends the desired destination (the
// kernel only supports append semantics for the default-destination list, not
// NLM_F_REPLACE) and then removes every other destination.
//
// The requests are crafted manually because the vishvananda/netlink Neigh API
// does not expose the NDA_PORT attribute, which is required to reach peers
// behind port-translating NATs.
func EnsureFdbEntry(linkIndex int, dst net.IP, port uint16) error {
	dst4 := dst.To4()
	if dst4 == nil {
		return fmt.Errorf("FDB destination %q is not an IPv4 address", dst)
	}
	desired := FdbDestination{IP: dst4, Port: port}

	current, err := listFdbDestinations(linkIndex)
	if err != nil {
		return err
	}

	found := false
	for _, d := range current {
		if d.IP.Equal(desired.IP) && d.Port == desired.Port {
			found = true
			break
		}
	}
	if !found {
		if err := addFdbDestination(linkIndex, desired); err != nil {
			return err
		}
	}

	// Remove stale destinations (e.g., a previous peer endpoint after a move, or
	// leftovers from a previous run on a reused device) so that traffic is not
	// flooded to dead addresses.
	for _, d := range current {
		if d.IP.Equal(desired.IP) && d.Port == desired.Port {
			continue
		}
		if err := delFdbDestination(linkIndex, d); err != nil {
			klog.Warningf("Cannot remove stale FDB destination %s: %v", d, err)
		}
	}
	return nil
}

// addFdbDestination appends a destination to the all-zeros-MAC FDB entry.
func addFdbDestination(linkIndex int, dst FdbDestination) error {
	req := nl.NewNetlinkRequest(unix.RTM_NEWNEIGH, unix.NLM_F_CREATE|unix.NLM_F_APPEND|unix.NLM_F_ACK)
	addFdbPayload(req, linkIndex, dst)
	if _, err := req.Execute(unix.NETLINK_ROUTE, 0); err != nil {
		return fmt.Errorf("cannot append FDB destination %s: %w", dst, err)
	}
	return nil
}

// delFdbDestination removes a destination from the all-zeros-MAC FDB entry.
func delFdbDestination(linkIndex int, dst FdbDestination) error {
	req := nl.NewNetlinkRequest(unix.RTM_DELNEIGH, unix.NLM_F_ACK)
	addFdbPayload(req, linkIndex, dst)
	if _, err := req.Execute(unix.NETLINK_ROUTE, 0); err != nil {
		return fmt.Errorf("cannot delete FDB destination %s: %w", dst, err)
	}
	return nil
}

func addFdbPayload(req *nl.NetlinkRequest, linkIndex int, dst FdbDestination) {
	req.AddData(&netlink.Ndmsg{
		Family: unix.AF_BRIDGE,
		Index:  uint32(linkIndex), //nolint:gosec // interface indexes fit in uint32
		State:  netlink.NUD_PERMANENT | netlink.NUD_NOARP,
		Flags:  netlink.NTF_SELF,
	})
	req.AddData(nl.NewRtAttr(netlink.NDA_LLADDR, zeroMAC))
	req.AddData(nl.NewRtAttr(netlink.NDA_DST, dst.IP.To4()))
	if dst.Port != 0 {
		portData := make([]byte, 2)
		binary.BigEndian.PutUint16(portData, dst.Port)
		req.AddData(nl.NewRtAttr(netlink.NDA_PORT, portData))
	}
}

// listFdbDestinations dumps the destinations of the all-zeros-MAC FDB entry of
// the given device, including the per-destination UDP port that the standard
// netlink Neigh API does not parse. The kernel omits NDA_PORT when it equals
// the device destination port, so missing ports are normalized to it.
func listFdbDestinations(linkIndex int) ([]FdbDestination, error) {
	var defaultPort uint16
	if link, err := netlink.LinkByIndex(linkIndex); err == nil {
		if vx, ok := link.(*netlink.Vxlan); ok {
			defaultPort = uint16(vx.Port) //nolint:gosec // ports fit in uint16
		}
	}

	req := nl.NewNetlinkRequest(unix.RTM_GETNEIGH, unix.NLM_F_DUMP)
	req.AddData(&netlink.Ndmsg{
		Family: unix.AF_BRIDGE,
		Index:  uint32(linkIndex), //nolint:gosec // interface indexes fit in uint32
	})

	msgs, err := req.Execute(unix.NETLINK_ROUTE, unix.RTM_NEWNEIGH)
	if err != nil {
		return nil, fmt.Errorf("cannot dump FDB entries: %w", err)
	}

	const ndmsgLen = 12
	var dests []FdbDestination
	for _, m := range msgs {
		if len(m) < ndmsgLen {
			continue
		}
		// ndmsg layout: family(1) pad(3) ifindex(4) state(2) flags(1) type(1).
		ifindex := int(int32(binary.LittleEndian.Uint32(m[4:8]))) //nolint:gosec // fits by construction
		if ifindex != linkIndex {
			continue
		}

		attrs, err := nl.ParseRouteAttr(m[ndmsgLen:])
		if err != nil {
			continue
		}
		var dst FdbDestination
		zero := false
		for i := range attrs {
			switch attrs[i].Attr.Type {
			case netlink.NDA_LLADDR:
				zero = bytes.Equal(attrs[i].Value, zeroMAC)
			case netlink.NDA_DST:
				if len(attrs[i].Value) == net.IPv4len {
					dst.IP = net.IP(append([]byte(nil), attrs[i].Value...))
				}
			case netlink.NDA_PORT:
				if len(attrs[i].Value) == 2 {
					dst.Port = binary.BigEndian.Uint16(attrs[i].Value)
				}
			}
		}
		if zero && dst.IP != nil {
			if dst.Port == 0 {
				dst.Port = defaultPort
			}
			dests = append(dests, dst)
		}
	}
	return dests, nil
}

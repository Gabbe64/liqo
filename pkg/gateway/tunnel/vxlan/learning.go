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
	"encoding/binary"
	"fmt"
	"net"
	"time"

	"golang.org/x/net/bpf"
	"golang.org/x/sys/unix"
	"k8s.io/klog/v2"
)

const (
	// learnSnapLen is the number of bytes of each matched packet delivered to the
	// learner: outer IP header (max 60) + UDP header (8) + VXLAN header (8).
	learnSnapLen = 76

	// vxlanFlagVNIValid is the VXLAN header flag marking a valid VNI field.
	vxlanFlagVNIValid = 0x08
)

// RunEndpointLearner observes the VXLAN packets received on the local UDP port
// with the expected VNI, and keeps the FDB default destination pointed at their
// outer source (address and port). Replying to the observed source is what makes
// the tunnel traverse NATs and LoadBalancers whose ingress and egress addresses
// differ: return traffic rides the exact conntrack mappings created by the
// forward traffic.
//
// It blocks until the context is canceled.
func RunEndpointLearner(ctx context.Context, opts *Options, linkIndex int) error {
	fd, err := openLearningSocket(uint16(opts.Port), uint32(opts.VNI)) //nolint:gosec // validated in ValidateOptions
	if err != nil {
		return fmt.Errorf("cannot open endpoint learning socket: %w", err)
	}

	// Close the socket on context cancellation to unblock Recvfrom.
	go func() {
		<-ctx.Done()
		_ = unix.Close(fd)
	}()

	klog.Infof("Endpoint learning started (port=%d, vni=%d)", opts.Port, opts.VNI)

	var (
		currentIP   net.IP
		currentPort uint16
		lastUpdate  time.Time
	)

	buf := make([]byte, learnSnapLen)
	for {
		n, from, err := unix.Recvfrom(fd, buf, 0)
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			if err == unix.EINTR {
				continue
			}
			return fmt.Errorf("cannot read from endpoint learning socket: %w", err)
		}

		// Ignore packets emitted by this host: only inbound traffic reveals the peer.
		if ll, ok := from.(*unix.SockaddrLinklayer); ok && ll.Pkttype == unix.PACKET_OUTGOING {
			continue
		}

		srcIP, srcPort, ok := parseOuterSource(buf[:n])
		if !ok {
			continue
		}

		// The in-kernel filter already excludes the learned endpoint, but packets
		// queued before a filter swap may still surface here: re-check in memory.
		if srcIP.Equal(currentIP) && srcPort == currentPort {
			continue
		}
		if time.Since(lastUpdate) < opts.LearningCooldown {
			// Bound the endpoint flap rate: a legitimate move is re-observed on the
			// next packet once the cooldown has expired.
			continue
		}

		if err := EnsureFdbEntry(linkIndex, srcIP, srcPort); err != nil {
			klog.Errorf("Cannot update learned endpoint to %s:%d: %v", srcIP, srcPort, err)
			continue
		}

		klog.Infof("Learned peer endpoint %s:%d (was %s:%d)", srcIP, srcPort, currentIP, currentPort)
		currentIP = srcIP
		currentPort = srcPort
		lastUpdate = time.Now()
		endpointUpdates.Inc()
		endpointLastUpdate.SetToCurrentTime()

		// Make the filter exclude the new endpoint, so that steady-state traffic
		// is discarded in kernel and never copied to this loop.
		if err := attachLearningFilter(fd, uint16(opts.Port), uint32(opts.VNI), //nolint:gosec // validated in ValidateOptions
			&FdbDestination{IP: currentIP, Port: currentPort}); err != nil {
			klog.Warningf("Cannot update learning filter: %v", err)
		}
	}
}

// openLearningSocket opens an AF_PACKET socket (cooked mode, so packets start at
// the IP header) filtered in kernel to VXLAN packets for the given port and VNI.
func openLearningSocket(port uint16, vni uint32) (int, error) {
	// ETH_P_IP in network byte order (htons).
	const ethPIP = uint16(unix.ETH_P_IP)
	proto := int((ethPIP&0xff)<<8 | ethPIP>>8)
	fd, err := unix.Socket(unix.AF_PACKET, unix.SOCK_DGRAM|unix.SOCK_CLOEXEC, proto)
	if err != nil {
		return -1, fmt.Errorf("cannot create packet socket: %w", err)
	}

	if err := attachLearningFilter(fd, port, vni, nil); err != nil {
		_ = unix.Close(fd)
		return -1, err
	}
	return fd, nil
}

// attachLearningFilter (re)builds the learning filter and atomically attaches it
// to the socket, replacing any previous program. Passing the currently learned
// endpoint makes the filter self-excluding: in steady state every tunnel packet
// is discarded in kernel and userspace only sees packets from a new source.
func attachLearningFilter(fd int, port uint16, vni uint32, current *FdbDestination) error {
	filter, err := buildLearningFilter(port, vni, current)
	if err != nil {
		return err
	}
	if err := unix.SetsockoptSockFprog(fd, unix.SOL_SOCKET, unix.SO_ATTACH_FILTER, filter); err != nil {
		return fmt.Errorf("cannot attach BPF filter: %w", err)
	}
	return nil
}

// buildLearningFilter assembles the classic BPF program matching non-fragmented
// IPv4/UDP packets destined to the given port and carrying a VXLAN header with
// the given VNI. Offsets are relative to the IP header (cooked capture).
//
// When exclude is non-nil, packets whose outer source matches it are discarded
// in kernel: only packets revealing a *moved* endpoint reach userspace.
func buildLearningFilter(port uint16, vni uint32, exclude *FdbDestination) (*unix.SockFprog, error) {
	// The accept/drop epilogue shifts down when the exclusion block is present.
	excludeLen := uint8(0)
	if exclude != nil {
		excludeLen = 4
	}
	// toDrop returns the SkipTrue displacement from instruction index i to the
	// final "drop" instruction (index 17 without exclusion, 21 with it).
	toDrop := func(i uint8) uint8 { return 17 + excludeLen - i - 1 }

	insns := []bpf.Instruction{
		// IPv4 only.
		bpf.LoadAbsolute{Off: 0, Size: 1},
		bpf.ALUOpConstant{Op: bpf.ALUOpAnd, Val: 0xf0},
		bpf.JumpIf{Cond: bpf.JumpNotEqual, Val: 0x40, SkipTrue: toDrop(2)},
		// UDP only.
		bpf.LoadAbsolute{Off: 9, Size: 1},
		bpf.JumpIf{Cond: bpf.JumpNotEqual, Val: uint32(unix.IPPROTO_UDP), SkipTrue: toDrop(4)},
		// No fragments (the UDP and VXLAN headers must be in this packet).
		bpf.LoadAbsolute{Off: 6, Size: 2},
		bpf.JumpIf{Cond: bpf.JumpBitsSet, Val: 0x1fff, SkipTrue: toDrop(6)},
		// X <- IP header length.
		bpf.LoadMemShift{Off: 0},
		// UDP destination port.
		bpf.LoadIndirect{Off: 2, Size: 2},
		bpf.JumpIf{Cond: bpf.JumpNotEqual, Val: uint32(port), SkipTrue: toDrop(9)},
		// VXLAN flags: VNI-valid bit set.
		bpf.LoadIndirect{Off: 8, Size: 1},
		bpf.ALUOpConstant{Op: bpf.ALUOpAnd, Val: vxlanFlagVNIValid},
		bpf.JumpIf{Cond: bpf.JumpNotEqual, Val: vxlanFlagVNIValid, SkipTrue: toDrop(12)},
		// VNI (upper 3 bytes of the last VXLAN header word).
		bpf.LoadIndirect{Off: 12, Size: 4},
		bpf.ALUOpConstant{Op: bpf.ALUOpShiftRight, Val: 8},
		bpf.JumpIf{Cond: bpf.JumpNotEqual, Val: vni, SkipTrue: toDrop(15)},
	}

	if exclude != nil {
		insns = append(insns,
			// Outer source IP: different from the learned one -> accept (skip to accept).
			bpf.LoadAbsolute{Off: 12, Size: 4},
			bpf.JumpIf{Cond: bpf.JumpNotEqual, Val: binary.BigEndian.Uint32(exclude.IP.To4()), SkipTrue: 2},
			// Outer UDP source port: same as the learned one -> drop.
			bpf.LoadIndirect{Off: 0, Size: 2},
			bpf.JumpIf{Cond: bpf.JumpEqual, Val: uint32(exclude.Port), SkipTrue: 1},
		)
	}

	insns = append(insns,
		bpf.RetConstant{Val: learnSnapLen},
		bpf.RetConstant{Val: 0},
	)

	raw, err := bpf.Assemble(insns)
	if err != nil {
		return nil, fmt.Errorf("cannot assemble BPF filter: %w", err)
	}
	filters := make([]unix.SockFilter, len(raw))
	for i := range raw {
		filters[i] = unix.SockFilter{Code: raw[i].Op, Jt: raw[i].Jt, Jf: raw[i].Jf, K: raw[i].K}
	}
	return &unix.SockFprog{
		Len:    uint16(len(filters)), //nolint:gosec // filter length is constant and small
		Filter: &filters[0],
	}, nil
}

// parseOuterSource extracts the outer source address and UDP source port from a
// packet already validated by the BPF filter.
func parseOuterSource(pkt []byte) (ip net.IP, port uint16, ok bool) {
	const ipv4MinLen = 20
	if len(pkt) < ipv4MinLen {
		return nil, 0, false
	}
	ihl := int(pkt[0]&0x0f) * 4
	if ihl < ipv4MinLen || len(pkt) < ihl+2 {
		return nil, 0, false
	}
	srcIP := make(net.IP, net.IPv4len)
	copy(srcIP, pkt[12:16])
	srcPort := binary.BigEndian.Uint16(pkt[ihl : ihl+2])
	if srcPort == 0 {
		return nil, 0, false
	}
	return srcIP, srcPort, true
}

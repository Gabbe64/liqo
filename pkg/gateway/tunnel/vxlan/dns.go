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
	"net"
	"time"

	"k8s.io/klog/v2"
)

// IsDNSRoutineRequired tells whether the remote endpoint address needs periodic
// DNS re-resolution (i.e., it is a DNS name, such as the hostname of a cloud
// LoadBalancer, rather than a literal IP).
func IsDNSRoutineRequired(opts *Options) bool {
	return opts.RemoteAddress != "" && net.ParseIP(opts.RemoteAddress) == nil
}

// RunDNSRoutine periodically re-resolves the remote endpoint address and updates
// the FDB default destination when the resolved IP changes. It blocks until the
// context is canceled.
func RunDNSRoutine(ctx context.Context, opts *Options, linkIndex int) error {
	klog.Infof("DNS routine started: resolving %q every %s", opts.RemoteAddress, opts.DNSCheckInterval)

	var current net.IP
	if ip, err := ResolveRemoteIP(ctx, opts.RemoteAddress); err == nil {
		current = ip
	}

	ticker := time.NewTicker(opts.DNSCheckInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}

		ip, err := ResolveRemoteIP(ctx, opts.RemoteAddress)
		if err != nil {
			klog.Warningf("Cannot re-resolve remote endpoint %q: %v", opts.RemoteAddress, err)
			continue
		}
		if ip.Equal(current) {
			continue
		}

		if err := EnsureFdbEntry(linkIndex, ip, uint16(opts.RemotePort)); err != nil { //nolint:gosec // port fits in uint16
			klog.Errorf("Cannot update FDB default destination to %s:%d: %v", ip, opts.RemotePort, err)
			continue
		}
		klog.Infof("Remote endpoint %q moved: %s -> %s", opts.RemoteAddress, current, ip)
		current = ip
	}
}

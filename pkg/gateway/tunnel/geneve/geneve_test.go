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
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/vishvananda/netlink"

	"github.com/liqotech/liqo/pkg/gateway"
)

func TestGeneve(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Geneve tunnel test suite")
}

// clientOptions returns a valid set of client-mode options, which each spec then
// perturbs in exactly one way.
func clientOptions() *Options {
	return &Options{
		GwOptions:     &gateway.Options{Mode: gateway.ModeClient},
		MTU:           1340,
		VNI:           DefaultVNI,
		Port:          31840,
		RemoteAddress: "10.0.0.1",
		RemotePort:    31840,
		L3:            true,
	}
}

var _ = Describe("ValidateOptions", func() {
	Context("in client mode", func() {
		It("accepts a consistent configuration", func() {
			Expect(ValidateOptions(clientOptions())).To(Succeed())
		})

		It("rejects a remote port differing from the local one", func() {
			// A Geneve device transmits on the port it listens on, so a mismatch
			// black-holes the return path instead of failing visibly.
			opts := clientOptions()
			opts.RemotePort = 51840
			err := ValidateOptions(opts)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("must equal the local tunnel port"))
		})

		It("rejects a missing remote address", func() {
			opts := clientOptions()
			opts.RemoteAddress = ""
			Expect(ValidateOptions(opts)).NotTo(Succeed())
		})

		It("rejects an out-of-range remote port without claiming the flag is missing", func() {
			opts := clientOptions()
			opts.RemotePort = 70000
			err := ValidateOptions(opts)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("must be a valid port"))
		})
	})

	Context("in server mode", func() {
		It("accepts the absence of a peer, which arrives via PeerEndpoint", func() {
			opts := clientOptions()
			opts.GwOptions = &gateway.Options{Mode: gateway.ModeServer}
			opts.RemoteAddress, opts.RemotePort = "", 0
			Expect(ValidateOptions(opts)).To(Succeed())
		})
	})

	DescribeTable("rejects out-of-range values",
		func(mutate func(*Options)) {
			opts := clientOptions()
			mutate(opts)
			Expect(ValidateOptions(opts)).NotTo(Succeed())
		},
		Entry("a VNI wider than 24 bits", func(o *Options) { o.VNI = 1 << 24 }),
		Entry("a negative VNI", func(o *Options) { o.VNI = -1 }),
		Entry("a zero port", func(o *Options) { o.Port = 0 }),
		Entry("a port above the 16-bit range", func(o *Options) { o.Port = 65536 }),
		Entry("a non-positive MTU", func(o *Options) { o.MTU = 0 }),
	)
})

var _ = Describe("geneveParamsMatch", func() {
	var opts *Options

	BeforeEach(func() { opts = clientOptions() })

	// Only the parameters the kernel refuses to change on a live device are
	// compared; everything else is reconciled in place.
	link := func(o *Options) *netlink.Geneve {
		return &netlink.Geneve{
			ID:                uint32(o.VNI),
			Dport:             uint16(o.Port),
			InnerProtoInherit: o.L3,
		}
	}

	It("matches an identical device", func() {
		Expect(geneveParamsMatch(link(opts), opts)).To(BeTrue())
	})

	It("does not match on a different VNI", func() {
		gn := link(opts)
		gn.ID = 200
		Expect(geneveParamsMatch(gn, opts)).To(BeFalse())
	})

	It("does not match on a different port", func() {
		gn := link(opts)
		gn.Dport = 6081
		Expect(geneveParamsMatch(gn, opts)).To(BeFalse())
	})

	It("does not match on a different inner protocol mode", func() {
		gn := link(opts)
		gn.InnerProtoInherit = false
		Expect(geneveParamsMatch(gn, opts)).To(BeFalse())
	})

	It("ignores the MTU, which is adjusted in place", func() {
		gn := link(opts)
		gn.MTU = 9000
		Expect(geneveParamsMatch(gn, opts)).To(BeTrue())
	})

	It("ignores Df, which the netlink library never parses back", func() {
		// Guards the reason ensureGeneveDevice has to re-assert it on reuse:
		// a device whose DF bit is unset still compares as a match.
		gn := link(opts)
		gn.Df = netlink.GENEVE_DF_UNSET
		Expect(geneveParamsMatch(gn, opts)).To(BeTrue())
	})
})

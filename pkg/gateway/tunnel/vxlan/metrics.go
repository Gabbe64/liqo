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
	"github.com/prometheus/client_golang/prometheus"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
)

var (
	endpointUpdates = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "liqo_gateway_vxlan_endpoint_updates_total",
		Help: "Number of times the learned peer endpoint has been updated.",
	})
	endpointLastUpdate = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "liqo_gateway_vxlan_endpoint_last_update_timestamp_seconds",
		Help: "Timestamp of the last learned peer endpoint update.",
	})
)

func init() {
	metrics.Registry.MustRegister(endpointUpdates, endpointLastUpdate)
}

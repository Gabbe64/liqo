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

package shadowendpointslicectrl

import (
	"context"
	"errors"
	"fmt"
	"maps"

	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"

	offloadingv1beta1 "github.com/liqotech/liqo/apis/offloading/v1beta1"
	"github.com/liqotech/liqo/pkg/consts"
	"github.com/liqotech/liqo/pkg/utils/directconnection"
	"github.com/liqotech/liqo/pkg/virtualKubelet/forge"
)

// This file groups everything the controller does for the direct-connections feature: endpointslices of
// Services annotated for direct connections come in pairs (a direct slice carrying the
// provider-to-provider addresses and an -indirect companion carrying the hub-and-spoke ones), and
// only one member of the pair serves traffic at any time, selected through the endpoints' Ready conditions.

// EventReasonDirectConnectionNotPeered is used when a Service requests direct connections towards
// providers that were never network-peered.
const EventReasonDirectConnectionNotPeered = "DirectConnectionNotPeered"

// directPathState describes the usability of the provider-to-provider direct path the endpoints
// of a slice pair depend on.
type directPathState int

const (
	// directPathUnused: the slice takes no part in direct connections (no direct-connections
	// data and not an indirect companion).
	directPathUnused directPathState = iota
	// directPathActive: a Connection towards every direct cluster exists and is Connected.
	directPathActive
	// directPathDown: Connections exist towards all the direct clusters, but at least one is not
	// (yet) Connected. Transient: it recovers on its own and the Connection watch retriggers the
	// reconcile when it does.
	directPathDown
	// directPathNotPeered: no Connection exists at all towards some of the direct clusters — the
	// providers were never network-peered (or have been un-peered since). A misconfiguration
	// that requires operator action.
	directPathNotPeered
	// directPathDenied: this provider denies direct connections altogether (controller flag), so
	// the path is never used regardless of any peering. Their health is not even checked.
	directPathDenied
)

// directPath is the outcome of resolveDirectPath: the role of the slice in a direct/indirect
// pair and the usability of the direct path its endpoints depend on.
type directPath struct {
	state      directPathState
	isIndirect bool

	// data holds the direct-connections annotation content: for each cluster reachable through a
	// direct connection, the endpoint addresses that belong to it.
	data directconnection.ClusterAddresses

	// notPeered carries the clusters missing a Connection when state is directPathNotPeered.
	notPeered *directconnection.NotPeeredError
}

// isDirect reports whether the slice is the direct member of a pair: it carries
// direct-connections data and is not the indirect companion.
func (dp *directPath) isDirect() bool {
	return !dp.isIndirect && len(dp.data.Clusters) > 0
}

// resolveDirectPath classifies the shadow slice with respect to the direct-connections feature
// and, for the slices taking part in it, determines the usability of the direct path by checking
// the health of the Connections towards the involved clusters.
func (r *Reconciler) resolveDirectPath(ctx context.Context,
	shadowEps *offloadingv1beta1.ShadowEndpointSlice) (directPath, error) {
	dp := directPath{isIndirect: shadowEps.Labels[forge.IndirectEndpointSliceLabelKey] == "true"}

	if val, ok := shadowEps.Annotations[consts.DirectConnectionDataAnnotationKey]; ok {
		if err := dp.data.FromJSON([]byte(val)); err != nil {
			return dp, fmt.Errorf("failed to unmarshal direct connection data: %w", err)
		}
	}

	if !dp.isIndirect && len(dp.data.Clusters) == 0 {
		dp.state = directPathUnused
		return dp, nil
	}

	if r.DenyDirectConnections {
		dp.state = directPathDenied
		return dp, nil
	}

	var notPeered *directconnection.NotPeeredError
	switch err := directconnection.CheckConnections(ctx, r.Client, dp.data.ClusterIDs()); {
	case err == nil:
		dp.state = directPathActive
	case errors.As(err, &notPeered):
		dp.state = directPathNotPeered
		dp.notPeered = notPeered
	case errors.Is(err, directconnection.ErrConnectionsDown):
		dp.state = directPathDown
	default:
		return dp, fmt.Errorf("failed to check direct connections status: %w", err)
	}

	return dp, nil
}

// rejectNotPeered handles the never-peered misconfiguration for a direct slice, which cannot be
// materialized correctly without the peering (there is no Configuration to translate the direct
// addresses with, and no amount of waiting will create one). It surfaces the problem with a
// Warning event, deletes the EndpointSlice previously materialized from this shadow if any.
// The indirect companion is unaffected (it reconciles independently and its endpoints become ready since the direct path is not active),
// so the Service keeps working through the consumer path in the meantime.
func (r *Reconciler) rejectNotPeered(ctx context.Context, shadowEps *offloadingv1beta1.ShadowEndpointSlice,
	notPeered *directconnection.NotPeeredError) error {
	eventMsg := fmt.Sprintf("no direct network peering to clusters %v", notPeered.Clusters)
	r.Recorder.Event(r.eventTargetFor(ctx, shadowEps), corev1.EventTypeWarning, EventReasonDirectConnectionNotPeered, eventMsg)

	staleEps := discoveryv1.EndpointSlice{ObjectMeta: metav1.ObjectMeta{Name: shadowEps.Name, Namespace: shadowEps.Namespace}}
	if err := r.Delete(ctx, &staleEps); err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("failed to delete endpointslice for not-peered shadowendpointslice %q: %w", klog.KObj(shadowEps), err)
	}

	return fmt.Errorf("failed creating the endpointslice from shadowendpointslice %q: %w", klog.KObj(shadowEps), notPeered)
}

// eventTargetFor returns the object to record direct-connections events on: 
// the reflected Service the slice belongs to, so that the event is propagated back also to the
// consumer cluster; 
// or the ShadowEndpointSlice itself when the Service cannot be resolved.
func (r *Reconciler) eventTargetFor(ctx context.Context, shadowEps *offloadingv1beta1.ShadowEndpointSlice) client.Object {
	svcName := shadowEps.Labels[discoveryv1.LabelServiceName]
	if svcName == "" {
		return shadowEps
	}

	var svc corev1.Service
	if err := r.Get(ctx, types.NamespacedName{Name: svcName, Namespace: shadowEps.Namespace}, &svc); err != nil {
		return shadowEps
	}
	return &svc
}

// computeEndpointsReady returns the Ready condition to apply to the endpoints of the slice:
// besides the foreign cluster being healthy, at most one member of a direct/indirect pair is
// ready at any time, according to the usability of the direct path.
func computeEndpointsReady(dp *directPath, networkReady, apiServerReady bool) bool {
	// Endpoints are ready only if both the tunnel endpoint and the API server of the foreign
	// cluster (the consumer the slice was reflected from) are ready.
	ready := networkReady && apiServerReady

	switch {
	case dp.isDirect():
		// The direct slice serves traffic only while the direct path is fully usable (the
		// never-peered case does not reach this point: the reconcile stopped earlier and no
		// slice is materialized at all).
		return ready && dp.state == directPathActive

	case dp.isIndirect && len(dp.data.Clusters) == 0:
		// Companion without direct-connections data: none of the endpoints of this slice runs on
		// a directly-connected provider, so the direct member of the pair is a plain slice
		// carrying the exact same endpoints, ready under the ordinary conditions. Keep the
		// companion not-ready to avoid duplicating them.
		return false

	case dp.isIndirect:
		// The indirect companion serves traffic whenever the direct path is not usable (down,
		// not peered, or denied), falling back to the hub-and-spoke path through the consumer.
		return ready && dp.state != directPathActive

	default:
		return ready
	}
}

// applyReadiness applies the computed Ready condition to the endpoints.
//
// Note: an endpoint is updated only if its Ready condition is True or nil, i.e. if the foreign
// cluster sets the endpoint condition Ready to False (the backing pod is failing its readiness
// probe at the origin), the local condition stays False regardless of the computed value: a
// failover must never resurrect an endpoint that is unhealthy at the origin.
func applyReadiness(endpoints []discoveryv1.Endpoint, ready bool) {
	for i := range endpoints {
		endpoint := &endpoints[i]
		if endpoint.Conditions.Ready == nil || *endpoint.Conditions.Ready {
			endpoint.Conditions.Ready = &ready
		}
	}
}

// removeDirectConnectionAnnotation returns a copy of annotations without direct-connection data.
func removeDirectConnectionAnnotation(annotations map[string]string) map[string]string {
	if annotations == nil {
		return nil
	}
	filtered := maps.Clone(annotations)
	delete(filtered, consts.DirectConnectionDataAnnotationKey)
	return filtered
}

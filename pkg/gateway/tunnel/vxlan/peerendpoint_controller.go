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

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	"k8s.io/klog/v2"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	networkingv1beta1 "github.com/liqotech/liqo/apis/networking/v1beta1"
	"github.com/liqotech/liqo/pkg/consts"
)

// PeerEndpointReconciler keeps the FDB default destination aligned with the
// endpoint advertised by the peer through a PeerEndpoint resource.
//
// It is the static-mode counterpart of the endpoint learner: instead of
// observing the outer source of received traffic, the peer endpoint is
// configured out of band. Because it is a watcher rather than a startup flag,
// the endpoint may arrive (or change) at any time without restarting the pod,
// which decouples the tunnel from gateway startup ordering.
type PeerEndpointReconciler struct {
	client.Client
	Scheme         *runtime.Scheme
	EventsRecorder record.EventRecorder
	Options        *Options

	linkIndex int
}

// NewPeerEndpointReconciler returns a new PeerEndpointReconciler.
func NewPeerEndpointReconciler(cl client.Client, s *runtime.Scheme, er record.EventRecorder,
	opts *Options, linkIndex int) *PeerEndpointReconciler {
	return &PeerEndpointReconciler{
		Client:         cl,
		Scheme:         s,
		EventsRecorder: er,
		Options:        opts,
		linkIndex:      linkIndex,
	}
}

// +kubebuilder:rbac:groups=networking.liqo.io,resources=peerendpoints,verbs=get;list;watch

// Reconcile applies the peer endpoint to the tunnel device.
func (r *PeerEndpointReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	peerEndpoint := &networkingv1beta1.PeerEndpoint{}
	if err := r.Get(ctx, req.NamespacedName, peerEndpoint); err != nil {
		if apierrors.IsNotFound(err) {
			klog.V(6).Infof("There is no peerEndpoint %s", req.String())
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, fmt.Errorf("unable to get the peerEndpoint %q: %w", req.NamespacedName, err)
	}

	endpoint := peerEndpoint.Spec.Endpoint
	if len(endpoint.Addresses) == 0 {
		klog.Warningf("PeerEndpoint %s has no addresses yet", req.String())
		return ctrl.Result{}, nil
	}
	if endpoint.Port <= 0 || endpoint.Port > 65535 {
		return ctrl.Result{}, fmt.Errorf("peerEndpoint %q has an invalid port %d", req.NamespacedName, endpoint.Port)
	}

	// Prefer a literal IP; fall back to resolving a hostname (cloud LoadBalancers
	// commonly advertise one).
	remoteIP, err := ResolveRemoteIP(ctx, endpoint.Addresses[0])
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("cannot resolve peer endpoint address: %w", err)
	}

	port := uint16(endpoint.Port) //nolint:gosec // validated above
	if err := EnsureFdbEntry(r.linkIndex, remoteIP, port); err != nil {
		return ctrl.Result{}, fmt.Errorf("cannot set FDB default destination to %s:%d: %w", remoteIP, port, err)
	}

	klog.Infof("Peer endpoint set to %s:%d (from PeerEndpoint %s)", remoteIP, port, req.String())
	endpointUpdates.Inc()
	endpointLastUpdate.SetToCurrentTime()

	return ctrl.Result{}, EnsureConnection(ctx, r.Client, r.Scheme, r.Options)
}

// SetupWithManager registers the PeerEndpointReconciler to the manager.
func (r *PeerEndpointReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).Named(consts.CtrlPeerEndpoint).
		For(&networkingv1beta1.PeerEndpoint{}, r.Predicates()).
		Complete(r)
}

// Predicates filters the PeerEndpoint resources belonging to this peering.
func (r *PeerEndpointReconciler) Predicates() builder.Predicates {
	return builder.WithPredicates(
		predicate.NewPredicateFuncs(func(object client.Object) bool {
			id, ok := object.GetLabels()[string(consts.RemoteClusterID)]
			if !ok {
				return false
			}
			return id == r.Options.GwOptions.RemoteClusterID
		}))
}

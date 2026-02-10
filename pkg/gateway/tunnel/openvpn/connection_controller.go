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

package openvpn

import (
	"context"
	"fmt"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	"k8s.io/klog/v2"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	networkingv1beta1 "github.com/liqotech/liqo/apis/networking/v1beta1"
	"github.com/liqotech/liqo/pkg/consts"
	"github.com/liqotech/liqo/pkg/gateway"
)

// ConnectionReconciler creates the Connection resource once the OpenVPN gateway is ready.
type ConnectionReconciler struct {
	Client         client.Client
	Scheme         *runtime.Scheme
	EventsRecorder record.EventRecorder
	Options        *Options
}

// NewConnectionReconciler returns a new ConnectionReconciler.
func NewConnectionReconciler(cl client.Client, s *runtime.Scheme, er record.EventRecorder, options *Options) *ConnectionReconciler {
	return &ConnectionReconciler{
		Client:         cl,
		Scheme:         s,
		EventsRecorder: er,
		Options:        options,
	}
}

// Reconcile creates the Connection CR when the OpenVPN gateway has an internal endpoint.
func (r *ConnectionReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	switch r.Options.GwOptions.Mode {
	case gateway.ModeClient:
		gw := &networkingv1beta1.OvpnGatewayClient{}
		if err := r.Client.Get(ctx, req.NamespacedName, gw); err != nil {
			if apierrors.IsNotFound(err) {
				klog.Infof("There is no ovpn gateway client %s", req.String())
				return ctrl.Result{}, nil
			}
			return ctrl.Result{}, fmt.Errorf("unable to get the ovpn gateway client %q: %w", req.NamespacedName, err)
		}
		/*
		// Requeue until the internal endpoint is ready
		if gw.Status.InternalEndpoint == nil {
			klog.Infof("The ovpn gateway client %s has no internal endpoint yet, requeuing", req.String())
			return ctrl.Result{Requeue: true}, nil
		}
			*/
	case gateway.ModeServer:
		gw := &networkingv1beta1.OvpnGatewayServer{}
		if err := r.Client.Get(ctx, req.NamespacedName, gw); err != nil {
			if apierrors.IsNotFound(err) {
				klog.Infof("There is no ovpn gateway server %s", req.String())
				return ctrl.Result{}, nil
			}
			return ctrl.Result{}, fmt.Errorf("unable to get the ovpn gateway server %q: %w", req.NamespacedName, err)
		}
		/*
		// Requeue until the internal endpoint is ready
		if gw.Status.InternalEndpoint == nil {
			klog.Infof("The ovpn gateway server %s has no internal endpoint yet, requeuing", req.String())
			return ctrl.Result{Requeue: true}, nil
		}
			*/
	default:
		return ctrl.Result{}, fmt.Errorf("invalid gateway mode %q", r.Options.GwOptions.Mode)
	}
	klog.Infof("Creating the connection for the ovpn gateway %q, no internalEndpoint check", req.NamespacedName)
	return ctrl.Result{}, EnsureConnection(ctx, r.Client, r.Scheme, r.Options)
}

// SetupWithManager registers the ConnectionReconciler to the manager.
func (r *ConnectionReconciler) SetupWithManager(mgr ctrl.Manager) error {
	builder := ctrl.NewControllerManagedBy(mgr).Named("ovpn_connection").WithEventFilter(r.Predicates())

	switch r.Options.GwOptions.Mode {
	case gateway.ModeClient:
		builder = builder.For(&networkingv1beta1.OvpnGatewayClient{})
	case gateway.ModeServer:
		builder = builder.For(&networkingv1beta1.OvpnGatewayServer{})
	default:
		return fmt.Errorf("invalid gateway mode %q", r.Options.GwOptions.Mode)
	}

	return builder.Complete(r)
}

// Predicates returns the predicates required for the Connection controller.
func (r *ConnectionReconciler) Predicates() predicate.Predicate {
	return predicate.Funcs{
		CreateFunc:  func(e event.CreateEvent) bool { return r.hasRemoteClusterID(e.Object) },
		UpdateFunc:  func(e event.UpdateEvent) bool { return r.hasRemoteClusterID(e.ObjectNew) },
		DeleteFunc:  func(e event.DeleteEvent) bool { return r.hasRemoteClusterID(e.Object) },
		GenericFunc: func(e event.GenericEvent) bool { return r.hasRemoteClusterID(e.Object) },
	}
}

func (r *ConnectionReconciler) hasRemoteClusterID(obj client.Object) bool {
	id, ok := obj.GetLabels()[string(consts.RemoteClusterID)]
	if !ok {
		return false
	}
	return id == r.Options.GwOptions.RemoteClusterID
}

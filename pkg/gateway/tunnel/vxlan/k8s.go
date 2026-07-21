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

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"

	networkingv1beta1 "github.com/liqotech/liqo/apis/networking/v1beta1"
	"github.com/liqotech/liqo/pkg/consts"
	"github.com/liqotech/liqo/pkg/gateway"
	"github.com/liqotech/liqo/pkg/gateway/forge"
	"github.com/liqotech/liqo/pkg/utils/resource"
)

// EnsureConnection creates or updates the Connection resource, which the
// gateway connection checker then keeps updated with the tunnel status.
// Unlike WireGuard, no key exchange gates the tunnel: the Connection is
// created as soon as the tunnel device is configured.
func EnsureConnection(ctx context.Context, cl client.Client, scheme *runtime.Scheme, opts *Options) error {
	conn := &networkingv1beta1.Connection{ObjectMeta: metav1.ObjectMeta{
		Name: forge.GatewayResourceName(opts.GwOptions.Name), Namespace: opts.GwOptions.Namespace,
		Labels: map[string]string{
			string(consts.RemoteClusterID): opts.GwOptions.RemoteClusterID,
		},
	}}

	_, err := resource.CreateOrUpdate(ctx, cl, conn, func() error {
		if err := gateway.SetOwnerReferenceWithMode(opts.GwOptions, conn, scheme); err != nil {
			return err
		}
		conn.Spec.GatewayRef.APIVersion = networkingv1beta1.GroupVersion.String()
		conn.Spec.GatewayRef.Name = opts.GwOptions.Name
		conn.Spec.GatewayRef.Namespace = opts.GwOptions.Namespace
		conn.Spec.GatewayRef.UID = types.UID(opts.GwOptions.GatewayUID)
		switch opts.GwOptions.Mode {
		case gateway.ModeServer:
			conn.Spec.Type = networkingv1beta1.ConnectionTypeServer
			conn.Spec.GatewayRef.Kind = networkingv1beta1.VxlanGatewayServerKind
		case gateway.ModeClient:
			conn.Spec.Type = networkingv1beta1.ConnectionTypeClient
			conn.Spec.GatewayRef.Kind = networkingv1beta1.VxlanGatewayClientKind
		}
		return nil
	})
	if err != nil {
		return err
	}

	klog.Infof("Connection %q enforced", conn.Name)

	conn.Status.Value = networkingv1beta1.Connecting
	return cl.Status().Update(ctx, conn)
}

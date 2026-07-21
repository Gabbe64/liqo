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

	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/klog/v2"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	networkingv1beta1 "github.com/liqotech/liqo/apis/networking/v1beta1"
	"github.com/liqotech/liqo/pkg/consts"
	"github.com/liqotech/liqo/pkg/gateway"
)

// podEnquerer maps a Pod event to a reconcile request for the VXLAN gateway that owns it.
func podEnquerer(_ context.Context, obj client.Object) []ctrl.Request {
	pod, ok := obj.(*corev1.Pod)
	if !ok {
		return nil
	}

	if pod.Labels == nil {
		return nil
	}
	gwName, ok := pod.Labels[consts.GatewayNameLabel]
	if !ok {
		return nil
	}
	gwNs, ok := pod.Labels[consts.GatewayNamespaceLabel]
	if !ok {
		return nil
	}

	return []ctrl.Request{
		{
			NamespacedName: client.ObjectKey{
				Namespace: gwNs,
				Name:      gwName,
			},
		},
	}
}

// clusterRoleBindingEnquerer maps a ClusterRoleBinding event to a reconcile request
// for the VXLAN gateway that owns it.
func clusterRoleBindingEnquerer(_ context.Context, obj client.Object) []ctrl.Request {
	crb, ok := obj.(*rbacv1.ClusterRoleBinding)
	if !ok {
		return nil
	}

	if crb.Labels == nil {
		return nil
	}
	gwName, ok := crb.Labels[consts.GatewayNameLabel]
	if !ok {
		return nil
	}
	gwNs, ok := crb.Labels[consts.GatewayNamespaceLabel]
	if !ok {
		return nil
	}

	return []ctrl.Request{
		{
			NamespacedName: client.ObjectKey{
				Namespace: gwNs,
				Name:      gwName,
			},
		},
	}
}

// forgeInternalEndpointFromPods lists the active gateway pods in the given namespace and
// returns the internal endpoint from the single active gateway pod.
func forgeInternalEndpointFromPods(ctx context.Context, cl client.Client, namespace string) (*networkingv1beta1.InternalGatewayEndpoint, error) {
	podsSelector := client.MatchingLabelsSelector{
		Selector: labels.SelectorFromSet(gateway.ForgeActiveGatewayPodLabels()),
	}

	var podList corev1.PodList
	if err := cl.List(ctx, &podList, client.InNamespace(namespace), podsSelector); err != nil {
		klog.Errorf("Unable to list active gateway pods in namespace %q: %v", namespace, err)
		return nil, err
	}

	if len(podList.Items) != 1 {
		return nil, fmt.Errorf("expected exactly 1 active gateway pod in namespace %q, found %d",
			namespace, len(podList.Items))
	}

	pod := &podList.Items[0]
	if pod.Status.PodIP == "" {
		return nil, fmt.Errorf("active gateway pod %q/%q has no IP yet", pod.Namespace, pod.Name)
	}

	return &networkingv1beta1.InternalGatewayEndpoint{
		IP:   ptr.To(networkingv1beta1.IP(pod.Status.PodIP)),
		Node: &pod.Spec.NodeName,
	}, nil
}

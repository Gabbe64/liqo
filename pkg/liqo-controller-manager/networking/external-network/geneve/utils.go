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
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/klog/v2"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	networkingv1beta1 "github.com/liqotech/liqo/apis/networking/v1beta1"
	"github.com/liqotech/liqo/pkg/consts"
	"github.com/liqotech/liqo/pkg/gateway"
	"github.com/liqotech/liqo/pkg/utils"
)

// skipReflectionValue is the value of the annotation marking a resource as not
// to be reflected to remote clusters.
const skipReflectionValue = "true"

// podEnquerer maps a Pod event to a reconcile request for the Geneve gateway that owns it.
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
// for the Geneve gateway that owns it.
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

// forgeEndpointStatus builds the endpoint at which a gateway is reachable from
// its Service, according to the service type. Shared by the server (always) and
// by the client (only in "static" tunnel mode, where it must be reachable too).
func forgeEndpointStatus(ctx context.Context, cl client.Client, service *corev1.Service,
	namespace string) (*networkingv1beta1.EndpointStatus, error) {
	switch service.Spec.Type {
	case corev1.ServiceTypeClusterIP:
		return forgeEndpointStatusClusterIP(service)
	case corev1.ServiceTypeNodePort:
		return forgeEndpointStatusNodePort(ctx, cl, service, namespace)
	case corev1.ServiceTypeLoadBalancer:
		return forgeEndpointStatusLoadBalancer(service)
	default:
		return nil, fmt.Errorf("service type %q not supported for Geneve gateway Service %s/%s",
			service.Spec.Type, service.Namespace, service.Name)
	}
}

func forgeEndpointStatusClusterIP(service *corev1.Service) (*networkingv1beta1.EndpointStatus, error) {
	if len(service.Spec.Ports) == 0 {
		return nil, fmt.Errorf("service %s/%s has no ports", service.Namespace, service.Name)
	}

	return &networkingv1beta1.EndpointStatus{
		Protocol:  &service.Spec.Ports[0].Protocol,
		Port:      service.Spec.Ports[0].Port,
		Addresses: service.Spec.ClusterIPs,
	}, nil
}

// forgeEndpointStatusNodePort advertises the address of the node hosting the
// active gateway pod, together with the allocated node port.
func forgeEndpointStatusNodePort(ctx context.Context, cl client.Client, service *corev1.Service,
	namespace string) (*networkingv1beta1.EndpointStatus, error) {
	if len(service.Spec.Ports) == 0 {
		return nil, fmt.Errorf("service %s/%s has no ports", service.Namespace, service.Name)
	}

	podsSelector := client.MatchingLabelsSelector{Selector: labels.SelectorFromSet(gateway.ForgeActiveGatewayPodLabels())}
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

	node := &corev1.Node{}
	if err := cl.Get(ctx, types.NamespacedName{Name: pod.Spec.NodeName}, node); err != nil && !apierrors.IsNotFound(err) {
		klog.Errorf("Unable to get node %q: %v", pod.Spec.NodeName, err)
		return nil, err
	}

	addresses := make([]string, 1)
	if utils.IsNodeReady(node) {
		var err error
		if addresses[0], err = utils.GetAddress(node); err != nil {
			klog.Errorf("Unable to get address of node %q: %v", pod.Spec.NodeName, err)
			return nil, err
		}
	}

	return &networkingv1beta1.EndpointStatus{
		Protocol:  &service.Spec.Ports[0].Protocol,
		Port:      service.Spec.Ports[0].NodePort,
		Addresses: addresses,
	}, nil
}

func forgeEndpointStatusLoadBalancer(service *corev1.Service) (*networkingv1beta1.EndpointStatus, error) {
	if len(service.Spec.Ports) == 0 {
		return nil, fmt.Errorf("service %s/%s has no ports", service.Namespace, service.Name)
	}

	var addresses []string
	for i := range service.Status.LoadBalancer.Ingress {
		// Prefer the IP over the hostname when both are set: the tunnel runtime
		// resolves hostnames, but a literal IP avoids the DNS dependency.
		if ip := service.Status.LoadBalancer.Ingress[i].IP; ip != "" {
			addresses = append(addresses, ip)
		}
		if hostName := service.Status.LoadBalancer.Ingress[i].Hostname; hostName != "" {
			addresses = append(addresses, hostName)
		}
	}

	return &networkingv1beta1.EndpointStatus{
		Protocol:  &service.Spec.Ports[0].Protocol,
		Port:      service.Spec.Ports[0].Port,
		Addresses: addresses,
	}, nil
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

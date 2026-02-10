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

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"k8s.io/klog/v2"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"

	networkingv1beta1 "github.com/liqotech/liqo/apis/networking/v1beta1"
	"github.com/liqotech/liqo/pkg/consts"
	"github.com/liqotech/liqo/pkg/gateway"
	"github.com/liqotech/liqo/pkg/gateway/forge"
	enutils "github.com/liqotech/liqo/pkg/liqo-controller-manager/networking/external-network/utils"
	"github.com/liqotech/liqo/pkg/utils"
	mapsutil "github.com/liqotech/liqo/pkg/utils/maps"
	"github.com/liqotech/liqo/pkg/utils/resource"
)

// OvpnGatewayServerReconciler manage OvpnGatewayServer lifecycle.
type OvpnGatewayServerReconciler struct {
	client.Client
	Scheme          *runtime.Scheme
	clusterRoleName string

	eventRecorder record.EventRecorder
}

// NewOvpnGatewayServerReconciler returns a new OvpnGatewayServerReconciler.
func NewOvpnGatewayServerReconciler(cl client.Client, s *runtime.Scheme,
	recorder record.EventRecorder,
	clusterRoleName string) *OvpnGatewayServerReconciler {
	return &OvpnGatewayServerReconciler{
		Client:          cl,
		Scheme:          s,
		clusterRoleName: clusterRoleName,

		eventRecorder: recorder,
	}
}

// +kubebuilder:rbac:groups=networking.liqo.io,resources=ovpngatewayservers,verbs=get;list;watch;delete;create;update;patch
// +kubebuilder:rbac:groups=networking.liqo.io,resources=ovpngatewayservers/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=networking.liqo.io,resources=ovpngatewayservers/finalizers,verbs=update
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;delete;create;update;patch
// +kubebuilder:rbac:groups=core,resources=pods,verbs=get;list;watch
// +kubebuilder:rbac:groups=core,resources=nodes,verbs=get;list;watch
// +kubebuilder:rbac:groups=core,resources=nodes/finalizers,verbs=update
// +kubebuilder:rbac:groups=core,resources=services,verbs=get;list;watch;delete;create;update;patch
// +kubebuilder:rbac:groups=core,resources=secrets,verbs=get;list;create;delete;update
// +kubebuilder:rbac:groups=monitoring.coreos.com,resources=servicemonitors,verbs=get;list;watch;delete;create;update;patch
// +kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=clusterrolebindings,verbs=get;list;watch;delete;create;update;patch
// +kubectl:rbac:groups=core,resources=serviceaccounts,verbs=get;list;watch;delete;create;update;patch

// Reconcile manage OvpnGatewayServer lifecycle.
func (r *OvpnGatewayServerReconciler) Reconcile(ctx context.Context, req ctrl.Request) (res ctrl.Result, err error) {
	ovpnServer := &networkingv1beta1.OvpnGatewayServer{}
	if err = r.Get(ctx, req.NamespacedName, ovpnServer); err != nil {
		if apierrors.IsNotFound(err) {
			klog.V(4).Infof("OpenVPN gateway server %q not found", req.NamespacedName)
			return ctrl.Result{}, nil
		}
		klog.Errorf("Unable to get the OpenVPN gateway server %q: %v", req.NamespacedName, err)
		return ctrl.Result{}, err
	}

	if remoteClusterID := ovpnServer.GetLabels()[consts.RemoteClusterID]; remoteClusterID != "" {
		klog.V(4).Infof("Reconciling OpenVPN gateway server %q (remoteClusterID=%q)", req.NamespacedName, remoteClusterID)
	} else {
		klog.V(4).Infof("Reconciling OpenVPN gateway server %q (remoteClusterID missing)", req.NamespacedName)
	}

	if !ovpnServer.DeletionTimestamp.IsZero() {
		if controllerutil.ContainsFinalizer(ovpnServer, consts.ClusterRoleBindingFinalizer) {
			if err = enutils.DeleteClusterRoleBinding(ctx, r.Client, ovpnServer); err != nil {
				return ctrl.Result{}, err
			}

			controllerutil.RemoveFinalizer(ovpnServer, consts.ClusterRoleBindingFinalizer)
			if err = r.Update(ctx, ovpnServer); err != nil {
				klog.Errorf("Unable to remove finalizer %q from OpenVPN gateway server %q: %v",
					consts.ClusterRoleBindingFinalizer, req.NamespacedName, err)
				return ctrl.Result{}, err
			}
		}

		// Resource is deleting and child resources are deleted as well by garbage collector. Nothing to do.
		return ctrl.Result{}, nil
	}

	originalOvpnServer := ovpnServer.DeepCopy()

	// Ensure ServiceAccount and ClusterRoleBinding (create or update)
	if err = enutils.EnsureServiceAccountAndClusterRoleBinding(ctx, r.Client, r.Scheme, &ovpnServer.Spec.Deployment, ovpnServer,
		r.clusterRoleName); err != nil {
		return ctrl.Result{}, err
	}

	// update if the ovpnServer has been updated
	if !equality.Semantic.DeepEqual(originalOvpnServer, ovpnServer) {
		if err := r.Update(ctx, ovpnServer); err != nil {
			return ctrl.Result{}, err
		}

		// we return here to avoid conflicts
		return ctrl.Result{}, nil
	}

	deployNsName := types.NamespacedName{Namespace: ovpnServer.Namespace, Name: forge.GatewayResourceName(ovpnServer.Name)}
	svcNsName := types.NamespacedName{Namespace: ovpnServer.Namespace, Name: forge.GatewayResourceName(ovpnServer.Name)}
	klog.V(4).Infof("OpenVPN gateway server %q targets: deployment=%s service=%s", req.NamespacedName, deployNsName, svcNsName)

	var deploy *appsv1.Deployment
	var d appsv1.Deployment
	err = r.Get(ctx, deployNsName, &d)
	switch {
	case apierrors.IsNotFound(err):
		deploy = nil
		klog.V(4).Infof("Deployment %q not found for OpenVPN gateway server %q", deployNsName, req.NamespacedName)
	case err != nil:
		klog.Errorf("Unable to get the deployment %q: %v", deployNsName, err)
		return ctrl.Result{}, err
	default:
		deploy = &d
		klog.V(4).Infof("Deployment %q found for OpenVPN gateway server %q", deployNsName, req.NamespacedName)
	}

	// Handle status
	defer func() {
		newErr := r.Status().Update(ctx, ovpnServer)
		if newErr != nil {
			if err != nil {
				klog.Errorf("Error reconciling the OpenVPN gateway server %q: %s", req.NamespacedName, err)
			}
			klog.Errorf("Unable to update the OpenVPN gateway server status %q: %s", req.NamespacedName, newErr)
			err = newErr
			return
		}

		r.eventRecorder.Event(ovpnServer, corev1.EventTypeNormal, "Reconciled", "OpenVPN gateway server reconciled")
	}()

	// Ensure service (create or update)
	_, err = r.ensureService(ctx, ovpnServer, svcNsName)
	if err != nil {
		return ctrl.Result{}, err
	}
	r.eventRecorder.Event(ovpnServer, corev1.EventTypeNormal, "ServiceEnforced", "Enforced service")
	klog.V(4).Infof("Service %q enforced for OpenVPN gateway server %q", svcNsName, req.NamespacedName)

	if err := r.handleEndpointStatus(ctx, ovpnServer, svcNsName, deploy); err != nil {
		return ctrl.Result{}, err
	}
	if ovpnServer.Status.Endpoint == nil {
		klog.V(4).Infof("OpenVPN gateway server %q endpoint not set yet", req.NamespacedName)
	} else {
		klog.V(4).Infof("OpenVPN gateway server %q endpoint set: port=%d addresses=%v",
			req.NamespacedName, ovpnServer.Status.Endpoint.Port, ovpnServer.Status.Endpoint.Addresses)
	}

	if err := r.handleInternalEndpointStatus(ctx, ovpnServer, svcNsName, deploy); err != nil {
		klog.Errorf("Error while handling internal endpoint status: %v", err)
		r.eventRecorder.Event(ovpnServer, corev1.EventTypeWarning, "InternalEndpointStatusFailed",
			fmt.Sprintf("Failed to handle internal endpoint status: %s", err))
		return ctrl.Result{}, err
	}
	if ovpnServer.Status.InternalEndpoint == nil {
		klog.V(4).Infof("OpenVPN gateway server %q internal endpoint not set yet", req.NamespacedName)
	} else {
		klog.V(4).Infof("OpenVPN gateway server %q internal endpoint set: ip=%s node=%s",
			req.NamespacedName, ptr.Deref(ovpnServer.Status.InternalEndpoint.IP, ""), ptr.Deref(ovpnServer.Status.InternalEndpoint.Node, ""))
	}

	// OpenVPN secret handling (skeleton).
	if ovpnServer.Spec.SecretRef.Name == "" {
		if err = ensureOpenVPNSecret(ctx, r.Client, ovpnServer); err != nil {
			r.eventRecorder.Event(ovpnServer, corev1.EventTypeWarning, "SecretEnforcedFailed", "Failed to enforce OpenVPN secret")
			return ctrl.Result{}, err
		}
		r.eventRecorder.Event(ovpnServer, corev1.EventTypeNormal, "SecretEnforced", "Enforced OpenVPN secret (skeleton)")
	} else {
		if err = checkExistingOpenVPNSecret(ctx, r.Client, ovpnServer.Spec.SecretRef.Name, ovpnServer.Namespace, ovpnServer.GetObjectMeta()); err != nil {
			r.eventRecorder.Event(ovpnServer, corev1.EventTypeWarning, "SecretCheckFailed", fmt.Sprintf("Failed to check OpenVPN secret: %s", err))
			return ctrl.Result{}, err
		}
		r.eventRecorder.Event(ovpnServer, corev1.EventTypeNormal, "SecretChecked", "Checked OpenVPN secret (skeleton)")
	}

	if err := r.handleSecretRefStatus(ctx, ovpnServer); err != nil {
		klog.Errorf("Error while handling secret ref status: %v", err)
		r.eventRecorder.Event(ovpnServer, corev1.EventTypeWarning, "SecretRefStatusFailed",
			fmt.Sprintf("Failed to handle secret ref status: %s", err))
		return ctrl.Result{}, err
	}

	// Ensure deployment (create or update)
	_, err = r.ensureDeployment(ctx, ovpnServer, deployNsName)
	if err != nil {
		return ctrl.Result{}, err
	}
	r.eventRecorder.Event(ovpnServer, corev1.EventTypeNormal, "DeploymentEnforced", "Enforced deployment")
	klog.V(4).Infof("Deployment %q enforced for OpenVPN gateway server %q", deployNsName, req.NamespacedName)

	// Ensure Metrics (if set)
	metrics := withGatewayMetricsSelector(ovpnServer.Spec.Metrics, ovpnServer)
	err = enutils.EnsureMetrics(ctx,
		r.Client, r.Scheme,
		metrics, ovpnServer)
	if err != nil {
		return ctrl.Result{}, err
	}
	r.eventRecorder.Event(ovpnServer, corev1.EventTypeNormal, "MetricsEnforced", "Enforced metrics")

	return ctrl.Result{}, nil
}

// SetupWithManager register the OvpnGatewayServerReconciler to the manager.
func (r *OvpnGatewayServerReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).Named("ovpngatewayserver").
		For(&networkingv1beta1.OvpnGatewayServer{}).
		Owns(&appsv1.Deployment{}).
		Owns(&corev1.Service{}).
		Owns(&corev1.ServiceAccount{}).
		Watches(&corev1.Pod{}, handler.EnqueueRequestsFromMapFunc(podEnquerer)).
		Watches(&rbacv1.ClusterRoleBinding{},
			handler.EnqueueRequestsFromMapFunc(clusterRoleBindingEnquerer)).
		Complete(r)
}

func (r *OvpnGatewayServerReconciler) ensureDeployment(ctx context.Context, ovpnServer *networkingv1beta1.OvpnGatewayServer,
	depNsName types.NamespacedName) (*appsv1.Deployment, error) {
	dep := appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{
		Name:      depNsName.Name,
		Namespace: depNsName.Namespace,
	}}

	op, err := resource.CreateOrUpdate(ctx, r.Client, &dep, func() error {
		return r.mutateFnOvpnServerDeployment(&dep, ovpnServer)
	})
	if err != nil {
		klog.Errorf("error while creating/updating deployment %q (operation: %s): %v", depNsName, op, err)
		return nil, err
	}

	klog.Infof("Deployment %q correctly enforced (operation: %s)", depNsName, op)
	return &dep, nil
}

func (r *OvpnGatewayServerReconciler) ensureService(ctx context.Context, ovpnServer *networkingv1beta1.OvpnGatewayServer,
	svcNsName types.NamespacedName) (*corev1.Service, error) {
	svc := corev1.Service{ObjectMeta: metav1.ObjectMeta{
		Name:      svcNsName.Name,
		Namespace: svcNsName.Namespace,
	}}

	op, err := resource.CreateOrUpdate(ctx, r.Client, &svc, func() error {
		return r.mutateFnOvpnServerService(&svc, ovpnServer)
	})
	if err != nil {
		klog.Errorf("error while creating/updating service %q (operation: %s): %v", svcNsName, op, err)
		return nil, err
	}

	klog.Infof("Service %q correctly enforced (operation: %s)", svcNsName, op)
	return &svc, nil
}

func (r *OvpnGatewayServerReconciler) mutateFnOvpnServerDeployment(deployment *appsv1.Deployment, ovpnServer *networkingv1beta1.OvpnGatewayServer) error {
	// Forge metadata
	mapsutil.SmartMergeLabels(deployment, ovpnServer.Spec.Deployment.Metadata.GetLabels())
	mapsutil.SmartMergeAnnotations(deployment, ovpnServer.Spec.Deployment.Metadata.GetAnnotations())

	// Forge spec
	deployment.Spec = ovpnServer.Spec.Deployment.Spec

	// TODO: handle OpenVPN secret injection once defined.

	// Set OpenVPN server as owner of the deployment
	return controllerutil.SetControllerReference(ovpnServer, deployment, r.Scheme)
}

func (r *OvpnGatewayServerReconciler) mutateFnOvpnServerService(service *corev1.Service, ovpnServer *networkingv1beta1.OvpnGatewayServer) error {
	// Forge metadata
	mapsutil.SmartMergeLabels(service, ovpnServer.Spec.Service.Metadata.GetLabels())
	mapsutil.SmartMergeAnnotations(service, ovpnServer.Spec.Service.Metadata.GetAnnotations())

	// Forge spec
	serviceClassName := service.Spec.LoadBalancerClass
	service.Spec = ovpnServer.Spec.Service.Spec
	if ovpnServer.Spec.Service.Spec.LoadBalancerClass == nil {
		service.Spec.LoadBalancerClass = serviceClassName
	}
	//service.Spec.Selector = gatewayServiceSelector(ovpnServer, service.Spec.Selector)

	// Set OpenVPN server as owner of the service
	return controllerutil.SetControllerReference(ovpnServer, service, r.Scheme)
}

func (r *OvpnGatewayServerReconciler) handleEndpointStatus(ctx context.Context, ovpnServer *networkingv1beta1.OvpnGatewayServer,
	svcNsName types.NamespacedName, dep *appsv1.Deployment) error {
	if dep == nil {
		ovpnServer.Status.Endpoint = nil
		return nil
	}

	// Handle OpenVPN server Service
	var service corev1.Service
	err := r.Get(ctx, svcNsName, &service)
	if err != nil {
		klog.Error(err) // raise an error also if service NotFound
		return err
	}

	// Put service endpoint in OpenVPN server status
	var endpointStatus *networkingv1beta1.EndpointStatus
	switch service.Spec.Type {
	case corev1.ServiceTypeClusterIP:
		endpointStatus, err = r.forgeEndpointStatusClusterIP(&service)
	case corev1.ServiceTypeNodePort:
		endpointStatus, _, err = r.forgeEndpointStatusNodePort(ctx, &service, dep)
	case corev1.ServiceTypeLoadBalancer:
		endpointStatus, err = r.forgeEndpointStatusLoadBalancer(&service)
	default:
		err = fmt.Errorf("service type %q not supported for OpenVPN server Service %q", service.Spec.Type, svcNsName)
		klog.Error(err)
		ovpnServer.Status.Endpoint = nil // we empty the endpoint status to avoid misaligned spec and status
	}

	if err != nil {
		return err
	}

	ovpnServer.Status.Endpoint = endpointStatus

	return nil
}

func (r *OvpnGatewayServerReconciler) forgeEndpointStatusClusterIP(service *corev1.Service) (*networkingv1beta1.EndpointStatus, error) {
	if len(service.Spec.Ports) == 0 {
		err := fmt.Errorf("service %s/%s has no ports", service.Namespace, service.Name)
		klog.Error(err)
		return nil, err
	}

	port := service.Spec.Ports[0].Port
	protocol := &service.Spec.Ports[0].Protocol
	addresses := service.Spec.ClusterIPs

	return &networkingv1beta1.EndpointStatus{
		Protocol:  protocol,
		Port:      port,
		Addresses: addresses,
	}, nil
}

func (r *OvpnGatewayServerReconciler) forgeEndpointStatusNodePort(ctx context.Context, service *corev1.Service,
	dep *appsv1.Deployment) (*networkingv1beta1.EndpointStatus, *networkingv1beta1.InternalGatewayEndpoint, error) {
	if len(service.Spec.Ports) == 0 {
		err := fmt.Errorf("service %s/%s has no ports", service.Namespace, service.Name)
		klog.Error(err)
		return nil, nil, err
	}

	port := service.Spec.Ports[0].NodePort
	protocol := &service.Spec.Ports[0].Protocol

	podsSelector := client.MatchingLabelsSelector{Selector: labels.SelectorFromSet(gateway.ForgeActiveGatewayPodLabels())}
	var podList corev1.PodList
	if err := r.List(ctx, &podList, client.InNamespace(dep.Namespace), podsSelector); err != nil {
		klog.Errorf("Unable to list pods of deployment %s/%s: %v", dep.Namespace, dep.Name, err)
		return nil, nil, err
	}

	if len(podList.Items) != 1 {
		err := fmt.Errorf("wrong number of pods for deployment %s/%s: %d (must be 1)", dep.Namespace, dep.Name, len(podList.Items))
		klog.Error(err)
		return nil, nil, err
	}

	pod := &podList.Items[0]

	node := &corev1.Node{}
	err := r.Get(ctx, types.NamespacedName{Name: pod.Spec.NodeName}, node)
	if err != nil && !apierrors.IsNotFound(err) {
		klog.Errorf("Unable to get node %q: %v", pod.Spec.NodeName, err)
		return nil, nil, err
	}

	addresses := make([]string, 1)
	if utils.IsNodeReady(node) {
		if addresses[0], err = utils.GetAddress(node); err != nil {
			klog.Errorf("Unable to get address of node %q: %v", pod.Spec.NodeName, err)
			return nil, nil, err
		}
	}

	internalAddress := pod.Status.PodIP
	if internalAddress == "" {
		err := fmt.Errorf("pod %s/%s has no IP", pod.Namespace, pod.Name)
		klog.Error(err)
		return nil, nil, err
	}

	return &networkingv1beta1.EndpointStatus{
			Protocol:  protocol,
			Port:      port,
			Addresses: addresses,
		}, &networkingv1beta1.InternalGatewayEndpoint{
			IP:   ptr.To(networkingv1beta1.IP(internalAddress)),
			Node: &pod.Spec.NodeName,
		}, nil
}

func (r *OvpnGatewayServerReconciler) forgeEndpointStatusLoadBalancer(service *corev1.Service) (*networkingv1beta1.EndpointStatus, error) {
	if len(service.Spec.Ports) == 0 {
		err := fmt.Errorf("service %s/%s has no ports", service.Namespace, service.Name)
		klog.Error(err)
		return nil, err
	}

	port := service.Spec.Ports[0].Port
	protocol := &service.Spec.Ports[0].Protocol

	var addresses []string
	for i := range service.Status.LoadBalancer.Ingress {
		if hostName := service.Status.LoadBalancer.Ingress[i].Hostname; hostName != "" {
			addresses = append(addresses, hostName)
		}
		if ip := service.Status.LoadBalancer.Ingress[i].IP; ip != "" {
			addresses = append(addresses, ip)
		}
	}

	return &networkingv1beta1.EndpointStatus{
		Protocol:  protocol,
		Port:      port,
		Addresses: addresses,
	}, nil
}

func (r *OvpnGatewayServerReconciler) handleSecretRefStatus(ctx context.Context, ovpnServer *networkingv1beta1.OvpnGatewayServer) error {
	if ovpnServer.Spec.SecretRef.Name == "" {
		ovpnServer.Status.SecretRef = nil
		return nil
	}

	secret := &corev1.Secret{}
	err := r.Get(ctx, types.NamespacedName{Name: ovpnServer.Spec.SecretRef.Name, Namespace: ovpnServer.Namespace}, secret)
	switch {
	case apierrors.IsNotFound(err):
		ovpnServer.Status.SecretRef = nil
		return nil
	case err != nil:
		return err
	default:
		ovpnServer.Status.SecretRef = &corev1.ObjectReference{
			Name:      secret.Name,
			Namespace: secret.Namespace,
		}
		return nil
	}
}

func (r *OvpnGatewayServerReconciler) handleInternalEndpointStatus(ctx context.Context, ovpnServer *networkingv1beta1.OvpnGatewayServer,
	svcNsName types.NamespacedName, dep *appsv1.Deployment) error {
	if dep == nil {
		ovpnServer.Status.InternalEndpoint = nil
		return nil
	}

	var service corev1.Service
	err := r.Get(ctx, svcNsName, &service)
	if err != nil {
		klog.Error(err) // raise an error also if service NotFound
		return err
	}

	_, ige, err := r.forgeEndpointStatusNodePort(ctx, &service, dep)
	if err != nil {
		return err
	}

	ovpnServer.Status.InternalEndpoint = ige
	return nil
}

func withGatewayMetricsSelector(metrics *networkingv1beta1.Metrics, owner metav1.Object) *networkingv1beta1.Metrics {
	if metrics == nil || metrics.Service == nil {
		return metrics
	}

	metricsCopy := *metrics
	serviceCopy := *metrics.Service
	//serviceCopy.Spec.Selector = gatewayServiceSelector(owner, serviceCopy.Spec.Selector)
	metricsCopy.Service = &serviceCopy

	return &metricsCopy
}

/*
func gatewayServiceSelector(owner metav1.Object, existing map[string]string) map[string]string {
	required := map[string]string{
		consts.GatewayNameLabel:      owner.GetName(),
		consts.GatewayNamespaceLabel: owner.GetNamespace(),
	}
	required = labels.Merge(required, gateway.ForgeActiveGatewayPodLabels())
	return labels.Merge(existing, required)
}
*/
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
			NamespacedName: types.NamespacedName{
				Namespace: gwNs,
				Name:      gwName,
			},
		},
	}
}

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
			NamespacedName: types.NamespacedName{
				Namespace: gwNs,
				Name:      gwName,
			},
		},
	}
}

// ensureOpenVPNSecret is a placeholder for OpenVPN secret creation.
func ensureOpenVPNSecret(_ context.Context, _ client.Client, _ metav1.Object) error {
	// TODO: implement OpenVPN secret creation.
	return nil
}

// checkExistingOpenVPNSecret is a placeholder for OpenVPN secret validation.
func checkExistingOpenVPNSecret(_ context.Context, _ client.Client, _ string, _ string, _ metav1.Object) error {
	// TODO: implement OpenVPN secret validation.
	return nil
}

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
	mapsutil "github.com/liqotech/liqo/pkg/utils/maps"
	"github.com/liqotech/liqo/pkg/utils/resource"
)

// OvpnGatewayClientReconciler manages OvpnGatewayClient lifecycle.
type OvpnGatewayClientReconciler struct {
	client.Client
	Scheme          *runtime.Scheme
	clusterRoleName string

	eventRecorder record.EventRecorder
}

// NewOvpnGatewayClientReconciler returns a new OvpnGatewayClientReconciler.
func NewOvpnGatewayClientReconciler(cl client.Client, s *runtime.Scheme,
	recorder record.EventRecorder,
	clusterRoleName string) *OvpnGatewayClientReconciler {
	return &OvpnGatewayClientReconciler{
		Client:          cl,
		Scheme:          s,
		clusterRoleName: clusterRoleName,

		eventRecorder: recorder,
	}
}

// cluster-role
// +kubebuilder:rbac:groups=networking.liqo.io,resources=ovpngatewayclients,verbs=get;list;watch;delete;create;update;patch
// +kubebuilder:rbac:groups=networking.liqo.io,resources=ovpngatewayclients/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=networking.liqo.io,resources=ovpngatewayclients/finalizers,verbs=update
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;delete;create;update;patch
// +kubebuilder:rbac:groups=core,resources=secrets,verbs=get;list;create;delete;update
// +kubebuilder:rbac:groups=monitoring.coreos.com,resources=servicemonitors,verbs=get;list;watch;delete;create;update;patch
// +kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=clusterrolebindings,verbs=get;list;watch;delete;create;update;patch
// +kubectl:rbac:groups=core,resources=serviceaccounts,verbs=get;list;watch;delete;create;update;patch

// Reconcile manages OvpnGatewayClient lifecycle.
func (r *OvpnGatewayClientReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	ovpnClient := &networkingv1beta1.OvpnGatewayClient{}
	if err := r.Get(ctx, req.NamespacedName, ovpnClient); err != nil {
		if apierrors.IsNotFound(err) {
			klog.V(4).Infof("OpenVPN gateway client %q not found", req.NamespacedName)
			return ctrl.Result{}, nil
		}
		klog.Errorf("Unable to get the OpenVPN gateway client %q: %v", req.NamespacedName, err)
		return ctrl.Result{}, err
	}

	if remoteClusterID := ovpnClient.GetLabels()[consts.RemoteClusterID]; remoteClusterID != "" {
		klog.V(4).Infof("Reconciling OpenVPN gateway client %q (remoteClusterID=%q)", req.NamespacedName, remoteClusterID)
	} else {
		klog.V(4).Infof("Reconciling OpenVPN gateway client %q (remoteClusterID missing)", req.NamespacedName)
	}

	if !ovpnClient.DeletionTimestamp.IsZero() {
		if controllerutil.ContainsFinalizer(ovpnClient, consts.ClusterRoleBindingFinalizer) {
			if err := enutils.DeleteClusterRoleBinding(ctx, r.Client, ovpnClient); err != nil {
				return ctrl.Result{}, err
			}

			controllerutil.RemoveFinalizer(ovpnClient, consts.ClusterRoleBindingFinalizer)
			if err := r.Update(ctx, ovpnClient); err != nil {
				klog.Errorf("Unable to remove finalizer %q from OpenVPN gateway client %q: %v",
					consts.ClusterRoleBindingFinalizer, req.NamespacedName, err)
				return ctrl.Result{}, err
			}
		}

		// Resource is deleting and child resources are deleted as well by garbage collector. Nothing to do.
		return ctrl.Result{}, nil
	}

	originalOvpnClient := ovpnClient.DeepCopy()

	// Ensure ServiceAccount and ClusterRoleBinding (create or update)
	if err := enutils.EnsureServiceAccountAndClusterRoleBinding(ctx, r.Client, r.Scheme, &ovpnClient.Spec.Deployment, ovpnClient,
		r.clusterRoleName); err != nil {
		return ctrl.Result{}, err
	}

	// update if the ovpnClient has been updated
	if !equality.Semantic.DeepEqual(originalOvpnClient, ovpnClient) {
		if err := r.Update(ctx, ovpnClient); err != nil {
			return ctrl.Result{}, err
		}

		// we return here to avoid conflicts
		return ctrl.Result{}, nil
	}

	deployNsName := types.NamespacedName{Namespace: ovpnClient.Namespace, Name: forge.GatewayResourceName(ovpnClient.Name)}
	klog.V(4).Infof("OpenVPN gateway client %q deployment target: %s", req.NamespacedName, deployNsName)

	var deploy *appsv1.Deployment
	var d appsv1.Deployment
	err := r.Get(ctx, deployNsName, &d)
	switch {
	case apierrors.IsNotFound(err):
		deploy = nil
		klog.V(4).Infof("Deployment %q not found for OpenVPN gateway client %q", deployNsName, req.NamespacedName)
	case err != nil:
		klog.Errorf("error while getting deployment %q: %v", deployNsName, err)
		return ctrl.Result{}, err
	default:
		deploy = &d
		klog.V(4).Infof("Deployment %q found for OpenVPN gateway client %q", deployNsName, req.NamespacedName)
	}

	// Handle status
	defer func() {
		newErr := r.Status().Update(ctx, ovpnClient)
		if newErr != nil {
			if err != nil {
				klog.Errorf("Error reconciling the OpenVPN gateway client %q: %s", req.NamespacedName, err)
			}
			klog.Errorf("Unable to update the OpenVPN gateway client status %q: %s", req.NamespacedName, newErr)
			err = newErr
			return
		}

		r.eventRecorder.Event(ovpnClient, corev1.EventTypeNormal, "Reconciled", "OpenVPN gateway client reconciled")
	}()

	if err := r.handleInternalEndpointStatus(ctx, ovpnClient, deploy); err != nil {
		klog.Errorf("Error while handling internal endpoint status: %v", err)
		r.eventRecorder.Event(ovpnClient, corev1.EventTypeWarning, "InternalEndpointStatusFailed",
			fmt.Sprintf("Failed to handle internal endpoint status: %s", err))
		return ctrl.Result{}, err
	}
	if ovpnClient.Status.InternalEndpoint == nil {
		klog.V(4).Infof("OpenVPN gateway client %q internal endpoint not set yet", req.NamespacedName)
	} else {
		klog.V(4).Infof("OpenVPN gateway client %q internal endpoint set: ip=%s node=%s",
			req.NamespacedName, ptr.Deref(ovpnClient.Status.InternalEndpoint.IP, ""), ptr.Deref(ovpnClient.Status.InternalEndpoint.Node, ""))
	}

	// OpenVPN secret handling (skeleton).
	if ovpnClient.Spec.SecretRef.Name == "" {
		if err := ensureOpenVPNSecret(ctx, r.Client, ovpnClient); err != nil {
			r.eventRecorder.Event(ovpnClient, corev1.EventTypeWarning, "SecretEnforcedFailed", "Failed to enforce OpenVPN secret")
			return ctrl.Result{}, err
		}
		r.eventRecorder.Event(ovpnClient, corev1.EventTypeNormal, "SecretEnforced", "Enforced OpenVPN secret (skeleton)")
	} else {
		if err := checkExistingOpenVPNSecret(ctx, r.Client, ovpnClient.Spec.SecretRef.Name, ovpnClient.Namespace, ovpnClient.GetObjectMeta()); err != nil {
			r.eventRecorder.Event(ovpnClient, corev1.EventTypeWarning, "SecretCheckFailed", fmt.Sprintf("Failed to check OpenVPN secret: %s", err))
			return ctrl.Result{}, err
		}
		r.eventRecorder.Event(ovpnClient, corev1.EventTypeNormal, "SecretChecked", "Checked OpenVPN secret (skeleton)")
	}

	if err := r.handleSecretRefStatus(ctx, ovpnClient); err != nil {
		klog.Errorf("Error while handling secret ref status: %v", err)
		r.eventRecorder.Event(ovpnClient, corev1.EventTypeWarning, "SecretRefStatusFailed",
			fmt.Sprintf("Failed to handle secret ref status: %s", err))
		return ctrl.Result{}, err
	}

	// Ensure deployment (create or update)
	_, err = r.ensureDeployment(ctx, ovpnClient, deployNsName)
	if err != nil {
		return ctrl.Result{}, err
	}
	r.eventRecorder.Event(ovpnClient, corev1.EventTypeNormal, "DeploymentEnforced", "Enforced deployment")
	klog.V(4).Infof("Deployment %q enforced for OpenVPN gateway client %q", deployNsName, req.NamespacedName)

	// Ensure Metrics (if set)
	metrics := withGatewayMetricsSelector(ovpnClient.Spec.Metrics, ovpnClient)
	err = enutils.EnsureMetrics(ctx,
		r.Client, r.Scheme,
		metrics, ovpnClient)
	if err != nil {
		return ctrl.Result{}, err
	}
	r.eventRecorder.Event(ovpnClient, corev1.EventTypeNormal, "MetricsEnforced", "Enforced metrics")

	return ctrl.Result{}, nil
}

// SetupWithManager registers the OvpnGatewayClientReconciler to the manager.
func (r *OvpnGatewayClientReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).Named("ovpngatewayclient").
		For(&networkingv1beta1.OvpnGatewayClient{}).
		Owns(&appsv1.Deployment{}).
		Owns(&corev1.ServiceAccount{}).
		Watches(&corev1.Pod{}, handler.EnqueueRequestsFromMapFunc(podEnquerer)).
		Watches(&rbacv1.ClusterRoleBinding{},
			handler.EnqueueRequestsFromMapFunc(clusterRoleBindingEnquerer)).
		Complete(r)
}

// ensureDeployment ensures the OpenVPN gateway client deployment exists.
func (r *OvpnGatewayClientReconciler) ensureDeployment(ctx context.Context, ovpnClient *networkingv1beta1.OvpnGatewayClient,
	depNsName types.NamespacedName) (*appsv1.Deployment, error) {
	dep := appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{
		Name:      depNsName.Name,
		Namespace: depNsName.Namespace,
	}}

	op, err := resource.CreateOrUpdate(ctx, r.Client, &dep, func() error {
		return r.mutateFnOvpnClientDeployment(&dep, ovpnClient)
	})
	if err != nil {
		klog.Errorf("error while creating/updating deployment %q (operation: %s): %v", depNsName, op, err)
		return nil, err
	}

	klog.Infof("Deployment %q correctly enforced (operation: %s)", depNsName, op)
	return &dep, nil
}

// mutateFnOvpnClientDeployment mutates the deployment according to OvpnGatewayClient spec.
func (r *OvpnGatewayClientReconciler) mutateFnOvpnClientDeployment(deployment *appsv1.Deployment, ovpnClient *networkingv1beta1.OvpnGatewayClient) error {
	// Forge metadata
	mapsutil.SmartMergeLabels(deployment, ovpnClient.Spec.Deployment.Metadata.GetLabels())
	mapsutil.SmartMergeAnnotations(deployment, ovpnClient.Spec.Deployment.Metadata.GetAnnotations())

	// Forge spec
	deployment.Spec = ovpnClient.Spec.Deployment.Spec

	// TODO: handle OpenVPN secret injection once defined.

	// Set OpenVPN client as owner of the deployment
	return controllerutil.SetControllerReference(ovpnClient, deployment, r.Scheme)
}

// handleSecretRefStatus updates the OvpnGatewayClient status with the secret reference.
func (r *OvpnGatewayClientReconciler) handleSecretRefStatus(ctx context.Context, ovpnClient *networkingv1beta1.OvpnGatewayClient) error {
	if ovpnClient.Spec.SecretRef.Name == "" {
		ovpnClient.Status.SecretRef = nil
		return nil
	}

	secret := &corev1.Secret{}
	err := r.Get(ctx, types.NamespacedName{Name: ovpnClient.Spec.SecretRef.Name, Namespace: ovpnClient.Namespace}, secret)
	switch {
	case apierrors.IsNotFound(err):
		ovpnClient.Status.SecretRef = nil
		return nil
	case err != nil:
		return err
	default:
		ovpnClient.Status.SecretRef = &corev1.ObjectReference{
			Name:      secret.Name,
			Namespace: secret.Namespace,
		}
		return nil
	}
}

// handleInternalEndpointStatus updates the OvpnGatewayClient status with internal endpoint information.
func (r *OvpnGatewayClientReconciler) handleInternalEndpointStatus(ctx context.Context,
	ovpnClient *networkingv1beta1.OvpnGatewayClient, dep *appsv1.Deployment) error {
	if dep == nil {
		ovpnClient.Status.InternalEndpoint = nil
		return nil
	}

	podsSelector := client.MatchingLabelsSelector{Selector: labels.SelectorFromSet(gateway.ForgeActiveGatewayPodLabels())}
	var podList corev1.PodList
	if err := r.List(ctx, &podList, client.InNamespace(dep.Namespace), podsSelector); err != nil {
		klog.Errorf("Unable to list pods of deployment %s/%s: %v", dep.Namespace, dep.Name, err)
		return err
	}

	if len(podList.Items) != 1 {
		err := fmt.Errorf("wrong number of pods for deployment %s/%s: %d (must be 1)", dep.Namespace, dep.Name, len(podList.Items))
		klog.Error(err)
		return err
	}

	if podList.Items[0].Status.PodIP == "" {
		err := fmt.Errorf("pod %s/%s has no IP", podList.Items[0].Namespace, podList.Items[0].Name)
		klog.Error(err)
		return err
	}

	ovpnClient.Status.InternalEndpoint = &networkingv1beta1.InternalGatewayEndpoint{
		IP:   ptr.To(networkingv1beta1.IP(podList.Items[0].Status.PodIP)),
		Node: &podList.Items[0].Spec.NodeName,
	}
	return nil
}

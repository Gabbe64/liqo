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

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"k8s.io/klog/v2"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"

	networkingv1beta1 "github.com/liqotech/liqo/apis/networking/v1beta1"
	"github.com/liqotech/liqo/pkg/consts"
	"github.com/liqotech/liqo/pkg/gateway/forge"
	enutils "github.com/liqotech/liqo/pkg/liqo-controller-manager/networking/external-network/utils"
	mapsutil "github.com/liqotech/liqo/pkg/utils/maps"
	"github.com/liqotech/liqo/pkg/utils/resource"
)

// GatewayClientReconciler manages VxlanGatewayClient lifecycle.
//
// The client is a pure initiator: it only runs a deployment. No Service is
// created, since the server learns the client endpoint from the data plane.
type GatewayClientReconciler struct {
	client.Client
	Scheme          *runtime.Scheme
	clusterRoleName string

	eventRecorder record.EventRecorder
}

// NewGatewayClientReconciler returns a new GatewayClientReconciler.
func NewGatewayClientReconciler(cl client.Client, s *runtime.Scheme,
	recorder record.EventRecorder,
	clusterRoleName string) *GatewayClientReconciler {
	return &GatewayClientReconciler{
		Client:          cl,
		Scheme:          s,
		clusterRoleName: clusterRoleName,

		eventRecorder: recorder,
	}
}

// +kubebuilder:rbac:groups=networking.liqo.io,resources=vxlangatewayclients,verbs=get;list;watch;delete;create;update;patch
// +kubebuilder:rbac:groups=networking.liqo.io,resources=vxlangatewayclients/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=networking.liqo.io,resources=vxlangatewayclients/finalizers,verbs=update
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;delete;create;update;patch
// +kubebuilder:rbac:groups=core,resources=pods,verbs=get;list;watch
// +kubebuilder:rbac:groups=core,resources=serviceaccounts,verbs=get;list;watch;delete;create;update;patch
// +kubebuilder:rbac:groups=monitoring.coreos.com,resources=servicemonitors,verbs=get;list;watch;delete;create;update;patch
// +kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=clusterrolebindings,verbs=get;list;watch;delete;create;update;patch

// Reconcile manages VxlanGatewayClient lifecycle.
func (r *GatewayClientReconciler) Reconcile(ctx context.Context, req ctrl.Request) (res ctrl.Result, err error) {
	vxlanClient := &networkingv1beta1.VxlanGatewayClient{}
	if err = r.Get(ctx, req.NamespacedName, vxlanClient); err != nil {
		if apierrors.IsNotFound(err) {
			klog.V(4).Infof("VXLAN gateway client %q not found", req.NamespacedName)
			return ctrl.Result{}, nil
		}
		klog.Errorf("Unable to get the VXLAN gateway client %q: %v", req.NamespacedName, err)
		return ctrl.Result{}, err
	}

	if !vxlanClient.DeletionTimestamp.IsZero() {
		if controllerutil.ContainsFinalizer(vxlanClient, consts.ClusterRoleBindingFinalizer) {
			if err = enutils.DeleteClusterRoleBinding(ctx, r.Client, vxlanClient); err != nil {
				return ctrl.Result{}, err
			}

			controllerutil.RemoveFinalizer(vxlanClient, consts.ClusterRoleBindingFinalizer)
			if err = r.Update(ctx, vxlanClient); err != nil {
				klog.Errorf("Unable to remove finalizer %q from VXLAN gateway client %q: %v",
					consts.ClusterRoleBindingFinalizer, req.NamespacedName, err)
				return ctrl.Result{}, err
			}
		}

		// Resource is deleting and child resources are deleted as well by garbage collector. Nothing to do.
		return ctrl.Result{}, nil
	}

	originalVxlanClient := vxlanClient.DeepCopy()

	// Ensure ServiceAccount and ClusterRoleBinding (create or update).
	if err = enutils.EnsureServiceAccountAndClusterRoleBinding(ctx, r.Client, r.Scheme, &vxlanClient.Spec.Deployment, vxlanClient,
		r.clusterRoleName); err != nil {
		return ctrl.Result{}, err
	}

	// Update if vxlanClient has been updated.
	if !equality.Semantic.DeepEqual(originalVxlanClient, vxlanClient) {
		if err := r.Update(ctx, vxlanClient); err != nil {
			return ctrl.Result{}, err
		}

		// We return here to avoid conflicts.
		return ctrl.Result{}, nil
	}

	deployNsName := types.NamespacedName{Namespace: vxlanClient.Namespace, Name: forge.GatewayResourceName(vxlanClient.Name)}

	// Handle status.
	defer func() {
		newErr := r.Status().Update(ctx, vxlanClient)
		if newErr != nil {
			if err != nil {
				klog.Errorf("Error reconciling the VXLAN gateway client %q: %s", req.NamespacedName, err)
			}
			klog.Errorf("Unable to update the VXLAN gateway client status %q: %s", req.NamespacedName, newErr)
			err = newErr
			return
		}

		r.eventRecorder.Event(vxlanClient, corev1.EventTypeNormal, "Reconciled", "VXLAN gateway client reconciled")
	}()

	// Ensure deployment (create or update).
	_, err = r.ensureDeployment(ctx, vxlanClient, deployNsName)
	if err != nil {
		return ctrl.Result{}, err
	}
	r.eventRecorder.Event(vxlanClient, corev1.EventTypeNormal, "DeploymentEnforced", "Enforced deployment")

	// Handle internal endpoint status (requires running pods).
	r.handleInternalEndpointStatus(ctx, vxlanClient, deployNsName)

	// Ensure metrics (if set).
	err = enutils.EnsureMetrics(ctx,
		r.Client, r.Scheme,
		vxlanClient.Spec.Metrics, vxlanClient)
	if err != nil {
		return ctrl.Result{}, err
	}
	r.eventRecorder.Event(vxlanClient, corev1.EventTypeNormal, "MetricsEnforced", "Enforced metrics")

	return ctrl.Result{}, nil
}

// SetupWithManager registers the GatewayClientReconciler to the manager.
func (r *GatewayClientReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).Named(consts.CtrlVxlanGatewayClient).
		For(&networkingv1beta1.VxlanGatewayClient{}).
		Owns(&appsv1.Deployment{}).
		Owns(&corev1.ServiceAccount{}).
		Watches(&corev1.Pod{}, handler.EnqueueRequestsFromMapFunc(podEnquerer)).
		Watches(&rbacv1.ClusterRoleBinding{},
			handler.EnqueueRequestsFromMapFunc(clusterRoleBindingEnquerer)).
		Complete(r)
}

func (r *GatewayClientReconciler) ensureDeployment(ctx context.Context, vxlanClient *networkingv1beta1.VxlanGatewayClient,
	depNsName types.NamespacedName) (*appsv1.Deployment, error) {
	dep := appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{
		Name:      depNsName.Name,
		Namespace: depNsName.Namespace,
	}}

	op, err := resource.CreateOrUpdate(ctx, r.Client, &dep, func() error {
		return r.mutateFnVxlanClientDeployment(&dep, vxlanClient)
	})
	if err != nil {
		klog.Errorf("error while creating/updating deployment %q (operation: %s): %v", depNsName, op, err)
		return nil, err
	}

	klog.Infof("Deployment %q correctly enforced (operation: %s)", depNsName, op)
	return &dep, nil
}

func (r *GatewayClientReconciler) mutateFnVxlanClientDeployment(deployment *appsv1.Deployment,
	vxlanClient *networkingv1beta1.VxlanGatewayClient) error {
	// Forge metadata.
	mapsutil.SmartMergeLabels(deployment, vxlanClient.Spec.Deployment.Metadata.GetLabels())
	mapsutil.SmartMergeAnnotations(deployment, vxlanClient.Spec.Deployment.Metadata.GetAnnotations())

	// Forge spec.
	deployment.Spec = vxlanClient.Spec.Deployment.Spec

	// Set VXLAN client as owner of the deployment.
	return controllerutil.SetControllerReference(vxlanClient, deployment, r.Scheme)
}

func (r *GatewayClientReconciler) handleInternalEndpointStatus(ctx context.Context,
	vxlanClient *networkingv1beta1.VxlanGatewayClient, deployNsName types.NamespacedName) {
	ige, err := forgeInternalEndpointFromPods(ctx, r.Client, deployNsName.Namespace)
	if err != nil {
		klog.V(4).Infof("VXLAN gateway client %q: internal endpoint not available yet: %v",
			deployNsName, err)
		vxlanClient.Status.InternalEndpoint = nil
		return
	}

	vxlanClient.Status.InternalEndpoint = ige
}

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

// GatewayServerReconciler manages VxlanGatewayServer lifecycle.
type GatewayServerReconciler struct {
	client.Client
	Scheme          *runtime.Scheme
	clusterRoleName string

	eventRecorder record.EventRecorder
}

// NewGatewayServerReconciler returns a new GatewayServerReconciler.
func NewGatewayServerReconciler(cl client.Client, s *runtime.Scheme,
	recorder record.EventRecorder,
	clusterRoleName string) *GatewayServerReconciler {
	return &GatewayServerReconciler{
		Client:          cl,
		Scheme:          s,
		clusterRoleName: clusterRoleName,

		eventRecorder: recorder,
	}
}

// +kubebuilder:rbac:groups=networking.liqo.io,resources=vxlangatewayservers,verbs=get;list;watch;delete;create;update;patch
// +kubebuilder:rbac:groups=networking.liqo.io,resources=vxlangatewayservers/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=networking.liqo.io,resources=vxlangatewayservers/finalizers,verbs=update
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;delete;create;update;patch
// +kubebuilder:rbac:groups=core,resources=pods,verbs=get;list;watch
// +kubebuilder:rbac:groups=core,resources=nodes,verbs=get;list;watch
// +kubebuilder:rbac:groups=core,resources=services,verbs=get;list;watch;delete;create;update;patch
// +kubebuilder:rbac:groups=core,resources=serviceaccounts,verbs=get;list;watch;delete;create;update;patch
// +kubebuilder:rbac:groups=monitoring.coreos.com,resources=servicemonitors,verbs=get;list;watch;delete;create;update;patch
// +kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=clusterrolebindings,verbs=get;list;watch;delete;create;update;patch

// Reconcile manages VxlanGatewayServer lifecycle.
func (r *GatewayServerReconciler) Reconcile(ctx context.Context, req ctrl.Request) (res ctrl.Result, err error) {
	vxlanServer := &networkingv1beta1.VxlanGatewayServer{}
	if err = r.Get(ctx, req.NamespacedName, vxlanServer); err != nil {
		if apierrors.IsNotFound(err) {
			klog.V(4).Infof("VXLAN gateway server %q not found", req.NamespacedName)
			return ctrl.Result{}, nil
		}
		klog.Errorf("Unable to get the VXLAN gateway server %q: %v", req.NamespacedName, err)
		return ctrl.Result{}, err
	}

	if !vxlanServer.DeletionTimestamp.IsZero() {
		if controllerutil.ContainsFinalizer(vxlanServer, consts.ClusterRoleBindingFinalizer) {
			if err = enutils.DeleteClusterRoleBinding(ctx, r.Client, vxlanServer); err != nil {
				return ctrl.Result{}, err
			}

			controllerutil.RemoveFinalizer(vxlanServer, consts.ClusterRoleBindingFinalizer)
			if err = r.Update(ctx, vxlanServer); err != nil {
				klog.Errorf("Unable to remove finalizer %q from VXLAN gateway server %q: %v",
					consts.ClusterRoleBindingFinalizer, req.NamespacedName, err)
				return ctrl.Result{}, err
			}
		}

		// Resource is deleting and child resources are deleted as well by garbage collector. Nothing to do.
		return ctrl.Result{}, nil
	}

	originalVxlanServer := vxlanServer.DeepCopy()

	// Ensure ServiceAccount and ClusterRoleBinding (create or update).
	if err = enutils.EnsureServiceAccountAndClusterRoleBinding(ctx, r.Client, r.Scheme, &vxlanServer.Spec.Deployment, vxlanServer,
		r.clusterRoleName); err != nil {
		return ctrl.Result{}, err
	}

	// Update if vxlanServer has been updated.
	if !equality.Semantic.DeepEqual(originalVxlanServer, vxlanServer) {
		if err := r.Update(ctx, vxlanServer); err != nil {
			return ctrl.Result{}, err
		}

		// We return here to avoid conflicts.
		return ctrl.Result{}, nil
	}

	deployNsName := types.NamespacedName{Namespace: vxlanServer.Namespace, Name: forge.GatewayResourceName(vxlanServer.Name)}
	svcNsName := types.NamespacedName{Namespace: vxlanServer.Namespace, Name: forge.GatewayResourceName(vxlanServer.Name)}

	// Handle status.
	defer func() {
		newErr := r.Status().Update(ctx, vxlanServer)
		if newErr != nil {
			if err != nil {
				klog.Errorf("Error reconciling the VXLAN gateway server %q: %s", req.NamespacedName, err)
			}
			klog.Errorf("Unable to update the VXLAN gateway server status %q: %s", req.NamespacedName, newErr)
			err = newErr
			return
		}

		r.eventRecorder.Event(vxlanServer, corev1.EventTypeNormal, "Reconciled", "VXLAN gateway server reconciled")
	}()

	// Ensure deployment (create or update).
	if _, err = r.ensureDeployment(ctx, vxlanServer, deployNsName); err != nil {
		return ctrl.Result{}, err
	}
	r.eventRecorder.Event(vxlanServer, corev1.EventTypeNormal, "DeploymentEnforced", "Enforced deployment")

	// Ensure service (create or update).
	_, err = r.ensureService(ctx, vxlanServer, svcNsName)
	if err != nil {
		return ctrl.Result{}, err
	}
	r.eventRecorder.Event(vxlanServer, corev1.EventTypeNormal, "ServiceEnforced", "Enforced service")

	// Handle endpoint status from the service.
	if err := r.handleEndpointStatus(ctx, vxlanServer, svcNsName); err != nil {
		klog.Errorf("Error while handling endpoint status: %v", err)
		r.eventRecorder.Event(vxlanServer, corev1.EventTypeWarning, "EndpointStatusFailed",
			fmt.Sprintf("Failed to handle endpoint status: %s", err))
		return ctrl.Result{}, err
	}

	// Handle internal endpoint status (requires running pods).
	r.handleInternalEndpointStatus(ctx, vxlanServer, deployNsName)

	// Ensure metrics (if set).
	err = enutils.EnsureMetrics(ctx,
		r.Client, r.Scheme,
		vxlanServer.Spec.Metrics, vxlanServer)
	if err != nil {
		return ctrl.Result{}, err
	}
	r.eventRecorder.Event(vxlanServer, corev1.EventTypeNormal, "MetricsEnforced", "Enforced metrics")

	return ctrl.Result{}, nil
}

// SetupWithManager registers the GatewayServerReconciler to the manager.
func (r *GatewayServerReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).Named(consts.CtrlVxlanGatewayServer).
		For(&networkingv1beta1.VxlanGatewayServer{}).
		Owns(&appsv1.Deployment{}).
		Owns(&corev1.Service{}).
		Owns(&corev1.ServiceAccount{}).
		Watches(&corev1.Pod{}, handler.EnqueueRequestsFromMapFunc(podEnquerer)).
		Watches(&rbacv1.ClusterRoleBinding{},
			handler.EnqueueRequestsFromMapFunc(clusterRoleBindingEnquerer)).
		Complete(r)
}

func (r *GatewayServerReconciler) ensureDeployment(ctx context.Context, vxlanServer *networkingv1beta1.VxlanGatewayServer,
	depNsName types.NamespacedName) (*appsv1.Deployment, error) {
	dep := appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{
		Name:      depNsName.Name,
		Namespace: depNsName.Namespace,
	}}

	op, err := resource.CreateOrUpdate(ctx, r.Client, &dep, func() error {
		return r.mutateFnVxlanServerDeployment(&dep, vxlanServer)
	})
	if err != nil {
		klog.Errorf("error while creating/updating deployment %q (operation: %s): %v", depNsName, op, err)
		return nil, err
	}

	klog.Infof("Deployment %q correctly enforced (operation: %s)", depNsName, op)
	return &dep, nil
}

func (r *GatewayServerReconciler) ensureService(ctx context.Context, vxlanServer *networkingv1beta1.VxlanGatewayServer,
	svcNsName types.NamespacedName) (*corev1.Service, error) {
	svc := corev1.Service{ObjectMeta: metav1.ObjectMeta{
		Name:      svcNsName.Name,
		Namespace: svcNsName.Namespace,
	}}

	op, err := resource.CreateOrUpdate(ctx, r.Client, &svc, func() error {
		return r.mutateFnVxlanServerService(&svc, vxlanServer)
	})
	if err != nil {
		klog.Errorf("error while creating/updating service %q (operation: %s): %v", svcNsName, op, err)
		return nil, err
	}

	klog.Infof("Service %q correctly enforced (operation: %s)", svcNsName, op)
	return &svc, nil
}

func (r *GatewayServerReconciler) mutateFnVxlanServerDeployment(deployment *appsv1.Deployment,
	vxlanServer *networkingv1beta1.VxlanGatewayServer) error {
	// Forge metadata.
	mapsutil.SmartMergeLabels(deployment, vxlanServer.Spec.Deployment.Metadata.GetLabels())
	mapsutil.SmartMergeAnnotations(deployment, vxlanServer.Spec.Deployment.Metadata.GetAnnotations())

	// Forge spec.
	deployment.Spec = vxlanServer.Spec.Deployment.Spec

	// Set VXLAN server as owner of the deployment.
	return controllerutil.SetControllerReference(vxlanServer, deployment, r.Scheme)
}

func (r *GatewayServerReconciler) mutateFnVxlanServerService(service *corev1.Service,
	vxlanServer *networkingv1beta1.VxlanGatewayServer) error {
	// Forge metadata.
	mapsutil.SmartMergeLabels(service, vxlanServer.Spec.Service.Metadata.GetLabels())
	mapsutil.SmartMergeAnnotations(service, vxlanServer.Spec.Service.Metadata.GetAnnotations())

	// Ensure the service is never reflected to remote clusters.
	if service.Annotations == nil {
		service.Annotations = map[string]string{}
	}
	service.Annotations[consts.SkipReflectionAnnotationKey] = skipReflectionValue

	// Forge spec.
	serviceClassName := service.Spec.LoadBalancerClass
	service.Spec = vxlanServer.Spec.Service.Spec
	if vxlanServer.Spec.Service.Spec.LoadBalancerClass == nil {
		service.Spec.LoadBalancerClass = serviceClassName
	}

	// Set VXLAN server as owner of the service.
	return controllerutil.SetControllerReference(vxlanServer, service, r.Scheme)
}

func (r *GatewayServerReconciler) handleEndpointStatus(ctx context.Context, vxlanServer *networkingv1beta1.VxlanGatewayServer,
	svcNsName types.NamespacedName) error {
	var service corev1.Service
	err := r.Get(ctx, svcNsName, &service)
	if err != nil {
		if apierrors.IsNotFound(err) {
			vxlanServer.Status.Endpoint = nil
			return nil
		}
		klog.Error(err)
		return err
	}

	endpointStatus, err := forgeEndpointStatus(ctx, r.Client, &service, svcNsName.Namespace)
	if err != nil {
		// Empty the endpoint status to avoid a misaligned spec and status.
		vxlanServer.Status.Endpoint = nil
		return err
	}

	vxlanServer.Status.Endpoint = endpointStatus
	return nil
}

func (r *GatewayServerReconciler) handleInternalEndpointStatus(ctx context.Context,
	vxlanServer *networkingv1beta1.VxlanGatewayServer, deployNsName types.NamespacedName) {
	ige, err := forgeInternalEndpointFromPods(ctx, r.Client, deployNsName.Namespace)
	if err != nil {
		klog.V(4).Infof("VXLAN gateway server %q: internal endpoint not available yet: %v",
			deployNsName, err)
		vxlanServer.Status.InternalEndpoint = nil
		return
	}

	vxlanServer.Status.InternalEndpoint = ige
}

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

package fou

import (
	"context"
	"fmt"
	"strings"

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

// FouGatewayServerReconciler manages FouGatewayServer lifecycle.
type FouGatewayServerReconciler struct {
	client.Client
	Scheme          *runtime.Scheme
	clusterRoleName string

	eventRecorder record.EventRecorder
}

// NewFouGatewayServerReconciler returns a new FouGatewayServerReconciler.
func NewFouGatewayServerReconciler(cl client.Client, s *runtime.Scheme,
	recorder record.EventRecorder,
	clusterRoleName string) *FouGatewayServerReconciler {
	return &FouGatewayServerReconciler{
		Client:          cl,
		Scheme:          s,
		clusterRoleName: clusterRoleName,

		eventRecorder: recorder,
	}
}

// +kubebuilder:rbac:groups=networking.liqo.io,resources=fougatewayservers,verbs=get;list;watch;delete;create;update;patch
// +kubebuilder:rbac:groups=networking.liqo.io,resources=fougatewayservers/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=networking.liqo.io,resources=fougatewayservers/finalizers,verbs=update
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;delete;create;update;patch
// +kubebuilder:rbac:groups=core,resources=pods,verbs=get;list;watch
// +kubebuilder:rbac:groups=core,resources=nodes,verbs=get;list;watch
// +kubebuilder:rbac:groups=core,resources=nodes/finalizers,verbs=update
// +kubebuilder:rbac:groups=core,resources=services,verbs=get;list;watch;delete;create;update;patch
// +kubebuilder:rbac:groups=monitoring.coreos.com,resources=servicemonitors,verbs=get;list;watch;delete;create;update;patch
// +kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=clusterrolebindings,verbs=get;list;watch;delete;create;update;patch
// +kubectl:rbac:groups=core,resources=serviceaccounts,verbs=get;list;watch;delete;create;update;patch

// Reconcile manages FouGatewayServer lifecycle.
func (r *FouGatewayServerReconciler) Reconcile(ctx context.Context, req ctrl.Request) (res ctrl.Result, err error) {
	fouServer := &networkingv1beta1.FouGatewayServer{}
	if err = r.Get(ctx, req.NamespacedName, fouServer); err != nil {
		if apierrors.IsNotFound(err) {
			klog.V(4).Infof("FOU gateway server %q not found", req.NamespacedName)
			return ctrl.Result{}, nil
		}
		klog.Errorf("Unable to get the FOU gateway server %q: %v", req.NamespacedName, err)
		return ctrl.Result{}, err
	}

	if !fouServer.DeletionTimestamp.IsZero() {
		if controllerutil.ContainsFinalizer(fouServer, consts.ClusterRoleBindingFinalizer) {
			if err = enutils.DeleteClusterRoleBinding(ctx, r.Client, fouServer); err != nil {
				return ctrl.Result{}, err
			}

			controllerutil.RemoveFinalizer(fouServer, consts.ClusterRoleBindingFinalizer)
			if err = r.Update(ctx, fouServer); err != nil {
				klog.Errorf("Unable to remove finalizer %q from FOU gateway server %q: %v",
					consts.ClusterRoleBindingFinalizer, req.NamespacedName, err)
				return ctrl.Result{}, err
			}
		}

		// Resource is deleting and child resources are deleted as well by garbage collector. Nothing to do.
		return ctrl.Result{}, nil
	}

	originalFouServer := fouServer.DeepCopy()

	// Ensure ServiceAccount and ClusterRoleBinding (create or update).
	if err = enutils.EnsureServiceAccountAndClusterRoleBinding(ctx, r.Client, r.Scheme, &fouServer.Spec.Deployment, fouServer,
		r.clusterRoleName); err != nil {
		return ctrl.Result{}, err
	}

	// Update if fouServer has been updated.
	if !equality.Semantic.DeepEqual(originalFouServer, fouServer) {
		if err := r.Update(ctx, fouServer); err != nil {
			return ctrl.Result{}, err
		}

		// We return here to avoid conflicts.
		return ctrl.Result{}, nil
	}

	deployNsName := types.NamespacedName{Namespace: fouServer.Namespace, Name: forge.GatewayResourceName(fouServer.Name)}
	svcNsName := types.NamespacedName{Namespace: fouServer.Namespace, Name: forge.GatewayResourceName(fouServer.Name)}

	// Handle status.
	defer func() {
		newErr := r.Status().Update(ctx, fouServer)
		if newErr != nil {
			if err != nil {
				klog.Errorf("Error reconciling the FOU gateway server %q: %s", req.NamespacedName, err)
			}
			klog.Errorf("Unable to update the FOU gateway server status %q: %s", req.NamespacedName, newErr)
			err = newErr
			return
		}

		r.eventRecorder.Event(fouServer, corev1.EventTypeNormal, "Reconciled", "FOU gateway server reconciled")
	}()

	// Ensure service first so the LoadBalancer IP is assigned.
	_, err = r.ensureService(ctx, fouServer, svcNsName)
	if err != nil {
		return ctrl.Result{}, err
	}
	r.eventRecorder.Event(fouServer, corev1.EventTypeNormal, "ServiceEnforced", "Enforced service")

	// Handle endpoint status from the service (independent of deployment).
	if err := r.handleEndpointStatus(ctx, fouServer, svcNsName); err != nil {
		return ctrl.Result{}, err
	}

	// Only create/update the deployment when the client endpoint is known.
	// The server-operator renders --remote-port from
	// GatewayServer.Spec.ClientEndpoint into the deployment spec. If the arg is still
	// empty, the client endpoint has not been propagated yet.
	if hasNonEmptyArg(fouServer.Spec.Deployment.Spec, "--remote-port") {
		_, err = r.ensureDeployment(ctx, fouServer, deployNsName)
		if err != nil {
			return ctrl.Result{}, err
		}
		r.eventRecorder.Event(fouServer, corev1.EventTypeNormal, "DeploymentEnforced", "Enforced deployment")

		// Handle internal endpoint status (requires running pods).
		if err := r.handleInternalEndpointStatus(ctx, fouServer, deployNsName); err != nil {
			klog.Errorf("Error while handling internal endpoint status: %v", err)
			r.eventRecorder.Event(fouServer, corev1.EventTypeWarning, "InternalEndpointStatusFailed",
				fmt.Sprintf("Failed to handle internal endpoint status: %s", err))
			return ctrl.Result{}, err
		}
	} else {
		klog.V(4).Infof("FOU gateway server %q: waiting for client endpoint before creating deployment", req.NamespacedName)
		fouServer.Status.InternalEndpoint = nil
	}

	// Ensure metrics (if set).
	err = enutils.EnsureMetrics(ctx,
		r.Client, r.Scheme,
		fouServer.Spec.Metrics, fouServer)
	if err != nil {
		return ctrl.Result{}, err
	}
	r.eventRecorder.Event(fouServer, corev1.EventTypeNormal, "MetricsEnforced", "Enforced metrics")

	return ctrl.Result{}, nil
}

// SetupWithManager registers the FouGatewayServerReconciler to the manager.
func (r *FouGatewayServerReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).Named(consts.CtrlFOUGatewayServer).
		For(&networkingv1beta1.FouGatewayServer{}).
		Owns(&appsv1.Deployment{}).
		Owns(&corev1.Service{}).
		Owns(&corev1.ServiceAccount{}).
		Watches(&corev1.Pod{}, handler.EnqueueRequestsFromMapFunc(podEnquerer)).
		Watches(&rbacv1.ClusterRoleBinding{},
			handler.EnqueueRequestsFromMapFunc(clusterRoleBindingEnquerer)).
		Complete(r)
}

func (r *FouGatewayServerReconciler) ensureDeployment(ctx context.Context, fouServer *networkingv1beta1.FouGatewayServer,
	depNsName types.NamespacedName) (*appsv1.Deployment, error) {
	dep := appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{
		Name:      depNsName.Name,
		Namespace: depNsName.Namespace,
	}}

	op, err := resource.CreateOrUpdate(ctx, r.Client, &dep, func() error {
		return r.mutateFnFouServerDeployment(&dep, fouServer)
	})
	if err != nil {
		klog.Errorf("error while creating/updating deployment %q (operation: %s): %v", depNsName, op, err)
		return nil, err
	}

	klog.Infof("Deployment %q correctly enforced (operation: %s)", depNsName, op)
	return &dep, nil
}

func (r *FouGatewayServerReconciler) ensureService(ctx context.Context, fouServer *networkingv1beta1.FouGatewayServer,
	svcNsName types.NamespacedName) (*corev1.Service, error) {
	svc := corev1.Service{ObjectMeta: metav1.ObjectMeta{
		Name:      svcNsName.Name,
		Namespace: svcNsName.Namespace,
	}}

	op, err := resource.CreateOrUpdate(ctx, r.Client, &svc, func() error {
		return r.mutateFnFouServerService(&svc, fouServer)
	})
	if err != nil {
		klog.Errorf("error while creating/updating service %q (operation: %s): %v", svcNsName, op, err)
		return nil, err
	}

	klog.Infof("Service %q correctly enforced (operation: %s)", svcNsName, op)
	return &svc, nil
}

func (r *FouGatewayServerReconciler) mutateFnFouServerDeployment(deployment *appsv1.Deployment, fouServer *networkingv1beta1.FouGatewayServer) error {
	// Forge metadata.
	mapsutil.SmartMergeLabels(deployment, fouServer.Spec.Deployment.Metadata.GetLabels())
	mapsutil.SmartMergeAnnotations(deployment, fouServer.Spec.Deployment.Metadata.GetAnnotations())

	// Forge spec. The deployment spec already contains --remote-port
	// rendered by the server-operator from GatewayServer.Spec.ClientEndpoint.
	deployment.Spec = fouServer.Spec.Deployment.Spec

	// Set FOU server as owner of the deployment.
	return controllerutil.SetControllerReference(fouServer, deployment, r.Scheme)
}

// hasNonEmptyArg returns true if the deployment spec contains a non-empty value for the given flag
// (e.g. "--endpoint-address=10.0.0.1" → true, "--endpoint-address=" → false).
func hasNonEmptyArg(deploySpec appsv1.DeploymentSpec, flag string) bool {
	prefix := flag + "="
	for i := range deploySpec.Template.Spec.Containers {
		for _, arg := range deploySpec.Template.Spec.Containers[i].Args {
			if strings.HasPrefix(arg, prefix) && len(arg) > len(prefix) {
				return true
			}
		}
	}
	return false
}

func (r *FouGatewayServerReconciler) mutateFnFouServerService(service *corev1.Service, fouServer *networkingv1beta1.FouGatewayServer) error {
	// Forge metadata.
	mapsutil.SmartMergeLabels(service, fouServer.Spec.Service.Metadata.GetLabels())
	mapsutil.SmartMergeAnnotations(service, fouServer.Spec.Service.Metadata.GetAnnotations())

	// Ensure the service is never reflected to remote clusters.
	if service.Annotations == nil {
		service.Annotations = map[string]string{}
	}
	service.Annotations[consts.SkipReflectionAnnotationKey] = "true"

	// Forge spec. (LB only)
	serviceClassName := service.Spec.LoadBalancerClass
	service.Spec = fouServer.Spec.Service.Spec
	if fouServer.Spec.Service.Spec.LoadBalancerClass == nil {
		service.Spec.LoadBalancerClass = serviceClassName
	}

	// Set FOU server as owner of the service.
	return controllerutil.SetControllerReference(fouServer, service, r.Scheme)
}

func (r *FouGatewayServerReconciler) handleEndpointStatus(ctx context.Context, fouServer *networkingv1beta1.FouGatewayServer,
	svcNsName types.NamespacedName) error {
	var service corev1.Service
	err := r.Get(ctx, svcNsName, &service)
	if err != nil {
		if apierrors.IsNotFound(err) {
			fouServer.Status.Endpoint = nil
			return nil
		}
		klog.Error(err)
		return err
	}

	if service.Spec.Type != corev1.ServiceTypeLoadBalancer {
		err = fmt.Errorf("FOU gateway server only supports LoadBalancer service type, got %q for service %q",
			service.Spec.Type, svcNsName)
		klog.Error(err)
		fouServer.Status.Endpoint = nil
		return err
	}

	endpointStatus, err := forgeEndpointStatusLoadBalancer(&service)
	if err != nil {
		return err
	}

	fouServer.Status.Endpoint = endpointStatus
	return nil
}

func (r *FouGatewayServerReconciler) handleInternalEndpointStatus(ctx context.Context, fouServer *networkingv1beta1.FouGatewayServer,
	deployNsName types.NamespacedName) error {
	ige, err := forgeInternalEndpointFromPods(ctx, r.Client, deployNsName.Namespace)
	if err != nil {
		return err
	}

	fouServer.Status.InternalEndpoint = ige
	return nil
}

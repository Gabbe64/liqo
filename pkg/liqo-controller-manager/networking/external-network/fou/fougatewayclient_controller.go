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

// FouGatewayClientReconciler manages FouGatewayClient lifecycle.
type FouGatewayClientReconciler struct {
	client.Client
	Scheme          *runtime.Scheme
	clusterRoleName string

	eventRecorder record.EventRecorder
}

// NewFouGatewayClientReconciler returns a new FouGatewayClientReconciler.
func NewFouGatewayClientReconciler(cl client.Client, s *runtime.Scheme,
	recorder record.EventRecorder,
	clusterRoleName string) *FouGatewayClientReconciler {
	return &FouGatewayClientReconciler{
		Client:          cl,
		Scheme:          s,
		clusterRoleName: clusterRoleName,

		eventRecorder: recorder,
	}
}

// +kubebuilder:rbac:groups=networking.liqo.io,resources=fougatewayclients,verbs=get;list;watch;delete;create;update;patch
// +kubebuilder:rbac:groups=networking.liqo.io,resources=fougatewayclients/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=networking.liqo.io,resources=fougatewayclients/finalizers,verbs=update
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;delete;create;update;patch
// +kubebuilder:rbac:groups=core,resources=services,verbs=get;list;watch;delete;create;update;patch
// +kubebuilder:rbac:groups=monitoring.coreos.com,resources=servicemonitors,verbs=get;list;watch;delete;create;update;patch
// +kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=clusterrolebindings,verbs=get;list;watch;delete;create;update;patch
// +kubectl:rbac:groups=core,resources=serviceaccounts,verbs=get;list;watch;delete;create;update;patch

// Reconcile manages FouGatewayClient lifecycle.
func (r *FouGatewayClientReconciler) Reconcile(ctx context.Context, req ctrl.Request) (res ctrl.Result, err error) {
	fouClient := &networkingv1beta1.FouGatewayClient{}
	if err = r.Get(ctx, req.NamespacedName, fouClient); err != nil {
		if apierrors.IsNotFound(err) {
			klog.V(4).Infof("FOU gateway client %q not found", req.NamespacedName)
			return ctrl.Result{}, nil
		}
		klog.Errorf("Unable to get the FOU gateway client %q: %v", req.NamespacedName, err)
		return ctrl.Result{}, err
	}

	if !fouClient.DeletionTimestamp.IsZero() {
		if controllerutil.ContainsFinalizer(fouClient, consts.ClusterRoleBindingFinalizer) {
			if err = enutils.DeleteClusterRoleBinding(ctx, r.Client, fouClient); err != nil {
				return ctrl.Result{}, err
			}

			controllerutil.RemoveFinalizer(fouClient, consts.ClusterRoleBindingFinalizer)
			if err = r.Update(ctx, fouClient); err != nil {
				klog.Errorf("Unable to remove finalizer %q from FOU gateway client %q: %v",
					consts.ClusterRoleBindingFinalizer, req.NamespacedName, err)
				return ctrl.Result{}, err
			}
		}

		// Resource is deleting and child resources are deleted as well by garbage collector. Nothing to do.
		return ctrl.Result{}, nil
	}

	originalFouClient := fouClient.DeepCopy()

	// Ensure ServiceAccount and ClusterRoleBinding (create or update)
	if err = enutils.EnsureServiceAccountAndClusterRoleBinding(ctx, r.Client, r.Scheme, &fouClient.Spec.Deployment, fouClient,
		r.clusterRoleName); err != nil {
		return ctrl.Result{}, err
	}

	// Update if fouClient has been updated.
	if !equality.Semantic.DeepEqual(originalFouClient, fouClient) {
		if err := r.Update(ctx, fouClient); err != nil {
			return ctrl.Result{}, err
		}

		// We return here to avoid conflicts.
		return ctrl.Result{}, nil
	}

	deployNsName := types.NamespacedName{Namespace: fouClient.Namespace, Name: forge.GatewayResourceName(fouClient.Name)}
	svcNsName := types.NamespacedName{Namespace: fouClient.Namespace, Name: forge.GatewayResourceName(fouClient.Name)}

	// Handle status.
	defer func() {
		newErr := r.Status().Update(ctx, fouClient)
		if newErr != nil {
			if err != nil {
				klog.Errorf("Error reconciling the FOU gateway client %q: %s", req.NamespacedName, err)
			}
			klog.Errorf("Unable to update the FOU gateway client status %q: %s", req.NamespacedName, newErr)
			err = newErr
			return
		}

		r.eventRecorder.Event(fouClient, corev1.EventTypeNormal, "Reconciled", "FOU gateway client reconciled")
	}()

	// Ensure service first so the LoadBalancer IP can be assigned.
	_, err = r.ensureService(ctx, fouClient, svcNsName)
	if err != nil {
		return ctrl.Result{}, err
	}
	r.eventRecorder.Event(fouClient, corev1.EventTypeNormal, "ServiceEnforced", "Enforced service")

	// Handle endpoint status from the service (independent of deployment).
	if err := r.handleEndpointStatus(ctx, fouClient, svcNsName); err != nil {
		klog.Errorf("Error while handling endpoint status: %v", err)
		r.eventRecorder.Event(fouClient, corev1.EventTypeWarning, "EndpointStatusFailed",
			fmt.Sprintf("Failed to handle endpoint status: %s", err))
		return ctrl.Result{}, err
	}

	// Ensure deployment (create or update).
	deploy, err := r.ensureDeployment(ctx, fouClient, deployNsName)
	if err != nil {
		return ctrl.Result{}, err
	}
	r.eventRecorder.Event(fouClient, corev1.EventTypeNormal, "DeploymentEnforced", "Enforced deployment")

	if err := r.handleInternalEndpointStatus(ctx, fouClient, deploy); err != nil {
		klog.Errorf("Error while handling internal endpoint status: %v", err)
		r.eventRecorder.Event(fouClient, corev1.EventTypeWarning, "InternalEndpointStatusFailed",
			fmt.Sprintf("Failed to handle internal endpoint status: %s", err))
		return ctrl.Result{}, err
	}

	// Ensure metrics (if set).
	err = enutils.EnsureMetrics(ctx,
		r.Client, r.Scheme,
		fouClient.Spec.Metrics, fouClient)
	if err != nil {
		return ctrl.Result{}, err
	}
	r.eventRecorder.Event(fouClient, corev1.EventTypeNormal, "MetricsEnforced", "Enforced metrics")

	return ctrl.Result{}, nil
}

// SetupWithManager registers the FouGatewayClientReconciler to the manager.
func (r *FouGatewayClientReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).Named(consts.CtrlFOUGatewayClient).
		For(&networkingv1beta1.FouGatewayClient{}).
		Owns(&appsv1.Deployment{}).
		Owns(&corev1.Service{}).
		Owns(&corev1.ServiceAccount{}).
		Watches(&corev1.Pod{}, handler.EnqueueRequestsFromMapFunc(podEnquerer)).
		Watches(&rbacv1.ClusterRoleBinding{},
			handler.EnqueueRequestsFromMapFunc(clusterRoleBindingEnquerer)).
		Complete(r)
}

func (r *FouGatewayClientReconciler) ensureDeployment(ctx context.Context, fouClient *networkingv1beta1.FouGatewayClient,
	depNsName types.NamespacedName) (*appsv1.Deployment, error) {
	dep := appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{
		Name:      depNsName.Name,
		Namespace: depNsName.Namespace,
	}}

	op, err := resource.CreateOrUpdate(ctx, r.Client, &dep, func() error {
		return r.mutateFnFouClientDeployment(&dep, fouClient)
	})
	if err != nil {
		klog.Errorf("error while creating/updating deployment %q (operation: %s): %v", depNsName, op, err)
		return nil, err
	}

	klog.Infof("Deployment %q correctly enforced (operation: %s)", depNsName, op)
	return &dep, nil
}

func (r *FouGatewayClientReconciler) ensureService(ctx context.Context, fouClient *networkingv1beta1.FouGatewayClient,
	svcNsName types.NamespacedName) (*corev1.Service, error) {
	svc := corev1.Service{ObjectMeta: metav1.ObjectMeta{
		Name:      svcNsName.Name,
		Namespace: svcNsName.Namespace,
	}}

	op, err := resource.CreateOrUpdate(ctx, r.Client, &svc, func() error {
		return r.mutateFnFouClientService(&svc, fouClient)
	})
	if err != nil {
		klog.Errorf("error while creating/updating service %q (operation: %s): %v", svcNsName, op, err)
		return nil, err
	}

	klog.Infof("Service %q correctly enforced (operation: %s)", svcNsName, op)
	return &svc, nil
}

func (r *FouGatewayClientReconciler) mutateFnFouClientDeployment(deployment *appsv1.Deployment, fouClient *networkingv1beta1.FouGatewayClient) error {
	// Forge metadata.
	mapsutil.SmartMergeLabels(deployment, fouClient.Spec.Deployment.Metadata.GetLabels())
	mapsutil.SmartMergeAnnotations(deployment, fouClient.Spec.Deployment.Metadata.GetAnnotations())

	// Forge spec.
	deployment.Spec = fouClient.Spec.Deployment.Spec

	// Set FOU client as owner of the deployment.
	return controllerutil.SetControllerReference(fouClient, deployment, r.Scheme)
}

func (r *FouGatewayClientReconciler) mutateFnFouClientService(service *corev1.Service, fouClient *networkingv1beta1.FouGatewayClient) error {
	// Forge metadata.
	mapsutil.SmartMergeLabels(service, fouClient.Spec.Service.Metadata.GetLabels())
	mapsutil.SmartMergeAnnotations(service, fouClient.Spec.Service.Metadata.GetAnnotations())

	// Ensure the service is never reflected to remote clusters.
	if service.Annotations == nil {
		service.Annotations = map[string]string{}
	}
	service.Annotations[consts.SkipReflectionAnnotationKey] = "true"

	// Forge spec.
	serviceClassName := service.Spec.LoadBalancerClass
	service.Spec = fouClient.Spec.Service.Spec
	if fouClient.Spec.Service.Spec.LoadBalancerClass == nil {
		service.Spec.LoadBalancerClass = serviceClassName
	}

	// Set FOU client as owner of the service.
	return controllerutil.SetControllerReference(fouClient, service, r.Scheme)
}

func (r *FouGatewayClientReconciler) handleEndpointStatus(ctx context.Context, fouClient *networkingv1beta1.FouGatewayClient,
	svcNsName types.NamespacedName) error {
	var service corev1.Service
	err := r.Get(ctx, svcNsName, &service)
	if err != nil {
		if apierrors.IsNotFound(err) {
			fouClient.Status.Endpoint = nil
			return nil
		}
		klog.Error(err)
		return err
	}

	if service.Spec.Type != corev1.ServiceTypeLoadBalancer {
		err = fmt.Errorf("FOU gateway client only supports LoadBalancer service type, got %q for service %q",
			service.Spec.Type, svcNsName)
		klog.Error(err)
		fouClient.Status.Endpoint = nil
		return err
	}

	endpointStatus, err := forgeEndpointStatusLoadBalancer(&service)
	if err != nil {
		// LB IP not yet assigned – not an error, will be populated on next reconcile.
		klog.V(4).Infof("FOU gateway client %q: LB IP not yet assigned: %v", svcNsName, err)
		fouClient.Status.Endpoint = nil
		return nil
	}

	fouClient.Status.Endpoint = endpointStatus
	return nil
}

func (r *FouGatewayClientReconciler) handleInternalEndpointStatus(ctx context.Context,
	fouClient *networkingv1beta1.FouGatewayClient, dep *appsv1.Deployment) error {
	if dep == nil {
		fouClient.Status.InternalEndpoint = nil
		return nil
	}

	ige, err := forgeInternalEndpointFromPods(ctx, r.Client, dep.Namespace)
	if err != nil {
		return err
	}

	fouClient.Status.InternalEndpoint = ige
	return nil
}

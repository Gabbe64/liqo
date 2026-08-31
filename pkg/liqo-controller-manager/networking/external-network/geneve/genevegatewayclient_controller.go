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

// GatewayClientReconciler manages GeneveGatewayClient lifecycle.
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

// +kubebuilder:rbac:groups=networking.liqo.io,resources=genevegatewayclients,verbs=get;list;watch;delete;create;update;patch
// +kubebuilder:rbac:groups=networking.liqo.io,resources=genevegatewayclients/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=networking.liqo.io,resources=genevegatewayclients/finalizers,verbs=update
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;delete;create;update;patch
// +kubebuilder:rbac:groups=core,resources=pods,verbs=get;list;watch
// +kubebuilder:rbac:groups=core,resources=nodes,verbs=get;list;watch
// +kubebuilder:rbac:groups=core,resources=services,verbs=get;list;watch;delete;create;update;patch
// +kubebuilder:rbac:groups=core,resources=serviceaccounts,verbs=get;list;watch;delete;create;update;patch
// +kubebuilder:rbac:groups=monitoring.coreos.com,resources=servicemonitors,verbs=get;list;watch;delete;create;update;patch
// +kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=clusterrolebindings,verbs=get;list;watch;delete;create;update;patch

// Reconcile manages GeneveGatewayClient lifecycle.
func (r *GatewayClientReconciler) Reconcile(ctx context.Context, req ctrl.Request) (res ctrl.Result, err error) {
	geneveClient := &networkingv1beta1.GeneveGatewayClient{}
	if err = r.Get(ctx, req.NamespacedName, geneveClient); err != nil {
		if apierrors.IsNotFound(err) {
			klog.V(4).Infof("Geneve gateway client %q not found", req.NamespacedName)
			return ctrl.Result{}, nil
		}
		klog.Errorf("Unable to get the Geneve gateway client %q: %v", req.NamespacedName, err)
		return ctrl.Result{}, err
	}

	if !geneveClient.DeletionTimestamp.IsZero() {
		if controllerutil.ContainsFinalizer(geneveClient, consts.ClusterRoleBindingFinalizer) {
			if err = enutils.DeleteClusterRoleBinding(ctx, r.Client, geneveClient); err != nil {
				return ctrl.Result{}, err
			}

			controllerutil.RemoveFinalizer(geneveClient, consts.ClusterRoleBindingFinalizer)
			if err = r.Update(ctx, geneveClient); err != nil {
				klog.Errorf("Unable to remove finalizer %q from Geneve gateway client %q: %v",
					consts.ClusterRoleBindingFinalizer, req.NamespacedName, err)
				return ctrl.Result{}, err
			}
		}

		// Resource is deleting and child resources are deleted as well by garbage collector. Nothing to do.
		return ctrl.Result{}, nil
	}

	originalGeneveClient := geneveClient.DeepCopy()

	// Ensure ServiceAccount and ClusterRoleBinding (create or update).
	if err = enutils.EnsureServiceAccountAndClusterRoleBinding(ctx, r.Client, r.Scheme, &geneveClient.Spec.Deployment, geneveClient,
		r.clusterRoleName); err != nil {
		return ctrl.Result{}, err
	}

	// Update if geneveClient has been updated.
	if !equality.Semantic.DeepEqual(originalGeneveClient, geneveClient) {
		if err := r.Update(ctx, geneveClient); err != nil {
			return ctrl.Result{}, err
		}

		// We return here to avoid conflicts.
		return ctrl.Result{}, nil
	}

	deployNsName := types.NamespacedName{Namespace: geneveClient.Namespace, Name: forge.GatewayResourceName(geneveClient.Name)}
	svcNsName := types.NamespacedName{Namespace: geneveClient.Namespace, Name: forge.GatewayResourceName(geneveClient.Name)}

	// Handle status.
	defer func() {
		newErr := r.Status().Update(ctx, geneveClient)
		if newErr != nil {
			if err != nil {
				klog.Errorf("Error reconciling the Geneve gateway client %q: %s", req.NamespacedName, err)
			}
			klog.Errorf("Unable to update the Geneve gateway client status %q: %s", req.NamespacedName, newErr)
			err = newErr
			return
		}

		r.eventRecorder.Event(geneveClient, corev1.EventTypeNormal, "Reconciled", "Geneve gateway client reconciled")
	}()

	// Ensure deployment (create or update).
	_, err = r.ensureDeployment(ctx, geneveClient, deployNsName)
	if err != nil {
		return ctrl.Result{}, err
	}
	r.eventRecorder.Event(geneveClient, corev1.EventTypeNormal, "DeploymentEnforced", "Enforced deployment")

	// The Geneve client is not a pure initiator: the server replies to a
	// configured endpoint rather than to the observed source, so the client must
	// be reachable and always gets a Service whose endpoint it publishes.
	if _, err = r.ensureService(ctx, geneveClient, svcNsName); err != nil {
		return ctrl.Result{}, err
	}
	r.eventRecorder.Event(geneveClient, corev1.EventTypeNormal, "ServiceEnforced", "Enforced service")

	if err := r.handleEndpointStatus(ctx, geneveClient, svcNsName); err != nil {
		klog.Errorf("Error while handling endpoint status: %v", err)
		r.eventRecorder.Event(geneveClient, corev1.EventTypeWarning, "EndpointStatusFailed",
			fmt.Sprintf("Failed to handle endpoint status: %s", err))
		return ctrl.Result{}, err
	}

	// Handle internal endpoint status (requires running pods).
	r.handleInternalEndpointStatus(ctx, geneveClient, deployNsName)

	// Ensure metrics (if set).
	err = enutils.EnsureMetrics(ctx,
		r.Client, r.Scheme,
		geneveClient.Spec.Metrics, geneveClient)
	if err != nil {
		return ctrl.Result{}, err
	}
	r.eventRecorder.Event(geneveClient, corev1.EventTypeNormal, "MetricsEnforced", "Enforced metrics")

	return ctrl.Result{}, nil
}

// SetupWithManager registers the GatewayClientReconciler to the manager.
func (r *GatewayClientReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).Named(consts.CtrlGeneveGatewayClient).
		For(&networkingv1beta1.GeneveGatewayClient{}).
		Owns(&appsv1.Deployment{}).
		Owns(&corev1.Service{}).
		Owns(&corev1.ServiceAccount{}).
		Watches(&corev1.Pod{}, handler.EnqueueRequestsFromMapFunc(podEnquerer)).
		Watches(&rbacv1.ClusterRoleBinding{},
			handler.EnqueueRequestsFromMapFunc(clusterRoleBindingEnquerer)).
		Complete(r)
}

func (r *GatewayClientReconciler) ensureDeployment(ctx context.Context, geneveClient *networkingv1beta1.GeneveGatewayClient,
	depNsName types.NamespacedName) (*appsv1.Deployment, error) {
	dep := appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{
		Name:      depNsName.Name,
		Namespace: depNsName.Namespace,
	}}

	op, err := resource.CreateOrUpdate(ctx, r.Client, &dep, func() error {
		return r.mutateFnGeneveClientDeployment(&dep, geneveClient)
	})
	if err != nil {
		klog.Errorf("error while creating/updating deployment %q (operation: %s): %v", depNsName, op, err)
		return nil, err
	}

	klog.Infof("Deployment %q correctly enforced (operation: %s)", depNsName, op)
	return &dep, nil
}

func (r *GatewayClientReconciler) ensureService(ctx context.Context, geneveClient *networkingv1beta1.GeneveGatewayClient,
	svcNsName types.NamespacedName) (*corev1.Service, error) {
	svc := corev1.Service{ObjectMeta: metav1.ObjectMeta{
		Name:      svcNsName.Name,
		Namespace: svcNsName.Namespace,
	}}

	op, err := resource.CreateOrUpdate(ctx, r.Client, &svc, func() error {
		return r.mutateFnGeneveClientService(&svc, geneveClient)
	})
	if err != nil {
		klog.Errorf("error while creating/updating service %q (operation: %s): %v", svcNsName, op, err)
		return nil, err
	}

	klog.Infof("Service %q correctly enforced (operation: %s)", svcNsName, op)
	return &svc, nil
}

func (r *GatewayClientReconciler) mutateFnGeneveClientService(service *corev1.Service,
	geneveClient *networkingv1beta1.GeneveGatewayClient) error {
	// Forge metadata.
	mapsutil.SmartMergeLabels(service, geneveClient.Spec.Service.Metadata.GetLabels())
	mapsutil.SmartMergeAnnotations(service, geneveClient.Spec.Service.Metadata.GetAnnotations())

	// Ensure the service is never reflected to remote clusters.
	if service.Annotations == nil {
		service.Annotations = map[string]string{}
	}
	service.Annotations[consts.SkipReflectionAnnotationKey] = skipReflectionValue

	// Forge spec.
	serviceClassName := service.Spec.LoadBalancerClass
	service.Spec = geneveClient.Spec.Service.Spec
	if geneveClient.Spec.Service.Spec.LoadBalancerClass == nil {
		service.Spec.LoadBalancerClass = serviceClassName
	}

	// Set Geneve client as owner of the service.
	return controllerutil.SetControllerReference(geneveClient, service, r.Scheme)
}

func (r *GatewayClientReconciler) handleEndpointStatus(ctx context.Context,
	geneveClient *networkingv1beta1.GeneveGatewayClient, svcNsName types.NamespacedName) error {
	var service corev1.Service
	if err := r.Get(ctx, svcNsName, &service); err != nil {
		if apierrors.IsNotFound(err) {
			geneveClient.Status.Endpoint = nil
			return nil
		}
		klog.Error(err)
		return err
	}

	endpointStatus, err := forgeEndpointStatus(ctx, r.Client, &service, svcNsName.Namespace)
	if err != nil {
		// Empty the endpoint status to avoid a misaligned spec and status.
		geneveClient.Status.Endpoint = nil
		return err
	}

	geneveClient.Status.Endpoint = endpointStatus
	return nil
}

func (r *GatewayClientReconciler) mutateFnGeneveClientDeployment(deployment *appsv1.Deployment,
	geneveClient *networkingv1beta1.GeneveGatewayClient) error {
	// Forge metadata.
	mapsutil.SmartMergeLabels(deployment, geneveClient.Spec.Deployment.Metadata.GetLabels())
	mapsutil.SmartMergeAnnotations(deployment, geneveClient.Spec.Deployment.Metadata.GetAnnotations())

	// Forge spec.
	deployment.Spec = geneveClient.Spec.Deployment.Spec

	// Set Geneve client as owner of the deployment.
	return controllerutil.SetControllerReference(geneveClient, deployment, r.Scheme)
}

func (r *GatewayClientReconciler) handleInternalEndpointStatus(ctx context.Context,
	geneveClient *networkingv1beta1.GeneveGatewayClient, deployNsName types.NamespacedName) {
	ige, err := forgeInternalEndpointFromPods(ctx, r.Client, deployNsName.Namespace)
	if err != nil {
		klog.V(4).Infof("Geneve gateway client %q: internal endpoint not available yet: %v",
			deployNsName, err)
		geneveClient.Status.InternalEndpoint = nil
		return
	}

	geneveClient.Status.InternalEndpoint = ige
}

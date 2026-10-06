package controller

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	sitesv1alpha1 "github.com/giordanGarci/staticsite-operator/api/v1alpha1"
)

// StaticSiteReconciler reconciles a StaticSite into a ConfigMap, a Deployment and a Service.
type StaticSiteReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=sites.giordan.io,resources=staticsites,verbs=get;list;watch
// +kubebuilder:rbac:groups=sites.giordan.io,resources=staticsites/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=sites.giordan.io,resources=staticsites/finalizers,verbs=update
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=configmaps;services,verbs=get;list;watch;create;update;patch;delete

// Reconcile drives the cluster toward the state described by a StaticSite.
// It is level-based: every call recomputes the full desired state, so it is safe to run any number of times.
func (r *StaticSiteReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	var site sitesv1alpha1.StaticSite
	if err := r.Get(ctx, req.NamespacedName, &site); err != nil {
		// Deleted: owned objects are garbage collected through their ownerReferences.
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	dep, svc, err := r.reconcileChildren(ctx, &site)
	if err != nil {
		logger.Error(err, "failed to reconcile owned resources")
		if statusErr := r.updateStatus(ctx, &site, nil, nil, err); statusErr != nil {
			logger.Error(statusErr, "failed to update status")
		}
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, r.updateStatus(ctx, &site, dep, svc, nil)
}

func (r *StaticSiteReconciler) reconcileChildren(ctx context.Context, site *sitesv1alpha1.StaticSite) (*appsv1.Deployment, *corev1.Service, error) {
	logger := log.FromContext(ctx)
	objMeta := metav1.ObjectMeta{Name: site.Name, Namespace: site.Namespace}

	cm := &corev1.ConfigMap{ObjectMeta: objMeta}
	op, err := controllerutil.CreateOrUpdate(ctx, r.Client, cm, func() error {
		mutateConfigMap(site, cm)
		return controllerutil.SetControllerReference(site, cm, r.Scheme)
	})
	if err != nil {
		return nil, nil, fmt.Errorf("configmap: %w", err)
	}
	logOp(logger, "ConfigMap", op)

	dep := &appsv1.Deployment{ObjectMeta: objMeta}
	op, err = controllerutil.CreateOrUpdate(ctx, r.Client, dep, func() error {
		mutateDeployment(site, dep)
		return controllerutil.SetControllerReference(site, dep, r.Scheme)
	})
	if err != nil {
		return nil, nil, fmt.Errorf("deployment: %w", err)
	}
	logOp(logger, "Deployment", op)

	svc := &corev1.Service{ObjectMeta: objMeta}
	op, err = controllerutil.CreateOrUpdate(ctx, r.Client, svc, func() error {
		mutateService(site, svc)
		return controllerutil.SetControllerReference(site, svc, r.Scheme)
	})
	if err != nil {
		return nil, nil, fmt.Errorf("service: %w", err)
	}
	logOp(logger, "Service", op)

	return dep, svc, nil
}

func (r *StaticSiteReconciler) updateStatus(ctx context.Context, site *sitesv1alpha1.StaticSite, dep *appsv1.Deployment, svc *corev1.Service, reconcileErr error) error {
	patch := client.MergeFrom(site.DeepCopy())
	site.Status.ObservedGeneration = site.Generation

	cond := metav1.Condition{Type: sitesv1alpha1.ConditionReady, ObservedGeneration: site.Generation}
	switch {
	case reconcileErr != nil:
		cond.Status, cond.Reason, cond.Message = metav1.ConditionFalse, "ReconcileFailed", reconcileErr.Error()
	default:
		site.Status.ReadyReplicas = dep.Status.ReadyReplicas
		site.Status.URL = fmt.Sprintf("http://%s.%s.svc.cluster.local", svc.Name, svc.Namespace)

		desired := site.DesiredReplicas()
		if dep.Status.ReadyReplicas >= desired && dep.Status.UpdatedReplicas >= desired {
			cond.Status, cond.Reason = metav1.ConditionTrue, "AllReplicasReady"
			cond.Message = fmt.Sprintf("%d/%d replicas ready", dep.Status.ReadyReplicas, desired)
		} else {
			cond.Status, cond.Reason = metav1.ConditionFalse, "Progressing"
			cond.Message = fmt.Sprintf("%d/%d replicas ready", dep.Status.ReadyReplicas, desired)
		}
	}
	meta.SetStatusCondition(&site.Status.Conditions, cond)

	return r.Status().Patch(ctx, site, patch)
}

func logOp(logger logr.Logger, kind string, op controllerutil.OperationResult) {
	if op != controllerutil.OperationResultNone {
		logger.Info("reconciled owned resource", "kind", kind, "operation", op)
	}
}

// SetupWithManager registers the controller. Owns() makes changes to child objects
// (including Deployment status updates) re-trigger reconciliation of the parent StaticSite.
func (r *StaticSiteReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&sitesv1alpha1.StaticSite{}).
		Owns(&corev1.ConfigMap{}).
		Owns(&appsv1.Deployment{}).
		Owns(&corev1.Service{}).
		Named("staticsite").
		Complete(r)
}

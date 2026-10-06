package controller

import (
	"context"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	sitesv1alpha1 "github.com/giordanGarci/staticsite-operator/api/v1alpha1"
)

var key = types.NamespacedName{Name: "hello", Namespace: "default"}

func newReconciler(t *testing.T, objs ...client.Object) *StaticSiteReconciler {
	t.Helper()
	s := runtime.NewScheme()
	if err := clientgoscheme.AddToScheme(s); err != nil {
		t.Fatal(err)
	}
	if err := sitesv1alpha1.AddToScheme(s); err != nil {
		t.Fatal(err)
	}
	c := fake.NewClientBuilder().
		WithScheme(s).
		WithObjects(objs...).
		WithStatusSubresource(&sitesv1alpha1.StaticSite{}, &appsv1.Deployment{}).
		Build()
	return &StaticSiteReconciler{Client: c, Scheme: s}
}

func newSite(content string, replicas int32) *sitesv1alpha1.StaticSite {
	return &sitesv1alpha1.StaticSite{
		ObjectMeta: metav1.ObjectMeta{Name: key.Name, Namespace: key.Namespace, Generation: 1},
		Spec:       sitesv1alpha1.StaticSiteSpec{Content: content, Replicas: ptr.To(replicas)},
	}
}

func reconcile(t *testing.T, r *StaticSiteReconciler) {
	t.Helper()
	if _, err := r.Reconcile(context.Background(), ctrl.Request{NamespacedName: key}); err != nil {
		t.Fatalf("reconcile: %v", err)
	}
}

func get[T client.Object](t *testing.T, r *StaticSiteReconciler, obj T) T {
	t.Helper()
	if err := r.Get(context.Background(), key, obj); err != nil {
		t.Fatalf("get %T: %v", obj, err)
	}
	return obj
}

func TestReconcileCreatesOwnedResources(t *testing.T) {
	site := newSite("<h1>hi</h1>", 2)
	r := newReconciler(t, site)
	reconcile(t, r)

	cm := get(t, r, &corev1.ConfigMap{})
	if got := cm.Data[indexFile]; got != "<h1>hi</h1>" {
		t.Errorf("configmap content = %q", got)
	}

	dep := get(t, r, &appsv1.Deployment{})
	if *dep.Spec.Replicas != 2 {
		t.Errorf("replicas = %d, want 2", *dep.Spec.Replicas)
	}
	if img := dep.Spec.Template.Spec.Containers[0].Image; img != sitesv1alpha1.DefaultImage {
		t.Errorf("image = %q, want default %q", img, sitesv1alpha1.DefaultImage)
	}

	svc := get(t, r, &corev1.Service{})
	if svc.Spec.Selector["app.kubernetes.io/instance"] != "hello" {
		t.Errorf("service selector = %v", svc.Spec.Selector)
	}

	for _, obj := range []client.Object{cm, dep, svc} {
		owner := metav1.GetControllerOf(obj)
		if owner == nil || owner.Kind != "StaticSite" || owner.Name != "hello" {
			t.Errorf("%T controller ref = %+v, want StaticSite/hello", obj, owner)
		}
	}
}

func TestContentChangeRollsPods(t *testing.T) {
	r := newReconciler(t, newSite("v1", 1))
	reconcile(t, r)
	before := get(t, r, &appsv1.Deployment{}).Spec.Template.Annotations[ContentHashAnnotation]

	site := get(t, r, &sitesv1alpha1.StaticSite{})
	site.Spec.Content = "v2"
	if err := r.Update(context.Background(), site); err != nil {
		t.Fatal(err)
	}
	reconcile(t, r)

	after := get(t, r, &appsv1.Deployment{}).Spec.Template.Annotations[ContentHashAnnotation]
	if before == "" || before == after {
		t.Errorf("content hash did not change: before=%q after=%q", before, after)
	}
	if got := get(t, r, &corev1.ConfigMap{}).Data[indexFile]; got != "v2" {
		t.Errorf("configmap content = %q, want v2", got)
	}
}

func TestStatusReflectsDeployment(t *testing.T) {
	r := newReconciler(t, newSite("hi", 2))
	reconcile(t, r)

	site := get(t, r, &sitesv1alpha1.StaticSite{})
	if c := meta.FindStatusCondition(site.Status.Conditions, sitesv1alpha1.ConditionReady); c == nil || c.Status != metav1.ConditionFalse {
		t.Fatalf("Ready condition before pods are up = %+v, want False", c)
	}

	// Simulate the Deployment controller reporting ready pods.
	dep := get(t, r, &appsv1.Deployment{})
	dep.Status.ReadyReplicas, dep.Status.UpdatedReplicas = 2, 2
	if err := r.Status().Update(context.Background(), dep); err != nil {
		t.Fatal(err)
	}
	reconcile(t, r)

	site = get(t, r, &sitesv1alpha1.StaticSite{})
	if c := meta.FindStatusCondition(site.Status.Conditions, sitesv1alpha1.ConditionReady); c == nil || c.Status != metav1.ConditionTrue {
		t.Errorf("Ready condition = %+v, want True", c)
	}
	if site.Status.ReadyReplicas != 2 {
		t.Errorf("readyReplicas = %d, want 2", site.Status.ReadyReplicas)
	}
	if want := "http://hello.default.svc.cluster.local"; site.Status.URL != want {
		t.Errorf("url = %q, want %q", site.Status.URL, want)
	}
}

func TestReconcileDeletedSiteIsNoop(t *testing.T) {
	r := newReconciler(t)
	reconcile(t, r)
}

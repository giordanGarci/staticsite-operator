package controller

import (
	"crypto/sha256"
	"encoding/hex"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/ptr"

	sitesv1alpha1 "github.com/giordanGarci/staticsite-operator/api/v1alpha1"
)

const (
	// ContentHashAnnotation is set on the pod template so a content change triggers a rolling update.
	ContentHashAnnotation = "sites.giordan.io/content-hash"

	indexFile  = "index.html"
	htmlDir    = "/usr/share/nginx/html"
	volumeName = "html"
)

func selectorLabels(site *sitesv1alpha1.StaticSite) map[string]string {
	return map[string]string{
		"app.kubernetes.io/name":     "staticsite",
		"app.kubernetes.io/instance": site.Name,
	}
}

func commonLabels(site *sitesv1alpha1.StaticSite) map[string]string {
	labels := selectorLabels(site)
	labels["app.kubernetes.io/managed-by"] = "staticsite-operator"
	return labels
}

func contentHash(content string) string {
	sum := sha256.Sum256([]byte(content))
	return hex.EncodeToString(sum[:])[:16]
}

// mutateConfigMap writes the desired state into cm, keeping fields set by the API server.
func mutateConfigMap(site *sitesv1alpha1.StaticSite, cm *corev1.ConfigMap) {
	cm.Labels = commonLabels(site)
	cm.Data = map[string]string{indexFile: site.Spec.Content}
}

// mutateDeployment writes the desired state into dep, keeping fields set by the API server.
func mutateDeployment(site *sitesv1alpha1.StaticSite, dep *appsv1.Deployment) {
	dep.Labels = commonLabels(site)
	dep.Spec.Replicas = ptr.To(site.DesiredReplicas())

	// The selector is immutable, so it is only set on creation.
	if dep.CreationTimestamp.IsZero() {
		dep.Spec.Selector = &metav1.LabelSelector{MatchLabels: selectorLabels(site)}
	}

	tmpl := &dep.Spec.Template
	tmpl.Labels = commonLabels(site)
	if tmpl.Annotations == nil {
		tmpl.Annotations = map[string]string{}
	}
	tmpl.Annotations[ContentHashAnnotation] = contentHash(site.Spec.Content)

	tmpl.Spec.Volumes = []corev1.Volume{{
		Name: volumeName,
		VolumeSource: corev1.VolumeSource{
			ConfigMap: &corev1.ConfigMapVolumeSource{
				LocalObjectReference: corev1.LocalObjectReference{Name: site.Name},
				DefaultMode:          ptr.To[int32](0o644),
			},
		},
	}}

	if len(tmpl.Spec.Containers) == 0 {
		tmpl.Spec.Containers = []corev1.Container{{Name: "web"}}
	}
	c := &tmpl.Spec.Containers[0]
	c.Image = site.DesiredImage()
	c.Ports = []corev1.ContainerPort{{Name: "http", ContainerPort: 80, Protocol: corev1.ProtocolTCP}}
	c.VolumeMounts = []corev1.VolumeMount{{Name: volumeName, MountPath: htmlDir, ReadOnly: true}}
	c.ReadinessProbe = &corev1.Probe{
		ProbeHandler: corev1.ProbeHandler{
			HTTPGet: &corev1.HTTPGetAction{Path: "/", Port: intstr.FromString("http"), Scheme: corev1.URISchemeHTTP},
		},
		PeriodSeconds:    5,
		TimeoutSeconds:   1,
		SuccessThreshold: 1,
		FailureThreshold: 3,
	}
}

// mutateService writes the desired state into svc, keeping fields set by the API server (e.g. clusterIP).
func mutateService(site *sitesv1alpha1.StaticSite, svc *corev1.Service) {
	svc.Labels = commonLabels(site)
	svc.Spec.Selector = selectorLabels(site)
	svc.Spec.Ports = []corev1.ServicePort{{
		Name:       "http",
		Port:       80,
		TargetPort: intstr.FromString("http"),
		Protocol:   corev1.ProtocolTCP,
	}}
}

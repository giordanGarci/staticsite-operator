package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	// DefaultImage is used when spec.image is empty.
	DefaultImage = "nginx:1.27-alpine"

	// ConditionReady reports whether every desired replica is serving the site.
	ConditionReady = "Ready"
)

// StaticSiteSpec defines the desired state of a StaticSite.
type StaticSiteSpec struct {
	// Content is the HTML served as index.html.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=65536
	Content string `json:"content"`

	// Replicas is the number of web server pods.
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=10
	// +kubebuilder:default=1
	// +optional
	Replicas *int32 `json:"replicas,omitempty"`

	// Image is the web server image. It must serve /usr/share/nginx/html on port 80.
	// +kubebuilder:default="nginx:1.27-alpine"
	// +optional
	Image string `json:"image,omitempty"`
}

// StaticSiteStatus defines the observed state of a StaticSite.
type StaticSiteStatus struct {
	// ObservedGeneration is the .metadata.generation the status was computed from.
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// ReadyReplicas is the number of pods ready to serve traffic.
	// +optional
	ReadyReplicas int32 `json:"readyReplicas,omitempty"`

	// URL is the in-cluster address of the site.
	// +optional
	URL string `json:"url,omitempty"`

	// Conditions follow the standard Kubernetes condition conventions.
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// StaticSite serves a single HTML page through an nginx Deployment and a Service.
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:subresource:scale:specpath=.spec.replicas,statuspath=.status.readyReplicas
// +kubebuilder:resource:shortName=site
// +kubebuilder:printcolumn:name="Replicas",type=integer,JSONPath=`.spec.replicas`
// +kubebuilder:printcolumn:name="Ready",type=integer,JSONPath=`.status.readyReplicas`
// +kubebuilder:printcolumn:name="URL",type=string,JSONPath=`.status.url`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`
type StaticSite struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   StaticSiteSpec   `json:"spec,omitempty"`
	Status StaticSiteStatus `json:"status,omitempty"`
}

// StaticSiteList contains a list of StaticSite.
// +kubebuilder:object:root=true
type StaticSiteList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []StaticSite `json:"items"`
}

// DesiredReplicas returns spec.replicas, falling back to the CRD default.
func (s *StaticSite) DesiredReplicas() int32 {
	if s.Spec.Replicas == nil {
		return 1
	}
	return *s.Spec.Replicas
}

// DesiredImage returns spec.image, falling back to the CRD default.
func (s *StaticSite) DesiredImage() string {
	if s.Spec.Image == "" {
		return DefaultImage
	}
	return s.Spec.Image
}

func init() {
	SchemeBuilder.Register(&StaticSite{}, &StaticSiteList{})
}

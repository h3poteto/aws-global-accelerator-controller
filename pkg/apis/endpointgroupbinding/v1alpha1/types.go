package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +kubebuilder:object:root=true

// EndpointGroupBinding
// +kubebuilder:printcolumn:name="EndpointArn",type=string,JSONPath=`.status.endpointArn`
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"
type EndpointGroupBinding struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   EndpointGroupBindingSpec   `json:"spec,omitempty"`
	Status EndpointGroupBindingStatus `json:"status,omitempty"`
}

type EndpointGroupBindingSpec struct {
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Type:=string
	EndpointGroupArn string `json:"endpointGroupArn"`
	// +optional
	// +kubebuilder:validation:Type:=boolean
	// +kubebuilder:default=false
	ClientIPPreservation bool `json:"clientIPPreservation"`
	// +optional
	// +kubebuilder:validation:Type:=integer
	// +kubebuilder:default=0
	Weight int64 `json:"weight"`

	// +optional
	ServiceRef *ServiceReference `json:"serviceRef"`
	// +optional
	IngressRef *IngressReference `json:"ingressRef"`
}

type ServiceReference struct {
	Name string `json:"name"`
}

type IngressReference struct {
	Name string `json:"name"`
}

type EndpointGroupBindingStatus struct {
	EndpointIds        []string `json:"endpointIds"`
	ObservedGeneration int64    `json:"observedGeneration"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +kubebuilder:object:root=true
// EndpointGroupBindingList is a list of EndpointGroupBinding
type EndpointGroupBindingList struct {
	metav1.TypeMeta `json:",inline"`
	// +optional
	metav1.ListMeta `json:"metadata,omitempty"`

	Items []EndpointGroupBinding `json:"items"`
}

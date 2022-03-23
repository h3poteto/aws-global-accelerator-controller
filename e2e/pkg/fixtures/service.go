package fixtures

import (
	"github.com/h3poteto/aws-global-accelerator-controller/pkg/apis"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

func NewNLBService(ns, name, hostname string) *corev1.Service {
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: ns,
			Annotations: map[string]string{
				apis.AWSGlobalAcceleratorManagedAnnotation:                                       "yes",
				apis.Route53HostnameAnnotation:                                                   hostname,
				"service.beta.kubernetes.io/aws-load-balancer-backend-protocol":                  "tcp",
				"service.beta.kubernetes.io/aws-load-balancer-cross-zone-load-balancing-enabled": "true",
				"service.beta.kubernetes.io/aws-load-balancer-type":                              "nlb",
				"service.beta.kubernetes.io/aws-load-balancer-scheme":                            "internet-facing",
			},
		},
		Spec: corev1.ServiceSpec{
			Ports: []corev1.ServicePort{
				{
					Name:     "http",
					Protocol: corev1.ProtocolTCP,
					Port:     80,
					TargetPort: intstr.IntOrString{
						IntVal: 8080,
					},
				},
				{
					Name:     "https",
					Protocol: corev1.ProtocolTCP,
					Port:     443,
					TargetPort: intstr.IntOrString{
						IntVal: 6443,
					},
				},
			},
			Selector: map[string]string{
				"app": "h3poteto",
			},
			Type:                  corev1.ServiceTypeLoadBalancer,
			SessionAffinity:       corev1.ServiceAffinityNone,
			ExternalTrafficPolicy: corev1.ServiceExternalTrafficPolicyTypeLocal,
		},
	}
}

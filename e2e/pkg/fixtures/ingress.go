package fixtures

import (
	"os"
	"strconv"

	apis "github.com/h3poteto/aws-global-accelerator-controller/pkg/apis/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	utilpointer "k8s.io/utils/pointer"
)

func NewALBIngress(ns, name, hostname string, port int) *networkingv1.Ingress {
	svc := newBackendService(ns, name)
	pt := networkingv1.PathTypePrefix
	listenPorts := "[{\"HTTPS\":" + strconv.Itoa(port) + "}]"

	return &networkingv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: ns,
			Annotations: map[string]string{
				apis.AWSGlobalAcceleratorManagedAnnotation:  "yes",
				apis.Route53HostnameAnnotation:              hostname,
				"alb.ingress.kubernetes.io/scheme":          "internet-facing",
				"alb.ingress.kubernetes.io/certificate-arn": os.Getenv("E2E_ACM_ARN"),
				"alb.ingress.kubernetes.io/listen-ports":    listenPorts,
			},
		},
		Spec: networkingv1.IngressSpec{
			IngressClassName: utilpointer.StringPtr("alb"),
			Rules: []networkingv1.IngressRule{
				{
					IngressRuleValue: networkingv1.IngressRuleValue{
						HTTP: &networkingv1.HTTPIngressRuleValue{
							Paths: []networkingv1.HTTPIngressPath{
								{
									Path:     "/",
									PathType: &pt,
									Backend: networkingv1.IngressBackend{
										Service: &networkingv1.IngressServiceBackend{
											Name: svc.Name,
											Port: networkingv1.ServiceBackendPort{
												Number: 80,
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}
}

func newBackendService(ns, name string) *corev1.Service {
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: ns,
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
			Type: corev1.ServiceTypeNodePort,
		},
	}
}

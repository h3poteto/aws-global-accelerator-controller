package aws

import (
	"testing"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/globalaccelerator"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	utilpointer "k8s.io/utils/pointer"
)

func TestListenerProtocolChange(t *testing.T) {
	cases := []struct {
		title          string
		listener       *globalaccelerator.Listener
		svc            *corev1.Service
		expectedResult bool
	}{
		{
			title: "Protocol is not changed, with single protocol",
			listener: &globalaccelerator.Listener{
				ListenerArn: aws.String("sample"),
				Protocol:    aws.String("UDP"),
			},
			svc: &corev1.Service{
				Spec: corev1.ServiceSpec{
					Ports: []corev1.ServicePort{
						{
							Name:     "udp",
							Protocol: corev1.ProtocolUDP,
						},
					},
				},
			},
			expectedResult: false,
		},
		{
			title: "Protocol is not changed, with multiple protocol",
			listener: &globalaccelerator.Listener{
				ListenerArn: aws.String("sample"),
				Protocol:    aws.String("TCP"),
			},
			svc: &corev1.Service{
				Spec: corev1.ServiceSpec{
					Ports: []corev1.ServicePort{
						{
							Name:     "tcp",
							Protocol: corev1.ProtocolTCP,
						},
						{
							Name:     "tcp",
							Protocol: corev1.ProtocolTCP,
						},
					},
				},
			},
			expectedResult: false,
		},
		{
			title: "Protocol not is changed, with multiple different protocol",
			listener: &globalaccelerator.Listener{
				ListenerArn: aws.String("sample"),
				Protocol:    aws.String("TCP"),
			},
			svc: &corev1.Service{
				Spec: corev1.ServiceSpec{
					Ports: []corev1.ServicePort{
						{
							Name:     "udp",
							Protocol: corev1.ProtocolUDP,
						},
						{
							Name:     "tcp",
							Protocol: corev1.ProtocolTCP,
						},
					},
				},
			},
			expectedResult: false,
		},
		{
			title: "Protocol is changed, with single protocol",
			listener: &globalaccelerator.Listener{
				ListenerArn: aws.String("sample"),
				Protocol:    aws.String("TCP"),
			},
			svc: &corev1.Service{
				Spec: corev1.ServiceSpec{
					Ports: []corev1.ServicePort{
						{
							Name:     "udp",
							Protocol: corev1.ProtocolUDP,
						},
					},
				},
			},
			expectedResult: true,
		},
		{
			title: "Protocol is changed, with multiple protocol",
			listener: &globalaccelerator.Listener{
				ListenerArn: aws.String("sample"),
				Protocol:    aws.String("TCP"),
			},
			svc: &corev1.Service{
				Spec: corev1.ServiceSpec{
					Ports: []corev1.ServicePort{
						{
							Name:     "udp",
							Protocol: corev1.ProtocolUDP,
						},
						{
							Name:     "udp",
							Protocol: corev1.ProtocolUDP,
						},
					},
				},
			},
			expectedResult: true,
		},
		{
			title: "Protocol is changed, with multiple different protocol",
			listener: &globalaccelerator.Listener{
				ListenerArn: aws.String("sample"),
				Protocol:    aws.String("TCP"),
			},
			svc: &corev1.Service{
				Spec: corev1.ServiceSpec{
					Ports: []corev1.ServicePort{
						{
							Name:     "tcp",
							Protocol: corev1.ProtocolTCP,
						},
						{
							Name:     "udp",
							Protocol: corev1.ProtocolUDP,
						},
					},
				},
			},
			expectedResult: true,
		},
	}

	for _, c := range cases {
		t.Run(c.title, func(tt *testing.T) {
			res := listenerProtocolChangedFromService(c.listener, c.svc)
			assert.Equal(tt, c.expectedResult, res)
		})

	}
}

func TestListenerPortChanged(t *testing.T) {
	cases := []struct {
		title    string
		listener *globalaccelerator.Listener
		svc      *corev1.Service
		expected bool
	}{
		{
			title: "Single port is not changed",
			listener: &globalaccelerator.Listener{
				ListenerArn: aws.String("sample"),
				PortRanges: []*globalaccelerator.PortRange{
					&globalaccelerator.PortRange{
						FromPort: aws.Int64(80),
						ToPort:   aws.Int64(80),
					},
				},
			},
			svc: &corev1.Service{
				Spec: corev1.ServiceSpec{
					Ports: []corev1.ServicePort{
						{
							Port: 80,
						},
					},
				},
			},
			expected: false,
		},
		{
			title: "Multiple ports are not changed",
			listener: &globalaccelerator.Listener{
				ListenerArn: aws.String("sample"),
				PortRanges: []*globalaccelerator.PortRange{
					&globalaccelerator.PortRange{
						FromPort: aws.Int64(80),
						ToPort:   aws.Int64(80),
					},
					&globalaccelerator.PortRange{
						FromPort: aws.Int64(443),
						ToPort:   aws.Int64(443),
					},
					&globalaccelerator.PortRange{
						FromPort: aws.Int64(8080),
						ToPort:   aws.Int64(8080),
					},
				},
			},
			svc: &corev1.Service{
				Spec: corev1.ServiceSpec{
					Ports: []corev1.ServicePort{
						{
							Port: 443,
						},
						{
							Port: 8080,
						},
						{
							Port: 80,
						},
					},
				},
			},
			expected: false,
		},
		{
			title: "Single port is changed",
			listener: &globalaccelerator.Listener{
				ListenerArn: aws.String("sample"),
				PortRanges: []*globalaccelerator.PortRange{
					&globalaccelerator.PortRange{
						FromPort: aws.Int64(80),
						ToPort:   aws.Int64(80),
					},
				},
			},
			svc: &corev1.Service{
				Spec: corev1.ServiceSpec{
					Ports: []corev1.ServicePort{
						{
							Port: 443,
						},
					},
				},
			},
			expected: true,
		},
		{
			title: "Multiple ports are changed",
			listener: &globalaccelerator.Listener{
				ListenerArn: aws.String("sample"),
				PortRanges: []*globalaccelerator.PortRange{
					&globalaccelerator.PortRange{
						FromPort: aws.Int64(80),
						ToPort:   aws.Int64(80),
					},
					&globalaccelerator.PortRange{
						FromPort: aws.Int64(8080),
						ToPort:   aws.Int64(8080),
					},
				},
			},
			svc: &corev1.Service{
				Spec: corev1.ServiceSpec{
					Ports: []corev1.ServicePort{
						{
							Port: 443,
						},
						{
							Port: 8080,
						},
					},
				},
			},
			expected: true,
		},
		{
			title: "Ports are increased",
			listener: &globalaccelerator.Listener{
				ListenerArn: aws.String("sample"),
				PortRanges: []*globalaccelerator.PortRange{
					&globalaccelerator.PortRange{
						FromPort: aws.Int64(80),
						ToPort:   aws.Int64(80),
					},
					&globalaccelerator.PortRange{
						FromPort: aws.Int64(8080),
						ToPort:   aws.Int64(8080),
					},
				},
			},
			svc: &corev1.Service{
				Spec: corev1.ServiceSpec{
					Ports: []corev1.ServicePort{
						{
							Port: 443,
						},
						{
							Port: 8080,
						},
						{
							Port: 8081,
						},
					},
				},
			},
			expected: true,
		},
		{
			title: "Ports are decreased",
			listener: &globalaccelerator.Listener{
				ListenerArn: aws.String("sample"),
				PortRanges: []*globalaccelerator.PortRange{
					&globalaccelerator.PortRange{
						FromPort: aws.Int64(80),
						ToPort:   aws.Int64(80),
					},
					&globalaccelerator.PortRange{
						FromPort: aws.Int64(443),
						ToPort:   aws.Int64(443),
					},
					&globalaccelerator.PortRange{
						FromPort: aws.Int64(8080),
						ToPort:   aws.Int64(8080),
					},
				},
			},
			svc: &corev1.Service{
				Spec: corev1.ServiceSpec{
					Ports: []corev1.ServicePort{
						{
							Port: 443,
						},
					},
				},
			},
			expected: true,
		},
	}

	for _, c := range cases {
		t.Run(c.title, func(tt *testing.T) {
			res := listenerPortChangedFromService(c.listener, c.svc)
			assert.Equal(tt, c.expected, res)
		})
	}
}

func TestListenerForIngress(t *testing.T) {
	pt := networkingv1.PathTypePrefix

	cases := []struct {
		title            string
		ingress          *networkingv1.Ingress
		expectedPorts    []int64
		expectedProtocol string
	}{
		{
			title: "Only spec rules",
			ingress: &networkingv1.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Name:        "Test ingress",
					Annotations: map[string]string{},
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
													Name: "Test service",
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
			},
			expectedPorts:    []int64{80},
			expectedProtocol: "TCP",
		},
		{
			title: "DefaultBackend is specified",
			ingress: &networkingv1.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Name:        "Test ingress",
					Annotations: map[string]string{},
				},
				Spec: networkingv1.IngressSpec{
					IngressClassName: utilpointer.StringPtr("alb"),
					DefaultBackend: &networkingv1.IngressBackend{
						Service: &networkingv1.IngressServiceBackend{
							Name: "Test service",
							Port: networkingv1.ServiceBackendPort{
								Number: 8080,
							},
						},
					},
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
													Name: "Test service",
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
			},
			expectedPorts:    []int64{8080, 80},
			expectedProtocol: "TCP",
		},
		{
			title: "ListenPorts annotation is specified",
			ingress: &networkingv1.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Name: "Test ingress",
					Annotations: map[string]string{
						"alb.ingress.kubernetes.io/listen-ports": "[{\"HTTP\": 80}, {\"HTTPS\": 443}]",
					},
				},
				Spec: networkingv1.IngressSpec{
					IngressClassName: utilpointer.StringPtr("alb"),
					DefaultBackend: &networkingv1.IngressBackend{
						Service: &networkingv1.IngressServiceBackend{
							Name: "Test service",
							Port: networkingv1.ServiceBackendPort{
								Number: 8080,
							},
						},
					},
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
													Name: "Test service",
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
			},
			expectedPorts:    []int64{80, 443},
			expectedProtocol: "TCP",
		},
	}

	for _, c := range cases {
		t.Run(c.title, func(tt *testing.T) {
			ports, protocol := listenerForIngress(c.ingress)
			assert.Equal(tt, c.expectedPorts, ports)
			assert.Equal(tt, c.expectedProtocol, protocol)
		})
	}
}

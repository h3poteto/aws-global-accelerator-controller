package aws

import (
	"errors"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	gatypes "github.com/aws/aws-sdk-go-v2/service/globalaccelerator/types"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	utilpointer "k8s.io/utils/pointer"
)

func TestListenerProtocolChange(t *testing.T) {
	cases := []struct {
		title          string
		listener       *gatypes.Listener
		svc            *corev1.Service
		expectedResult bool
	}{
		{
			title: "Protocol is not changed, with single protocol",
			listener: &gatypes.Listener{
				ListenerArn: aws.String("sample"),
				Protocol:    gatypes.ProtocolUdp,
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
			listener: &gatypes.Listener{
				ListenerArn: aws.String("sample"),
				Protocol:    gatypes.ProtocolTcp,
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
			listener: &gatypes.Listener{
				ListenerArn: aws.String("sample"),
				Protocol:    gatypes.ProtocolTcp,
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
			listener: &gatypes.Listener{
				ListenerArn: aws.String("sample"),
				Protocol:    gatypes.ProtocolTcp,
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
			listener: &gatypes.Listener{
				ListenerArn: aws.String("sample"),
				Protocol:    gatypes.ProtocolTcp,
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
			listener: &gatypes.Listener{
				ListenerArn: aws.String("sample"),
				Protocol:    gatypes.ProtocolTcp,
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
		listener *gatypes.Listener
		svc      *corev1.Service
		expected bool
	}{
		{
			title: "Single port is not changed",
			listener: &gatypes.Listener{
				ListenerArn: aws.String("sample"),
				PortRanges: []gatypes.PortRange{
					gatypes.PortRange{
						FromPort: aws.Int32(80),
						ToPort:   aws.Int32(80),
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
			listener: &gatypes.Listener{
				ListenerArn: aws.String("sample"),
				PortRanges: []gatypes.PortRange{
					gatypes.PortRange{
						FromPort: aws.Int32(80),
						ToPort:   aws.Int32(80),
					},
					gatypes.PortRange{
						FromPort: aws.Int32(443),
						ToPort:   aws.Int32(443),
					},
					gatypes.PortRange{
						FromPort: aws.Int32(8080),
						ToPort:   aws.Int32(8080),
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
			listener: &gatypes.Listener{
				ListenerArn: aws.String("sample"),
				PortRanges: []gatypes.PortRange{
					gatypes.PortRange{
						FromPort: aws.Int32(80),
						ToPort:   aws.Int32(80),
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
			listener: &gatypes.Listener{
				ListenerArn: aws.String("sample"),
				PortRanges: []gatypes.PortRange{
					gatypes.PortRange{
						FromPort: aws.Int32(80),
						ToPort:   aws.Int32(80),
					},
					gatypes.PortRange{
						FromPort: aws.Int32(8080),
						ToPort:   aws.Int32(8080),
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
			listener: &gatypes.Listener{
				ListenerArn: aws.String("sample"),
				PortRanges: []gatypes.PortRange{
					gatypes.PortRange{
						FromPort: aws.Int32(80),
						ToPort:   aws.Int32(80),
					},
					gatypes.PortRange{
						FromPort: aws.Int32(8080),
						ToPort:   aws.Int32(8080),
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
			listener: &gatypes.Listener{
				ListenerArn: aws.String("sample"),
				PortRanges: []gatypes.PortRange{
					gatypes.PortRange{
						FromPort: aws.Int32(80),
						ToPort:   aws.Int32(80),
					},
					gatypes.PortRange{
						FromPort: aws.Int32(443),
						ToPort:   aws.Int32(443),
					},
					gatypes.PortRange{
						FromPort: aws.Int32(8080),
						ToPort:   aws.Int32(8080),
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
		expectedPorts    []int32
		expectedProtocol gatypes.Protocol
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
			expectedPorts:    []int32{80},
			expectedProtocol: gatypes.ProtocolTcp,
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
			expectedPorts:    []int32{8080, 80},
			expectedProtocol: gatypes.ProtocolTcp,
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
			expectedPorts:    []int32{80, 443},
			expectedProtocol: gatypes.ProtocolTcp,
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

func TestSelectListenerForPorts(t *testing.T) {
	listener := func(arn string, protocol gatypes.Protocol, ports ...int32) gatypes.Listener {
		l := gatypes.Listener{ListenerArn: aws.String(arn), Protocol: protocol}
		for _, p := range ports {
			l.PortRanges = append(l.PortRanges, gatypes.PortRange{FromPort: aws.Int32(p), ToPort: aws.Int32(p)})
		}
		return l
	}
	// Mirrors a shared accelerator where TCP and UDP listeners reuse port numbers.
	listeners := []gatypes.Listener{
		listener("nginx", gatypes.ProtocolTcp, 8080, 8443, 9443),
		listener("update-status", gatypes.ProtocolUdp, 8080),
		listener("sia-tcp", gatypes.ProtocolTcp, 5001),
		listener("sia-udp", gatypes.ProtocolUdp, 5001),
		{
			ListenerArn: aws.String("range"),
			Protocol:    gatypes.ProtocolTcp,
			PortRanges:  []gatypes.PortRange{{FromPort: aws.Int32(1303), ToPort: aws.Int32(1306)}},
		},
	}

	cases := []struct {
		title       string
		ports       []int32
		protocol    gatypes.Protocol
		expectedArn string
		notFound    bool
		ambiguous   bool
	}{
		{title: "exact ports", ports: []int32{8080, 8443, 9443}, protocol: gatypes.ProtocolTcp, expectedArn: "nginx"},
		{title: "same port on other protocol", ports: []int32{8080}, protocol: gatypes.ProtocolUdp, expectedArn: "update-status"},
		{title: "service added a port", ports: []int32{5001, 5002}, protocol: gatypes.ProtocolTcp, expectedArn: "sia-tcp"},
		{title: "port inside a range", ports: []int32{1305}, protocol: gatypes.ProtocolTcp, expectedArn: "range"},
		{title: "no listener uses the ports", ports: []int32{9505}, protocol: gatypes.ProtocolTcp, notFound: true},
		{title: "ports split across listeners", ports: []int32{5001, 8443}, protocol: gatypes.ProtocolTcp, ambiguous: true},
	}

	for _, c := range cases {
		t.Run(c.title, func(t *testing.T) {
			l, err := selectListenerForPorts(listeners, c.ports, c.protocol)
			switch {
			case c.notFound:
				var notFoundErr *gatypes.ListenerNotFoundException
				assert.ErrorAs(t, err, &notFoundErr)
			case c.ambiguous:
				assert.Error(t, err)
				var notFoundErr *gatypes.ListenerNotFoundException
				assert.False(t, errors.As(err, &notFoundErr))
			default:
				assert.NoError(t, err)
				assert.Equal(t, c.expectedArn, aws.ToString(l.ListenerArn))
			}
		})
	}
}

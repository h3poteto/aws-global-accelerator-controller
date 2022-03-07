package aws

import (
	"testing"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/globalaccelerator"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
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

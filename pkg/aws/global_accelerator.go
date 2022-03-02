package aws

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/elbv2"
	"github.com/aws/aws-sdk-go/service/globalaccelerator"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/klog/v2"
)

func (a *AWS) EnsureGlobalAccelerator(ctx context.Context, svc *corev1.Service, ingress *corev1.LoadBalancerIngress, lbName, region string, correspondence map[string]string) (*string, error) {
	lb, err := a.findNetworkLoadBalancer(ctx, lbName)
	if err != nil {
		return nil, err
	}
	if *lb.DNSName != ingress.Hostname {
		return nil, fmt.Errorf("LoadBalancer's DNS name is not matched: %s", *lb.DNSName)
	}
	klog.Infof("LoadBalancer is %s", *lb.LoadBalancerArn)
	var accelerator *globalaccelerator.Accelerator
	// Confirm Global Accelerator
	if value, ok := correspondence[ingress.Hostname]; ok {
		accelerator, _ = a.findGlobalAccelerator(ctx, value)
	}

	if accelerator == nil {
		// Create Global Accelerator
		klog.Infof("Creating Global Accelerator for %s", *lb.DNSName)
		createdArn, err := a.createGlobalAccelerator(ctx, lb, svc, region)
		if err != nil {
			if createdArn != nil {
				a.cleanupGlobalAccelerator(ctx, *createdArn)
			}
			return nil, err
		}
		return createdArn, nil
	} else {
		// Update Global Accelerator
		// TODO
		klog.Infof("accelerator: %s", *accelerator.AcceleratorArn)
		return accelerator.AcceleratorArn, nil
	}
}

func (a *AWS) findGlobalAccelerator(ctx context.Context, arn string) (*globalaccelerator.Accelerator, error) {
	input := &globalaccelerator.DescribeAcceleratorInput{
		AcceleratorArn: aws.String(arn),
	}
	res, err := a.ga.DescribeAcceleratorWithContext(ctx, input)
	if err != nil {
		return nil, err
	}
	return res.Accelerator, nil
}

func (a *AWS) createGlobalAccelerator(ctx context.Context, lb *elbv2.LoadBalancer, svc *corev1.Service, region string) (*string, error) {
	acceleratorInput := &globalaccelerator.CreateAcceleratorInput{
		Enabled:       aws.Bool(true),
		IpAddressType: aws.String("IPV4"),
		Name:          aws.String(svc.Namespace + "-" + svc.Name),
		Tags: []*globalaccelerator.Tag{
			&globalaccelerator.Tag{
				Key:   aws.String("aws-global-accelerator-controller-managed"),
				Value: aws.String(svc.Namespace + "/" + svc.Name),
			},
		},
	}
	acceleratorRes, err := a.ga.CreateAcceleratorWithContext(ctx, acceleratorInput)
	if err != nil {
		return nil, err
	}
	klog.Infof("Global Accelerator is created: %s", *acceleratorRes.Accelerator.AcceleratorArn)

	var ports []*globalaccelerator.PortRange
	protocol := "TCP"
	for _, p := range svc.Spec.Ports {
		ports = append(ports, &globalaccelerator.PortRange{
			FromPort: aws.Int64(int64(p.Port)),
			ToPort:   aws.Int64(int64(p.Port)),
		})
		protocol = string(p.Protocol)
	}
	listenerInput := &globalaccelerator.CreateListenerInput{
		AcceleratorArn: acceleratorRes.Accelerator.AcceleratorArn,
		ClientAffinity: aws.String("NONE"),
		PortRanges:     ports,
		Protocol:       aws.String(protocol),
	}
	listenerRes, err := a.ga.CreateListenerWithContext(ctx, listenerInput)
	if err != nil {
		return acceleratorRes.Accelerator.AcceleratorArn, err
	}
	klog.Infof("Listener is created: %s", *listenerRes.Listener.ListenerArn)

	endpointInput := &globalaccelerator.CreateEndpointGroupInput{
		EndpointConfigurations: []*globalaccelerator.EndpointConfiguration{
			&globalaccelerator.EndpointConfiguration{
				EndpointId: lb.LoadBalancerArn,
			},
		},
		EndpointGroupRegion: aws.String(region),
		ListenerArn:         listenerRes.Listener.ListenerArn,
	}
	endpointRes, err := a.ga.CreateEndpointGroupWithContext(ctx, endpointInput)
	if err != nil {
		return acceleratorRes.Accelerator.AcceleratorArn, err
	}
	klog.Infof("EndpointGroup is created: %s", *endpointRes.EndpointGroup.EndpointGroupArn)

	return acceleratorRes.Accelerator.AcceleratorArn, nil
}

func (a *AWS) cleanupGlobalAccelerator(ctx context.Context, arn string) {
	// TODO:
}

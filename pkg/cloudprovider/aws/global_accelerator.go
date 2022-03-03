package aws

import (
	"context"
	"errors"
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
		accelerator, _ = a.getAccelerator(ctx, value)
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
		klog.Infof("Updating Global Accelerator: %s", *accelerator.AcceleratorArn)
		if err := a.updateGlobalAccelerator(ctx, accelerator, lb, svc, region); err != nil {
			return nil, err
		}
		return accelerator.AcceleratorArn, nil
	}
}

func (a *AWS) createGlobalAccelerator(ctx context.Context, lb *elbv2.LoadBalancer, svc *corev1.Service, region string) (*string, error) {
	accelerator, err := a.createAccelerator(ctx, svc.Namespace+"-"+svc.Name)
	if err != nil {
		return nil, err
	}

	listener, err := a.createListener(ctx, accelerator, svc)
	if err != nil {
		return accelerator.AcceleratorArn, err
	}
	_, err = a.createEndpointGroup(ctx, listener, lb.LoadBalancerArn, region)
	if err != nil {
		return accelerator.AcceleratorArn, err
	}

	return accelerator.AcceleratorArn, nil
}

func (a *AWS) cleanupGlobalAccelerator(ctx context.Context, arn string) {
	// TODO:
}

func (a *AWS) updateGlobalAccelerator(ctx context.Context, accelerator *globalaccelerator.Accelerator, lb *elbv2.LoadBalancer, svc *corev1.Service, region string) error {
	listener, err := a.getListener(ctx, *accelerator.AcceleratorArn)
	if err != nil {
		var notFoundErr *globalaccelerator.ListenerNotFoundException
		if errors.As(err, &notFoundErr) {
			listener, err = a.createListener(ctx, accelerator, svc)
			if err != nil {
				klog.Error(err)
				return err
			}
		} else {
			klog.Error(err)
			return err
		}
	}
	if listenerProtocolChanged(listener, svc) || listenerPortChanged(listener, svc) {
		listener, err = a.updateListener(ctx, listener, svc)
		if err != nil {
			klog.Error(err)
			return err
		}
	}
	endpoint, err := a.getEndpointGroup(ctx, *listener.ListenerArn)
	if err != nil {
		var notFoundErr *globalaccelerator.EndpointGroupNotFoundException
		if errors.As(err, &notFoundErr) {
			endpoint, err = a.createEndpointGroup(ctx, listener, lb.LoadBalancerArn, region)
			if err != nil {
				klog.Error(err)
				return err
			}
		} else {
			klog.Error(err)
			return err
		}
	}
	if !endpointContainsLB(endpoint, lb) {
		endpoint, err = a.updateEndpointGroup(ctx, endpoint, lb.LoadBalancerArn)
	}

	return nil
}

func listenerProtocolChanged(listener *globalaccelerator.Listener, svc *corev1.Service) bool {
	protocol := "TCP"
	for _, p := range svc.Spec.Ports {
		if p.Protocol != "" {
			protocol = string(p.Protocol)
		}
	}
	return *listener.Protocol != protocol

}

func listenerPortChanged(listener *globalaccelerator.Listener, svc *corev1.Service) bool {
	portCount := make(map[int]int)
	for _, p := range listener.PortRanges {
		portCount[int(*p.FromPort)]++

	}
	for _, p := range svc.Spec.Ports {
		portCount[int(p.Port)]++

	}
	for _, value := range portCount {
		if value <= 1 {
			return true
		}
	}
	return false
}

func endpointContainsLB(endpoint *globalaccelerator.EndpointGroup, lb *elbv2.LoadBalancer) bool {
	for _, d := range endpoint.EndpointDescriptions {
		if *d.EndpointId == *lb.LoadBalancerArn {
			return true
		}
	}
	return false
}

//---------------------------------
// Accelerator methods
//---------------------------------
func (a *AWS) getAccelerator(ctx context.Context, arn string) (*globalaccelerator.Accelerator, error) {
	input := &globalaccelerator.DescribeAcceleratorInput{
		AcceleratorArn: aws.String(arn),
	}
	res, err := a.ga.DescribeAcceleratorWithContext(ctx, input)
	if err != nil {
		return nil, err
	}
	return res.Accelerator, nil
}

func (a *AWS) createAccelerator(ctx context.Context, name string) (*globalaccelerator.Accelerator, error) {
	acceleratorInput := &globalaccelerator.CreateAcceleratorInput{
		Enabled:       aws.Bool(true),
		IpAddressType: aws.String("IPV4"),
		Name:          aws.String(name),
		Tags: []*globalaccelerator.Tag{
			&globalaccelerator.Tag{
				Key:   aws.String("aws-global-accelerator-controller-managed"),
				Value: aws.String(name),
			},
		},
	}
	acceleratorRes, err := a.ga.CreateAcceleratorWithContext(ctx, acceleratorInput)
	if err != nil {
		return nil, err
	}
	klog.Infof("Global Accelerator is created: %s", *acceleratorRes.Accelerator.AcceleratorArn)
	return acceleratorRes.Accelerator, nil
}

//---------------------------------
// Lstener methods
//---------------------------------
func (a *AWS) getListener(ctx context.Context, acceleratorArn string) (*globalaccelerator.Listener, error) {
	input := &globalaccelerator.ListListenersInput{
		AcceleratorArn: aws.String(acceleratorArn),
		MaxResults:     aws.Int64(100),
	}
	res, err := a.ga.ListListenersWithContext(ctx, input)
	if err != nil {
		return nil, err
	}
	if len(res.Listeners) <= 0 {
		return nil, &globalaccelerator.ListenerNotFoundException{}
	} else if len(res.Listeners) > 1 {
		return nil, errors.New("Too many listeners")
	}
	return res.Listeners[0], nil
}

func (a *AWS) createListener(ctx context.Context, accelerator *globalaccelerator.Accelerator, svc *corev1.Service) (*globalaccelerator.Listener, error) {
	var ports []*globalaccelerator.PortRange
	protocol := "TCP"
	for _, p := range svc.Spec.Ports {
		ports = append(ports, &globalaccelerator.PortRange{
			FromPort: aws.Int64(int64(p.Port)),
			ToPort:   aws.Int64(int64(p.Port)),
		})
		if p.Protocol != "" {
			protocol = string(p.Protocol)
		}
	}
	listenerInput := &globalaccelerator.CreateListenerInput{
		AcceleratorArn: accelerator.AcceleratorArn,
		ClientAffinity: aws.String("NONE"),
		PortRanges:     ports,
		Protocol:       aws.String(protocol),
	}
	listenerRes, err := a.ga.CreateListenerWithContext(ctx, listenerInput)
	if err != nil {
		return nil, err
	}
	klog.Infof("Listener is created: %s", *listenerRes.Listener.ListenerArn)
	return listenerRes.Listener, nil
}

func (a *AWS) updateListener(ctx context.Context, listener *globalaccelerator.Listener, svc *corev1.Service) (*globalaccelerator.Listener, error) {
	var ports []*globalaccelerator.PortRange
	protocol := "TCP"
	for _, p := range svc.Spec.Ports {
		ports = append(ports, &globalaccelerator.PortRange{
			FromPort: aws.Int64(int64(p.Port)),
			ToPort:   aws.Int64(int64(p.Port)),
		})
		if p.Protocol != "" {
			protocol = string(p.Protocol)
		}
	}
	input := &globalaccelerator.UpdateListenerInput{
		ClientAffinity: aws.String("NONE"),
		ListenerArn:    listener.ListenerArn,
		PortRanges:     ports,
		Protocol:       aws.String(protocol),
	}
	res, err := a.ga.UpdateListenerWithContext(ctx, input)
	if err != nil {
		return nil, err
	}
	klog.Infof("Listener is updated: %s", *res.Listener.ListenerArn)
	return res.Listener, nil
}

//---------------------------------
// EndpointGroup methods
//---------------------------------
func (a *AWS) getEndpointGroup(ctx context.Context, listenerArn string) (*globalaccelerator.EndpointGroup, error) {
	input := &globalaccelerator.ListEndpointGroupsInput{
		ListenerArn: aws.String(listenerArn),
		MaxResults:  aws.Int64(100),
	}
	res, err := a.ga.ListEndpointGroupsWithContext(ctx, input)
	if err != nil {
		return nil, err
	}
	if len(res.EndpointGroups) <= 0 {
		return nil, &globalaccelerator.EndpointGroupNotFoundException{}
	} else if len(res.EndpointGroups) > 1 {
		return nil, errors.New("Too many endpoint groups")
	}
	return res.EndpointGroups[0], nil
}

func (a *AWS) createEndpointGroup(ctx context.Context, listener *globalaccelerator.Listener, lbArn *string, region string) (*globalaccelerator.EndpointGroup, error) {
	endpointInput := &globalaccelerator.CreateEndpointGroupInput{
		EndpointConfigurations: []*globalaccelerator.EndpointConfiguration{
			&globalaccelerator.EndpointConfiguration{
				EndpointId: lbArn,
			},
		},
		EndpointGroupRegion: aws.String(region),
		ListenerArn:         listener.ListenerArn,
	}
	endpointRes, err := a.ga.CreateEndpointGroupWithContext(ctx, endpointInput)
	if err != nil {
		return nil, err
	}
	klog.Infof("EndpointGroup is created: %s", *endpointRes.EndpointGroup.EndpointGroupArn)
	return endpointRes.EndpointGroup, nil
}

func (a *AWS) updateEndpointGroup(ctx context.Context, endpoint *globalaccelerator.EndpointGroup, lbArn *string) (*globalaccelerator.EndpointGroup, error) {
	input := &globalaccelerator.UpdateEndpointGroupInput{
		EndpointConfigurations: []*globalaccelerator.EndpointConfiguration{
			&globalaccelerator.EndpointConfiguration{
				EndpointId: lbArn,
			},
		},
		EndpointGroupArn: endpoint.EndpointGroupArn,
	}
	res, err := a.ga.UpdateEndpointGroupWithContext(ctx, input)
	if err != nil {
		return nil, err
	}
	klog.Infof("EndpointGroup is updated: %s", *res.EndpointGroup.EndpointGroupArn)
	return res.EndpointGroup, nil
}

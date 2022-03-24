package aws

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/elbv2"
	"github.com/aws/aws-sdk-go/service/globalaccelerator"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/klog/v2"
)

const (
	globalAcceleratorManagedTagKey     = "aws-global-accelerator-controller-managed"
	globalAcceleratorOwnerTagKey       = "aws-global-accelerator-owner"
	globalAcceleratorTargetHostnameKey = "aws-global-accelerator-target-hostname"
)

func acceleratorOwnerTagValue(resource, ns, name string) string {
	return resource + "/" + ns + "/" + name
}

func (a *AWS) ListGlobalAcceleratorByHostname(ctx context.Context, hostname, resource, ns, name string) ([]*globalaccelerator.Accelerator, error) {
	accelerators, err := a.listAccelerator(ctx)
	if err != nil {
		klog.Error(err)
		return nil, err
	}
	res := []*globalaccelerator.Accelerator{}
	for _, accelerator := range accelerators {
		tags, err := a.listTagsForAccelerator(ctx, *accelerator.AcceleratorArn)
		if err != nil {
			return nil, err
		}
		if tagsContainsAllValues(tags, map[string]string{
			globalAcceleratorManagedTagKey:     "true",
			globalAcceleratorOwnerTagKey:       acceleratorOwnerTagValue(resource, ns, name),
			globalAcceleratorTargetHostnameKey: hostname,
		}) {
			res = append(res, accelerator)
		}
	}
	return res, nil
}

func (a *AWS) ListGlobalAcceleratorByResource(ctx context.Context, resource, ns, name string) ([]*globalaccelerator.Accelerator, error) {
	accelerators, err := a.listAccelerator(ctx)
	if err != nil {
		klog.Error(err)
		return nil, err
	}
	res := []*globalaccelerator.Accelerator{}
	for _, accelerator := range accelerators {
		tags, err := a.listTagsForAccelerator(ctx, *accelerator.AcceleratorArn)
		if err != nil {
			return nil, err
		}
		if tagsContainsAllValues(tags, map[string]string{
			globalAcceleratorManagedTagKey: "true",
			globalAcceleratorOwnerTagKey:   acceleratorOwnerTagValue(resource, ns, name),
		}) {
			res = append(res, accelerator)
		}
	}
	return res, nil
}

func (a *AWS) EnsureGlobalAcceleratorForService(
	ctx context.Context,
	svc *corev1.Service,
	lbIngress *corev1.LoadBalancerIngress,
	lbName, region string,
) (*string, bool, time.Duration, error) {
	lb, err := a.GetLoadBalancer(ctx, lbName)
	if err != nil {
		return nil, false, 0, err
	}
	if *lb.DNSName != lbIngress.Hostname {
		return nil, false, 0, fmt.Errorf("LoadBalancer's DNS name is not matched: %s", *lb.DNSName)
	}
	if *lb.State.Code != elbv2.LoadBalancerStateEnumActive {
		klog.Warningf("LoadBalancer %s is not Active: %s", *lb.LoadBalancerArn, *lb.State.Code)
		return nil, false, 30 * time.Second, nil
	}

	klog.Infof("LoadBalancer is %s", *lb.LoadBalancerArn)

	accelerators, err := a.ListGlobalAcceleratorByResource(ctx, "service", svc.Namespace, svc.Name)
	if err != nil {
		return nil, false, 0, err
	}
	if len(accelerators) == 0 {
		// Create Global Accelerator
		klog.Infof("Creating Global Accelerator for %s", *lb.DNSName)
		createdArn, err := a.createGlobalAcceleratorForService(ctx, lb, svc, region)
		if err != nil {
			klog.Error(err)
			if createdArn != nil {
				klog.Warningf("Failed to create Global Accelerator, but some resources are created, so cleanup %s", *createdArn)
				a.CleanupGlobalAccelerator(ctx, *createdArn)
			}
			return nil, false, 0, err
		}
		return createdArn, true, 0, nil
	}
	for _, accelerator := range accelerators {
		// Update Global Accelerator
		klog.Infof("Updating existing Global Accelerator %s", *accelerator.AcceleratorArn)
		if err := a.updateGlobalAcceleratorForService(ctx, accelerator, lb, svc, region); err != nil {
			return nil, false, 0, err
		}
	}
	return accelerators[0].AcceleratorArn, false, 0, nil
}

func (a *AWS) EnsureGlobalAcceleratorForIngress(
	ctx context.Context,
	ingress *networkingv1.Ingress,
	lbIngress *corev1.LoadBalancerIngress,
	lbName, region string,
) (*string, bool, time.Duration, error) {
	lb, err := a.GetLoadBalancer(ctx, lbName)
	if err != nil {
		klog.Error(err)
		return nil, false, 0, err
	}
	if *lb.DNSName != lbIngress.Hostname {
		err := fmt.Errorf("LoadBalancer's DNS name is not matched: %s", *lb.DNSName)
		klog.Error(err)
		return nil, false, 0, err
	}
	if *lb.State.Code != elbv2.LoadBalancerStateEnumActive {
		klog.Warningf("LoadBalancer %s is not Active: %s", *lb.LoadBalancerArn, *lb.State.Code)
		return nil, false, 30 * time.Second, nil
	}

	klog.Infof("LoadBalancer is %s", *lb.LoadBalancerArn)

	accelerators, err := a.ListGlobalAcceleratorByResource(ctx, "ingress", ingress.Namespace, ingress.Name)
	if err != nil {
		return nil, false, 0, err
	}
	if len(accelerators) == 0 {
		// Create Global Accelerator
		klog.Infof("Creating Global Accelerator for %s", *lb.DNSName)
		createdArn, err := a.createGlobalAcceleratorForIngress(ctx, lb, ingress, region)
		if err != nil {
			klog.Error(err)
			if createdArn != nil {
				klog.Warningf("Failed to create Global Accelerator, but some resources are created, so cleanup %s", *createdArn)
				a.CleanupGlobalAccelerator(ctx, *createdArn)
			}
			return nil, false, 0, err
		}
		return createdArn, true, 0, nil
	}

	for _, accelerator := range accelerators {
		// Update Global Accelerator
		klog.Infof("Updating existing Global Accelerator %s", *accelerator.AcceleratorArn)
		if err := a.updateGlobalAcceleratorForIngress(ctx, accelerator, lb, ingress, region); err != nil {
			klog.Error(err)
			return nil, false, 0, err
		}
	}
	return accelerators[0].AcceleratorArn, false, 0, nil
}

func (a *AWS) createGlobalAcceleratorForService(ctx context.Context, lb *elbv2.LoadBalancer, svc *corev1.Service, region string) (*string, error) {
	accelerator, err := a.createAccelerator(ctx, "service"+"-"+svc.Namespace+"-"+svc.Name, acceleratorOwnerTagValue("service", svc.Namespace, svc.Name), *lb.DNSName)
	if err != nil {
		return nil, err
	}

	ports, protocol := listenerForService(svc)
	listener, err := a.createListener(ctx, accelerator, ports, protocol)
	if err != nil {
		return accelerator.AcceleratorArn, err
	}
	_, err = a.createEndpointGroup(ctx, listener, lb.LoadBalancerArn, region)
	if err != nil {
		return accelerator.AcceleratorArn, err
	}

	return accelerator.AcceleratorArn, nil
}

func (a *AWS) createGlobalAcceleratorForIngress(ctx context.Context, lb *elbv2.LoadBalancer, ingress *networkingv1.Ingress, region string) (*string, error) {
	accelerator, err := a.createAccelerator(ctx, "ingress"+"-"+ingress.Namespace+"-"+ingress.Name, acceleratorOwnerTagValue("ingress", ingress.Namespace, ingress.Name), *lb.DNSName)
	if err != nil {
		return nil, err
	}
	ports, protocol := listenerForIngress(ingress)
	listener, err := a.createListener(ctx, accelerator, ports, protocol)
	if err != nil {
		return accelerator.AcceleratorArn, nil
	}
	_, err = a.createEndpointGroup(ctx, listener, lb.LoadBalancerArn, region)
	if err != nil {
		return accelerator.AcceleratorArn, err
	}

	return accelerator.AcceleratorArn, nil
}

func (a *AWS) CleanupGlobalAccelerator(ctx context.Context, arn string) error {
	accelerator, listener, endpoint := a.listRelatedGlobalAccelerator(ctx, arn)
	if endpoint != nil {
		if err := a.deleteEndpointGroup(ctx, *endpoint.EndpointGroupArn); err != nil {
			return err
		}
	}
	if listener != nil {
		if err := a.deleteListener(ctx, *listener.ListenerArn); err != nil {
			return err
		}
	}
	if accelerator != nil {
		if err := a.deleteAccelerator(ctx, *accelerator.AcceleratorArn); err != nil {
			return err
		}
	}
	return nil
}

func (a *AWS) listRelatedGlobalAccelerator(ctx context.Context, arn string) (*globalaccelerator.Accelerator, *globalaccelerator.Listener, *globalaccelerator.EndpointGroup) {
	accelerator, err := a.getAccelerator(ctx, arn)
	if err != nil {
		return nil, nil, nil
	}
	listener, err := a.GetListener(ctx, *accelerator.AcceleratorArn)
	if err != nil {
		return accelerator, nil, nil
	}
	endpoint, err := a.GetEndpointGroup(ctx, *listener.ListenerArn)
	if err != nil {
		return accelerator, listener, nil
	}
	return accelerator, listener, endpoint
}

func (a *AWS) updateGlobalAcceleratorForService(ctx context.Context, accelerator *globalaccelerator.Accelerator, lb *elbv2.LoadBalancer, svc *corev1.Service, region string) error {
	if a.acceleratorChanged(ctx, accelerator, *lb.DNSName, "service", svc) {
		if _, err := a.updateAccelerator(ctx, accelerator.AcceleratorArn, "service-"+svc.Namespace+"-"+svc.Name, acceleratorOwnerTagValue("service", svc.Namespace, svc.Name), *lb.DNSName); err != nil {
			klog.Error(err)
			return err
		}
	}

	listener, err := a.GetListener(ctx, *accelerator.AcceleratorArn)
	if err != nil {
		var notFoundErr *globalaccelerator.ListenerNotFoundException
		if errors.As(err, &notFoundErr) {
			ports, protocol := listenerForService(svc)
			listener, err = a.createListener(ctx, accelerator, ports, protocol)
			if err != nil {
				klog.Error(err)
				return err
			}
		} else {
			klog.Error(err)
			return err
		}
	}
	if listenerProtocolChangedFromService(listener, svc) || listenerPortChangedFromService(listener, svc) {
		klog.Infof("Listener is changed, so updating: %s", *listener.ListenerArn)
		ports, protocol := listenerForService(svc)
		listener, err = a.updateListener(ctx, listener, ports, protocol)
		if err != nil {
			klog.Error(err)
			return err
		}
	}
	endpoint, err := a.GetEndpointGroup(ctx, *listener.ListenerArn)
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
		klog.Infof("Endpoint Group is changed, so updating: %s", *endpoint.EndpointGroupArn)
		endpoint, err = a.updateEndpointGroup(ctx, endpoint, lb.LoadBalancerArn)
		if err != nil {
			klog.Error(err)
			return err
		}
	}

	klog.Infof("All resources are synced: %s", *accelerator.AcceleratorArn)
	return nil
}

func (a *AWS) updateGlobalAcceleratorForIngress(ctx context.Context, accelerator *globalaccelerator.Accelerator, lb *elbv2.LoadBalancer, ingress *networkingv1.Ingress, region string) error {
	if a.acceleratorChanged(ctx, accelerator, *lb.DNSName, "ingress", ingress) {
		if _, err := a.updateAccelerator(ctx, accelerator.AcceleratorArn, "ingress-"+ingress.Namespace+"-"+ingress.Name, acceleratorOwnerTagValue("ingress", ingress.Namespace, ingress.Name), *lb.DNSName); err != nil {
			klog.Error(err)
			return err
		}
	}

	listener, err := a.GetListener(ctx, *accelerator.AcceleratorArn)
	if err != nil {
		var notFoundErr *globalaccelerator.ListenerNotFoundException
		if errors.As(err, &notFoundErr) {
			ports, protocol := listenerForIngress(ingress)
			listener, err = a.createListener(ctx, accelerator, ports, protocol)
			if err != nil {
				klog.Error(err)
				return err
			}
		} else {
			klog.Error(err)
			return err
		}
	}
	if listenerProtocolChangedFromIngress(listener, ingress) || listenerPortChangedFromIngress(listener, ingress) {
		klog.Infof("Listener is changed, so updating: %s", *listener.ListenerArn)
		ports, protocol := listenerForIngress(ingress)
		listener, err = a.updateListener(ctx, listener, ports, protocol)
		if err != nil {
			klog.Error(err)
			return err
		}
	}
	endpoint, err := a.GetEndpointGroup(ctx, *listener.ListenerArn)
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
		klog.Infof("Endpoint Group is changed, so updating: %s", *endpoint.EndpointGroupArn)
		endpoint, err = a.updateEndpointGroup(ctx, endpoint, lb.LoadBalancerArn)
		if err != nil {
			klog.Error(err)
			return err
		}
	}

	klog.Infof("All resources are synced: %s", *accelerator.AcceleratorArn)
	return nil
}

func (a *AWS) acceleratorChanged(ctx context.Context, accelerator *globalaccelerator.Accelerator, hostname, resource string, obj metav1.Object) bool {
	if *accelerator.Enabled == false {
		return true
	}
	acceleratorName := resource + "-" + obj.GetNamespace() + "-" + obj.GetName()
	if *accelerator.Name != acceleratorName {
		return true
	}
	tags, err := a.listTagsForAccelerator(ctx, *accelerator.AcceleratorArn)
	if err != nil {
		klog.Warning(err)
		return false
	}
	if !tagsContainsAllValues(tags, map[string]string{
		globalAcceleratorManagedTagKey:     "true",
		globalAcceleratorOwnerTagKey:       acceleratorOwnerTagValue(resource, obj.GetNamespace(), obj.GetName()),
		globalAcceleratorTargetHostnameKey: hostname,
	}) {
		return true
	}
	return false

}

func listenerProtocolChangedFromService(listener *globalaccelerator.Listener, svc *corev1.Service) bool {
	protocol := "TCP"
	for _, p := range svc.Spec.Ports {
		if p.Protocol != "" {
			protocol = string(p.Protocol)
		}
	}
	return *listener.Protocol != protocol

}

func listenerProtocolChangedFromIngress(listener *globalaccelerator.Listener, ingress *networkingv1.Ingress) bool {
	// Ingress(= ALB) allows only HTTP or TCP, it does not allow UDP.
	// And Global Accelerator Listener is TCP or UDP. So if the listener protocol is not TCP, it is not allowed.
	return *listener.Protocol != "TCP"
}

func listenerPortChangedFromService(listener *globalaccelerator.Listener, svc *corev1.Service) bool {
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

func listenerPortChangedFromIngress(listener *globalaccelerator.Listener, ingress *networkingv1.Ingress) bool {
	portCount := make(map[int]int)
	for _, p := range listener.PortRanges {
		portCount[int(*p.FromPort)]++
	}
	ports, _ := listenerForIngress(ingress)
	for _, p := range ports {
		portCount[int(p)]++
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

func listenerForService(svc *corev1.Service) ([]int64, string) {
	ports := []int64{}
	protocol := "TCP"
	for _, p := range svc.Spec.Ports {
		ports = append(ports, int64(p.Port))
		if p.Protocol != "" {
			protocol = string(p.Protocol)
		}
	}
	return ports, protocol
}

func listenerForIngress(ingress *networkingv1.Ingress) ([]int64, string) {
	ports := []int64{}
	protocol := "TCP"
	if ingress.Spec.DefaultBackend != nil && ingress.Spec.DefaultBackend.Service != nil {
		ports = append(ports, int64(ingress.Spec.DefaultBackend.Service.Port.Number))
	}
	for _, rule := range ingress.Spec.Rules {
		if rule.HTTP != nil {
			for _, path := range rule.HTTP.Paths {
				if path.Backend.Service != nil {
					ports = append(ports, int64(path.Backend.Service.Port.Number))
				}
			}
		}
	}
	return ports, protocol
}

func tagsContainsAllValues(tags []*globalaccelerator.Tag, targetTags map[string]string) bool {
	actual := map[string]string{}
	for _, t := range tags {
		actual[*t.Key] = *t.Value
	}
	for key, value := range targetTags {
		if actual[key] != value {
			return false
		}
	}
	return true
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

func (a *AWS) listAccelerator(ctx context.Context) ([]*globalaccelerator.Accelerator, error) {
	input := &globalaccelerator.ListAcceleratorsInput{
		MaxResults: aws.Int64(100),
	}
	res, err := a.ga.ListAcceleratorsWithContext(ctx, input)
	if err != nil {
		return nil, err
	}
	return res.Accelerators, nil
}

func (a *AWS) listTagsForAccelerator(ctx context.Context, arn string) ([]*globalaccelerator.Tag, error) {
	input := &globalaccelerator.ListTagsForResourceInput{
		ResourceArn: aws.String(arn),
	}
	res, err := a.ga.ListTagsForResourceWithContext(ctx, input)
	if err != nil {
		return nil, err
	}
	return res.Tags, nil
}

func (a *AWS) createAccelerator(ctx context.Context, name string, owner string, hostname string) (*globalaccelerator.Accelerator, error) {
	klog.Infof("Creating Global Accelerator %s", name)
	acceleratorInput := &globalaccelerator.CreateAcceleratorInput{
		Enabled:       aws.Bool(true),
		IpAddressType: aws.String("IPV4"),
		Name:          aws.String(name),
		Tags: []*globalaccelerator.Tag{
			&globalaccelerator.Tag{
				Key:   aws.String(globalAcceleratorManagedTagKey),
				Value: aws.String("true"),
			},
			&globalaccelerator.Tag{
				Key:   aws.String(globalAcceleratorOwnerTagKey),
				Value: aws.String(owner),
			},
			&globalaccelerator.Tag{
				Key:   aws.String(globalAcceleratorTargetHostnameKey),
				Value: aws.String(hostname),
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

func (a *AWS) updateAccelerator(ctx context.Context, arn *string, name, owner, hostname string) (*globalaccelerator.Accelerator, error) {
	klog.Infof("Updating Global Accelerator %s", arn)
	updateInput := &globalaccelerator.UpdateAcceleratorInput{
		AcceleratorArn: arn,
		Enabled:        aws.Bool(true),
		Name:           aws.String(name),
	}
	updated, err := a.ga.UpdateAcceleratorWithContext(ctx, updateInput)
	if err != nil {
		klog.Error(err)
		return nil, err
	}
	tagInput := &globalaccelerator.TagResourceInput{
		ResourceArn: arn,
		Tags: []*globalaccelerator.Tag{
			&globalaccelerator.Tag{
				Key:   aws.String(globalAcceleratorManagedTagKey),
				Value: aws.String("true"),
			},
			&globalaccelerator.Tag{
				Key:   aws.String(globalAcceleratorOwnerTagKey),
				Value: aws.String(owner),
			},
			&globalaccelerator.Tag{
				Key:   aws.String(globalAcceleratorTargetHostnameKey),
				Value: aws.String(hostname),
			},
		},
	}
	_, err = a.ga.TagResourceWithContext(ctx, tagInput)
	if err != nil {
		klog.Error(err)
		return nil, err
	}

	return updated.Accelerator, nil
}

func (a *AWS) deleteAccelerator(ctx context.Context, arn string) error {
	klog.Infof("Disabling Global Accelerator %s", arn)
	updateInput := &globalaccelerator.UpdateAcceleratorInput{
		AcceleratorArn: aws.String(arn),
		Enabled:        aws.Bool(false),
	}
	_, err := a.ga.UpdateAcceleratorWithContext(ctx, updateInput)
	if err != nil {
		klog.Error(err)
		return err
	}

	// Wait until status is synced
	err = wait.Poll(10*time.Second, 3*time.Minute, func() (bool, error) {
		accelerator, err := a.getAccelerator(ctx, arn)
		if err != nil {
			return false, err
		}

		if *accelerator.Status == globalaccelerator.AcceleratorStatusDeployed {
			klog.Infof("Global Accelerator %s is %s", *accelerator.AcceleratorArn, *accelerator.Status)
			return true, nil
		}
		klog.Infof("Global Accelerator %s is %s, so waiting", *accelerator.AcceleratorArn, *accelerator.Status)
		return false, nil
	})
	if err != nil {
		klog.Error(err)
		return err
	}

	input := &globalaccelerator.DeleteAcceleratorInput{
		AcceleratorArn: aws.String(arn),
	}
	_, err = a.ga.DeleteAcceleratorWithContext(ctx, input)
	if err != nil {
		klog.Error(err)
		return err
	}
	klog.Infof("Global Accelerator is deleted: %s", arn)
	return nil
}

//---------------------------------
// Lstener methods
//---------------------------------
func (a *AWS) GetListener(ctx context.Context, acceleratorArn string) (*globalaccelerator.Listener, error) {
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

func (a *AWS) createListener(ctx context.Context, accelerator *globalaccelerator.Accelerator, ports []int64, protocol string) (*globalaccelerator.Listener, error) {
	var portRange []*globalaccelerator.PortRange
	for _, p := range ports {
		portRange = append(portRange, &globalaccelerator.PortRange{
			FromPort: aws.Int64(p),
			ToPort:   aws.Int64(p),
		})
	}
	listenerInput := &globalaccelerator.CreateListenerInput{
		AcceleratorArn: accelerator.AcceleratorArn,
		ClientAffinity: aws.String("NONE"),
		PortRanges:     portRange,
		Protocol:       aws.String(protocol),
	}
	listenerRes, err := a.ga.CreateListenerWithContext(ctx, listenerInput)
	if err != nil {
		return nil, err
	}
	klog.Infof("Listener is created: %s", *listenerRes.Listener.ListenerArn)
	return listenerRes.Listener, nil
}

func (a *AWS) updateListener(ctx context.Context, listener *globalaccelerator.Listener, ports []int64, protocol string) (*globalaccelerator.Listener, error) {
	var portRange []*globalaccelerator.PortRange
	for _, p := range ports {
		portRange = append(portRange, &globalaccelerator.PortRange{
			FromPort: aws.Int64(p),
			ToPort:   aws.Int64(p),
		})
	}
	input := &globalaccelerator.UpdateListenerInput{
		ClientAffinity: aws.String("NONE"),
		ListenerArn:    listener.ListenerArn,
		PortRanges:     portRange,
		Protocol:       aws.String(protocol),
	}
	res, err := a.ga.UpdateListenerWithContext(ctx, input)
	if err != nil {
		return nil, err
	}
	klog.Infof("Listener is updated: %s", *res.Listener.ListenerArn)
	return res.Listener, nil
}

func (a *AWS) deleteListener(ctx context.Context, arn string) error {
	input := &globalaccelerator.DeleteListenerInput{
		ListenerArn: aws.String(arn),
	}
	_, err := a.ga.DeleteListenerWithContext(ctx, input)
	if err != nil {
		return err
	}
	klog.Infof("Listener is deleted: %s", arn)
	return nil
}

//---------------------------------
// EndpointGroup methods
//---------------------------------
func (a *AWS) GetEndpointGroup(ctx context.Context, listenerArn string) (*globalaccelerator.EndpointGroup, error) {
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

func (a *AWS) deleteEndpointGroup(ctx context.Context, arn string) error {
	input := &globalaccelerator.DeleteEndpointGroupInput{
		EndpointGroupArn: aws.String(arn),
	}
	_, err := a.ga.DeleteEndpointGroupWithContext(ctx, input)
	if err != nil {
		return err
	}
	klog.Infof("EndpointGroup is deleted: %s", arn)
	return nil
}

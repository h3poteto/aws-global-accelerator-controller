package aws

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	elbv2types "github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2/types"
	"github.com/aws/aws-sdk-go-v2/service/globalaccelerator"
	gatypes "github.com/aws/aws-sdk-go-v2/service/globalaccelerator/types"
	"github.com/h3poteto/aws-global-accelerator-controller/pkg/apis"
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
	globalAcceleratorClusterTagKey     = "aws-global-accelerator-cluster"
)

func acceleratorOwnerTagValue(resource, ns, name string) string {
	return resource + "/" + ns + "/" + name
}

func acceleratorName(resource string, obj metav1.Object) string {
	return resource + "-" + obj.GetNamespace() + "-" + obj.GetName()
}

func (a *AWS) ListGlobalAcceleratorByHostname(ctx context.Context, hostname, clusterName string) ([]*gatypes.Accelerator, error) {
	accelerators, err := a.listAccelerator(ctx)
	if err != nil {
		klog.Error(err)
		return nil, err
	}
	res := []*gatypes.Accelerator{}
	for _, accelerator := range accelerators {
		tags, err := a.listTagsForAccelerator(ctx, *accelerator.AcceleratorArn)
		if err != nil {
			return nil, err
		}
		if tagsContainsAllValues(tags, map[string]string{
			globalAcceleratorManagedTagKey:     "true",
			globalAcceleratorTargetHostnameKey: hostname,
			globalAcceleratorClusterTagKey:     clusterName,
		}) {
			res = append(res, accelerator)
		} else {
			klog.V(4).Infof("Global Accelerator %s does not have match tags", *accelerator.AcceleratorArn)
		}
	}
	return res, nil
}

func (a *AWS) ListGlobalAcceleratorByResource(ctx context.Context, clusterName, resource, ns, name string) ([]*gatypes.Accelerator, error) {
	accelerators, err := a.listAccelerator(ctx)
	if err != nil {
		klog.Error(err)
		return nil, err
	}
	res := []*gatypes.Accelerator{}
	for _, accelerator := range accelerators {
		tags, err := a.listTagsForAccelerator(ctx, *accelerator.AcceleratorArn)
		if err != nil {
			return nil, err
		}
		if tagsContainsAllValues(tags, map[string]string{
			globalAcceleratorManagedTagKey: "true",
			globalAcceleratorOwnerTagKey:   acceleratorOwnerTagValue(resource, ns, name),
			globalAcceleratorClusterTagKey: clusterName,
		}) {
			res = append(res, accelerator)
		} else {
			klog.V(4).Infof("Global Accelerator %s does not have match tags", *accelerator.AcceleratorArn)
		}
	}
	return res, nil
}

func (a *AWS) EnsureGlobalAcceleratorForService(
	ctx context.Context,
	svc *corev1.Service,
	lbIngress *corev1.LoadBalancerIngress,
	clusterName, lbName, region string,
) (*string, bool, time.Duration, error) {
	lb, err := a.GetLoadBalancer(ctx, lbName)
	if err != nil {
		return nil, false, 0, err
	}
	if *lb.DNSName != lbIngress.Hostname {
		return nil, false, 0, fmt.Errorf("LoadBalancer's DNS name is not matched: %s", *lb.DNSName)
	}
	if lb.State.Code != elbv2types.LoadBalancerStateEnumActive {
		klog.Warningf("LoadBalancer %s is not Active: %s", *lb.LoadBalancerArn, lb.State.Code)
		return nil, false, 30 * time.Second, nil
	}

	klog.Infof("LoadBalancer is %s", *lb.LoadBalancerArn)

	accelerators, err := a.ListGlobalAcceleratorByResource(ctx, clusterName, "service", svc.Namespace, svc.Name)
	if err != nil {
		return nil, false, 0, err
	}
	if len(accelerators) == 0 {
		// Create Global Accelerator
		klog.Infof("Creating Global Accelerator for %s", *lb.DNSName)
		createdArn, err := a.createGlobalAcceleratorForService(ctx, lb, svc, clusterName, region)
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
	lbIngress *networkingv1.IngressLoadBalancerIngress,
	clusterName, lbName, region string,
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
	if lb.State.Code != elbv2types.LoadBalancerStateEnumActive {
		klog.Warningf("LoadBalancer %s is not Active: %s", *lb.LoadBalancerArn, lb.State.Code)
		return nil, false, 30 * time.Second, nil
	}

	klog.Infof("LoadBalancer is %s", *lb.LoadBalancerArn)

	accelerators, err := a.ListGlobalAcceleratorByResource(ctx, clusterName, "ingress", ingress.Namespace, ingress.Name)
	if err != nil {
		return nil, false, 0, err
	}
	if len(accelerators) == 0 {
		// Create Global Accelerator
		klog.Infof("Creating Global Accelerator for %s", *lb.DNSName)
		createdArn, err := a.createGlobalAcceleratorForIngress(ctx, lb, ingress, clusterName, region)
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

func (a *AWS) createGlobalAcceleratorForService(ctx context.Context, lb *elbv2types.LoadBalancer, svc *corev1.Service, clusterName, region string) (*string, error) {
	accelerator, err := a.createAccelerator(ctx, acceleratorName("service", svc), clusterName, acceleratorOwnerTagValue("service", svc.Namespace, svc.Name), *lb.DNSName)
	if err != nil {
		return nil, err
	}

	ports, protocol := listenerForService(svc)
	listener, err := a.createListener(ctx, accelerator, ports, protocol)
	if err != nil {
		return accelerator.AcceleratorArn, err
	}
	ipPreserve := svc.Annotations[apis.ClientIPPreservationAnnotation] == "true"
	_, err = a.createEndpointGroup(ctx, listener, lb.LoadBalancerArn, region, ipPreserve)
	if err != nil {
		return accelerator.AcceleratorArn, err
	}

	return accelerator.AcceleratorArn, nil
}

func (a *AWS) createGlobalAcceleratorForIngress(ctx context.Context, lb *elbv2types.LoadBalancer, ingress *networkingv1.Ingress, clusterName, region string) (*string, error) {
	accelerator, err := a.createAccelerator(ctx, acceleratorName("ingress", ingress), clusterName, acceleratorOwnerTagValue("ingress", ingress.Namespace, ingress.Name), *lb.DNSName)
	if err != nil {
		return nil, err
	}
	ports, protocol := listenerForIngress(ingress)
	listener, err := a.createListener(ctx, accelerator, ports, protocol)
	if err != nil {
		return accelerator.AcceleratorArn, nil
	}
	ipPreserve := ingress.Annotations[apis.ClientIPPreservationAnnotation] == "true"
	_, err = a.createEndpointGroup(ctx, listener, lb.LoadBalancerArn, region, ipPreserve)
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

func (a *AWS) listRelatedGlobalAccelerator(ctx context.Context, arn string) (*gatypes.Accelerator, *gatypes.Listener, *gatypes.EndpointGroup) {
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

func (a *AWS) updateGlobalAcceleratorForService(ctx context.Context, accelerator *gatypes.Accelerator, lb *elbv2types.LoadBalancer, svc *corev1.Service, region string) error {
	if a.acceleratorChanged(ctx, accelerator, *lb.DNSName, "service", svc) {
		if _, err := a.updateAccelerator(ctx, accelerator.AcceleratorArn, acceleratorName("service", svc), acceleratorOwnerTagValue("service", svc.Namespace, svc.Name), *lb.DNSName); err != nil {
			klog.Error(err)
			return err
		}
	}

	listener, err := a.GetListener(ctx, *accelerator.AcceleratorArn)
	if err != nil {
		var notFoundErr *gatypes.ListenerNotFoundException
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
		var notFoundErr *gatypes.EndpointGroupNotFoundException
		if errors.As(err, &notFoundErr) {
			ipPreserve := svc.Annotations[apis.ClientIPPreservationAnnotation] == "true"
			endpoint, err = a.createEndpointGroup(ctx, listener, lb.LoadBalancerArn, region, ipPreserve)
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
		ipPreserve := svc.Annotations[apis.ClientIPPreservationAnnotation] == "true"
		endpoint, err = a.updateEndpointGroup(ctx, endpoint, lb.LoadBalancerArn, ipPreserve)
		if err != nil {
			klog.Error(err)
			return err
		}
	}

	klog.Infof("All resources are synced: %s", *accelerator.AcceleratorArn)
	return nil
}

func (a *AWS) updateGlobalAcceleratorForIngress(ctx context.Context, accelerator *gatypes.Accelerator, lb *elbv2types.LoadBalancer, ingress *networkingv1.Ingress, region string) error {
	if a.acceleratorChanged(ctx, accelerator, *lb.DNSName, "ingress", ingress) {
		if _, err := a.updateAccelerator(ctx, accelerator.AcceleratorArn, acceleratorName("ingress", ingress), acceleratorOwnerTagValue("ingress", ingress.Namespace, ingress.Name), *lb.DNSName); err != nil {
			klog.Error(err)
			return err
		}
	}

	listener, err := a.GetListener(ctx, *accelerator.AcceleratorArn)
	if err != nil {
		var notFoundErr *gatypes.ListenerNotFoundException
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
		var notFoundErr *gatypes.EndpointGroupNotFoundException
		if errors.As(err, &notFoundErr) {
			ipPreserve := ingress.Annotations[apis.ClientIPPreservationAnnotation] == "true"
			endpoint, err = a.createEndpointGroup(ctx, listener, lb.LoadBalancerArn, region, ipPreserve)
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
		ipPreserve := ingress.Annotations[apis.ClientIPPreservationAnnotation] == "true"
		endpoint, err = a.updateEndpointGroup(ctx, endpoint, lb.LoadBalancerArn, ipPreserve)
		if err != nil {
			klog.Error(err)
			return err
		}
	}

	klog.Infof("All resources are synced: %s", *accelerator.AcceleratorArn)
	return nil
}

func (a *AWS) acceleratorChanged(ctx context.Context, accelerator *gatypes.Accelerator, hostname, resource string, obj metav1.Object) bool {
	if *accelerator.Enabled == false {
		return true
	}

	if *accelerator.Name != acceleratorName(resource, obj) {
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

func listenerProtocolChangedFromService(listener *gatypes.Listener, svc *corev1.Service) bool {
	protocol := gatypes.ProtocolTcp
	for _, p := range svc.Spec.Ports {
		if strings.ToLower(string(p.Protocol)) == "udp" {
			protocol = gatypes.ProtocolUdp
		} else if strings.ToLower(string(p.Protocol)) == "tcp" {
			protocol = gatypes.ProtocolTcp
		}
	}

	return listener.Protocol != protocol
}

func listenerProtocolChangedFromIngress(listener *gatypes.Listener, ingress *networkingv1.Ingress) bool {
	// Ingress(= ALB) allows only HTTP or TCP, it does not allow UDP.
	// And Global Accelerator Listener is TCP or UDP. So if the listener protocol is not TCP, it is not allowed.
	return listener.Protocol != gatypes.ProtocolTcp
}

func listenerPortChangedFromService(listener *gatypes.Listener, svc *corev1.Service) bool {
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

func listenerPortChangedFromIngress(listener *gatypes.Listener, ingress *networkingv1.Ingress) bool {
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

func endpointContainsLB(endpoint *gatypes.EndpointGroup, lb *elbv2types.LoadBalancer) bool {
	for _, d := range endpoint.EndpointDescriptions {
		if *d.EndpointId == *lb.LoadBalancerArn {
			return true
		}
	}
	return false
}

func listenerForService(svc *corev1.Service) ([]int32, gatypes.Protocol) {
	ports := []int32{}
	protocol := gatypes.ProtocolTcp
	for _, p := range svc.Spec.Ports {
		ports = append(ports, int32(p.Port))
		if strings.ToLower(string(p.Protocol)) == "udp" {
			protocol = gatypes.ProtocolUdp
		} else if strings.ToLower(string(p.Protocol)) == "tcp" {
			protocol = gatypes.ProtocolTcp
		}
	}
	return ports, protocol
}

type IngressPort struct {
	HTTP  int64 `json:"HTTP,omitempty"`
	HTTPS int64 `json:"HTTPS,omitempty"`
}

func listenerForIngress(ingress *networkingv1.Ingress) ([]int32, gatypes.Protocol) {
	ports := []int32{}
	protocol := gatypes.ProtocolTcp
	// If this annotation is specified, ALB listen these ports, we can ignore ports in rules.
	if val, ok := ingress.Annotations["alb.ingress.kubernetes.io/listen-ports"]; ok {
		ingressPorts := []IngressPort{}
		err := json.Unmarshal([]byte(val), &ingressPorts)
		if err != nil {
			klog.Error(err)
			return ports, protocol
		}
		for _, i := range ingressPorts {
			if i.HTTP != 0 {
				ports = append(ports, int32(i.HTTP))
			}
			if i.HTTPS != 0 {
				ports = append(ports, int32(i.HTTPS))
			}
		}
		return ports, protocol
	}

	if ingress.Spec.DefaultBackend != nil && ingress.Spec.DefaultBackend.Service != nil {
		ports = append(ports, int32(ingress.Spec.DefaultBackend.Service.Port.Number))
	}
	for _, rule := range ingress.Spec.Rules {
		if rule.HTTP != nil {
			for _, path := range rule.HTTP.Paths {
				if path.Backend.Service != nil {
					ports = append(ports, int32(path.Backend.Service.Port.Number))
				}
			}
		}
	}
	return ports, protocol
}

func tagsContainsAllValues(tags []gatypes.Tag, targetTags map[string]string) bool {
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

func (a *AWS) AddLBToEndpointGroup(ctx context.Context, endpointGroup *gatypes.EndpointGroup, lbName string, ipPreserve bool) (*string, time.Duration, error) {
	// Describe the LB
	lb, err := a.GetLoadBalancer(ctx, lbName)
	if err != nil {
		klog.Error(err)
		return nil, 0, err
	}
	if lb.State.Code != elbv2types.LoadBalancerStateEnumActive {
		klog.Warningf("LoadBalancer %s is not Active: %s", *lb.LoadBalancerArn, lb.State.Code)
		return nil, 30 * time.Second, nil
	}

	endpointId, err := a.addEndpoint(ctx, *endpointGroup.EndpointGroupArn, *lb.LoadBalancerArn, ipPreserve)
	if err != nil {
		klog.Error(err)
		return nil, 0, err
	}
	return endpointId, 0, nil
}

func (a *AWS) RemoveLBFromEdnpointGroup(ctx context.Context, endpointGroup *gatypes.EndpointGroup, endpointId string) error {
	err := a.removeEndpoint(ctx, *endpointGroup.EndpointGroupArn, endpointId)
	if err != nil {
		klog.Error(err)
		return err
	}
	return nil
}

// ---------------------------------
// Accelerator methods
// ---------------------------------
func (a *AWS) getAccelerator(ctx context.Context, arn string) (*gatypes.Accelerator, error) {
	input := &globalaccelerator.DescribeAcceleratorInput{
		AcceleratorArn: aws.String(arn),
	}
	res, err := a.ga.DescribeAccelerator(ctx, input)
	if err != nil {
		return nil, err
	}
	return res.Accelerator, nil
}

func (a *AWS) listAccelerator(ctx context.Context) ([]*gatypes.Accelerator, error) {
	input := &globalaccelerator.ListAcceleratorsInput{
		MaxResults: aws.Int32(100),
	}
	accelerators := []*gatypes.Accelerator{}
	paginator := globalaccelerator.NewListAcceleratorsPaginator(a.ga, input)
	for paginator.HasMorePages() {
		output, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, err
		}
		for i := range output.Accelerators {
			accelerators = append(accelerators, &output.Accelerators[i])
		}
	}

	return accelerators, nil
}

func (a *AWS) listTagsForAccelerator(ctx context.Context, arn string) ([]gatypes.Tag, error) {
	input := &globalaccelerator.ListTagsForResourceInput{
		ResourceArn: aws.String(arn),
	}
	res, err := a.ga.ListTagsForResource(ctx, input)
	if err != nil {
		return nil, err
	}
	return res.Tags, nil
}

func (a *AWS) createAccelerator(ctx context.Context, name, clusterName, owner, hostname string) (*gatypes.Accelerator, error) {
	klog.Infof("Creating Global Accelerator %s", name)
	acceleratorInput := &globalaccelerator.CreateAcceleratorInput{
		Enabled:       aws.Bool(true),
		IpAddressType: gatypes.IpAddressTypeIpv4,
		Name:          aws.String(name),
		Tags: []gatypes.Tag{
			gatypes.Tag{
				Key:   aws.String(globalAcceleratorManagedTagKey),
				Value: aws.String("true"),
			},
			gatypes.Tag{
				Key:   aws.String(globalAcceleratorOwnerTagKey),
				Value: aws.String(owner),
			},
			gatypes.Tag{
				Key:   aws.String(globalAcceleratorTargetHostnameKey),
				Value: aws.String(hostname),
			},
			gatypes.Tag{
				Key:   aws.String(globalAcceleratorClusterTagKey),
				Value: aws.String(clusterName),
			},
		},
	}
	acceleratorRes, err := a.ga.CreateAccelerator(ctx, acceleratorInput)
	if err != nil {
		return nil, err
	}
	klog.Infof("Global Accelerator is created: %s", *acceleratorRes.Accelerator.AcceleratorArn)
	return acceleratorRes.Accelerator, nil
}

func (a *AWS) updateAccelerator(ctx context.Context, arn *string, name, owner, hostname string) (*gatypes.Accelerator, error) {
	klog.Infof("Updating Global Accelerator %s", *arn)
	updateInput := &globalaccelerator.UpdateAcceleratorInput{
		AcceleratorArn: arn,
		Enabled:        aws.Bool(true),
		Name:           aws.String(name),
	}
	updated, err := a.ga.UpdateAccelerator(ctx, updateInput)
	if err != nil {
		klog.Error(err)
		return nil, err
	}
	tagInput := &globalaccelerator.TagResourceInput{
		ResourceArn: arn,
		Tags: []gatypes.Tag{
			gatypes.Tag{
				Key:   aws.String(globalAcceleratorManagedTagKey),
				Value: aws.String("true"),
			},
			gatypes.Tag{
				Key:   aws.String(globalAcceleratorOwnerTagKey),
				Value: aws.String(owner),
			},
			gatypes.Tag{
				Key:   aws.String(globalAcceleratorTargetHostnameKey),
				Value: aws.String(hostname),
			},
		},
	}
	_, err = a.ga.TagResource(ctx, tagInput)
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
	_, err := a.ga.UpdateAccelerator(ctx, updateInput)
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

		if accelerator.Status == gatypes.AcceleratorStatusDeployed {
			klog.Infof("Global Accelerator %s is %s", *accelerator.AcceleratorArn, accelerator.Status)
			return true, nil
		}
		klog.Infof("Global Accelerator %s is %s, so waiting", *accelerator.AcceleratorArn, accelerator.Status)
		return false, nil
	})
	if err != nil {
		klog.Error(err)
		return err
	}

	input := &globalaccelerator.DeleteAcceleratorInput{
		AcceleratorArn: aws.String(arn),
	}
	_, err = a.ga.DeleteAccelerator(ctx, input)
	if err != nil {
		klog.Error(err)
		return err
	}
	klog.Infof("Global Accelerator is deleted: %s", arn)
	return nil
}

// ---------------------------------
// Lstener methods
// ---------------------------------
func (a *AWS) GetListener(ctx context.Context, acceleratorArn string) (*gatypes.Listener, error) {
	input := &globalaccelerator.ListListenersInput{
		AcceleratorArn: aws.String(acceleratorArn),
		MaxResults:     aws.Int32(100),
	}
	listeners := []*gatypes.Listener{}
	paginator := globalaccelerator.NewListListenersPaginator(a.ga, input)
	for paginator.HasMorePages() {
		output, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, err
		}
		for _, listener := range output.Listeners {
			listeners = append(listeners, &listener)
		}
	}

	if len(listeners) <= 0 {
		return nil, &gatypes.ListenerNotFoundException{}
	} else if len(listeners) > 1 {
		klog.V(4).Infof("Too many listeners: %+v", listeners)
		return nil, errors.New("Too many listeners")
	}
	return listeners[0], nil
}

func (a *AWS) createListener(ctx context.Context, accelerator *gatypes.Accelerator, ports []int32, protocol gatypes.Protocol) (*gatypes.Listener, error) {
	var portRange []gatypes.PortRange
	for _, p := range ports {
		portRange = append(portRange, gatypes.PortRange{
			FromPort: aws.Int32(p),
			ToPort:   aws.Int32(p),
		})
	}
	listenerInput := &globalaccelerator.CreateListenerInput{
		AcceleratorArn: accelerator.AcceleratorArn,
		ClientAffinity: gatypes.ClientAffinityNone,
		PortRanges:     portRange,
		Protocol:       protocol,
	}
	listenerRes, err := a.ga.CreateListener(ctx, listenerInput)
	if err != nil {
		return nil, err
	}
	klog.Infof("Listener is created: %s", *listenerRes.Listener.ListenerArn)
	return listenerRes.Listener, nil
}

func (a *AWS) updateListener(ctx context.Context, listener *gatypes.Listener, ports []int32, protocol gatypes.Protocol) (*gatypes.Listener, error) {
	var portRange []gatypes.PortRange
	for _, p := range ports {
		portRange = append(portRange, gatypes.PortRange{
			FromPort: aws.Int32(p),
			ToPort:   aws.Int32(p),
		})
	}
	input := &globalaccelerator.UpdateListenerInput{
		ClientAffinity: gatypes.ClientAffinityNone,
		ListenerArn:    listener.ListenerArn,
		PortRanges:     portRange,
		Protocol:       protocol,
	}
	res, err := a.ga.UpdateListener(ctx, input)
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
	_, err := a.ga.DeleteListener(ctx, input)
	if err != nil {
		return err
	}
	klog.Infof("Listener is deleted: %s", arn)
	return nil
}

// ---------------------------------
// EndpointGroup methods
// ---------------------------------
func (a *AWS) DescribeEndpointGroup(ctx context.Context, endpointGroupArn string) (*gatypes.EndpointGroup, error) {
	input := &globalaccelerator.DescribeEndpointGroupInput{
		EndpointGroupArn: aws.String(endpointGroupArn),
	}
	res, err := a.ga.DescribeEndpointGroup(ctx, input)
	if err != nil {
		return nil, err
	}
	return res.EndpointGroup, nil
}

func (a *AWS) GetEndpointGroup(ctx context.Context, listenerArn string) (*gatypes.EndpointGroup, error) {
	input := &globalaccelerator.ListEndpointGroupsInput{
		ListenerArn: aws.String(listenerArn),
		MaxResults:  aws.Int32(100),
	}
	endpointGroups := []gatypes.EndpointGroup{}
	paginator := globalaccelerator.NewListEndpointGroupsPaginator(a.ga, input)
	for paginator.HasMorePages() {
		output, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, err
		}
		endpointGroups = append(endpointGroups, output.EndpointGroups...)
	}

	if len(endpointGroups) <= 0 {
		return nil, &gatypes.EndpointGroupNotFoundException{}
	} else if len(endpointGroups) > 1 {
		klog.V(4).Infof("Too many endpoint groups: %+v", endpointGroups)
		return nil, errors.New("Too many endpoint groups")
	}
	return &endpointGroups[0], nil
}

func (a *AWS) addEndpoint(ctx context.Context, endpointGroupArn, lbArn string, ipPreserve bool) (*string, error) {
	input := &globalaccelerator.AddEndpointsInput{
		EndpointConfigurations: []gatypes.EndpointConfiguration{
			gatypes.EndpointConfiguration{
				EndpointId:                  aws.String(lbArn),
				ClientIPPreservationEnabled: aws.Bool(ipPreserve),
			},
		},
		EndpointGroupArn: aws.String(endpointGroupArn),
	}
	res, err := a.ga.AddEndpoints(ctx, input)
	if err != nil {
		return nil, err
	}
	if res.EndpointDescriptions == nil || len(res.EndpointDescriptions) <= 0 {
		return nil, errors.New("No endpoint is added")
	}
	klog.Infof("Endpoint is added: %s", *res.EndpointDescriptions[0].EndpointId)
	return res.EndpointDescriptions[0].EndpointId, nil
}

func (a *AWS) removeEndpoint(ctx context.Context, endpointGroupArn, endpointId string) error {
	input := &globalaccelerator.RemoveEndpointsInput{
		EndpointGroupArn: aws.String(endpointGroupArn),
		EndpointIdentifiers: []gatypes.EndpointIdentifier{
			gatypes.EndpointIdentifier{
				EndpointId: aws.String(endpointId),
			},
		},
	}
	_, err := a.ga.RemoveEndpoints(ctx, input)
	if err != nil {
		return err
	}
	klog.Infof("Endpoint is removed: %s", endpointId)
	return nil
}

func (a *AWS) createEndpointGroup(ctx context.Context, listener *gatypes.Listener, lbArn *string, region string, ipPreserve bool) (*gatypes.EndpointGroup, error) {
	endpointInput := &globalaccelerator.CreateEndpointGroupInput{
		EndpointConfigurations: []gatypes.EndpointConfiguration{
			gatypes.EndpointConfiguration{
				EndpointId:                  lbArn,
				ClientIPPreservationEnabled: &ipPreserve,
			},
		},
		EndpointGroupRegion: aws.String(region),
		ListenerArn:         listener.ListenerArn,
	}
	endpointRes, err := a.ga.CreateEndpointGroup(ctx, endpointInput)
	if err != nil {
		return nil, err
	}
	klog.Infof("EndpointGroup is created: %s", *endpointRes.EndpointGroup.EndpointGroupArn)
	return endpointRes.EndpointGroup, nil
}

func (a *AWS) updateEndpointGroup(ctx context.Context, endpoint *gatypes.EndpointGroup, lbArn *string, ipPreserve bool) (*gatypes.EndpointGroup, error) {
	input := &globalaccelerator.UpdateEndpointGroupInput{
		EndpointConfigurations: []gatypes.EndpointConfiguration{
			gatypes.EndpointConfiguration{
				EndpointId:                  lbArn,
				ClientIPPreservationEnabled: &ipPreserve,
			},
		},
		EndpointGroupArn: endpoint.EndpointGroupArn,
	}
	res, err := a.ga.UpdateEndpointGroup(ctx, input)
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
	_, err := a.ga.DeleteEndpointGroup(ctx, input)
	if err != nil {
		return err
	}
	klog.Infof("EndpointGroup is deleted: %s", arn)
	return nil
}

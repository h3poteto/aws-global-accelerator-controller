package endpointgroupbinding

import (
	"context"

	endpointgroupbindingv1alpha1 "github.com/h3poteto/aws-global-accelerator-controller/pkg/apis/endpointgroupbinding/v1alpha1"
	"github.com/h3poteto/aws-global-accelerator-controller/pkg/cloudprovider"
	cloudaws "github.com/h3poteto/aws-global-accelerator-controller/pkg/cloudprovider/aws"
	"github.com/h3poteto/aws-global-accelerator-controller/pkg/reconcile"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"
)

const finalizer = "endpointgroupbindings.operator.h3poteto.dev"

func (c *EndpointGroupBindingController) reconcile(ctx context.Context, obj *endpointgroupbindingv1alpha1.EndpointGroupBinding) (reconcile.Result, error) {
	cloud := cloudaws.NewAWS("us-west-2")
	if obj.DeletionTimestamp != nil {
		return c.reconcileDelete(ctx, obj, cloud)
	}
	if len(obj.Status.EndpointIds) == 0 {
		return c.reconcileCreate(ctx, obj, cloud)
	}
	return c.reconcileUpdate(ctx, obj, cloud)
}

func (c *EndpointGroupBindingController) reconcileDelete(ctx context.Context, obj *endpointgroupbindingv1alpha1.EndpointGroupBinding, cloud *cloudaws.AWS) (reconcile.Result, error) {
	endpoint, err := cloud.DescribeEndpointGroup(ctx, obj.Spec.EndpointGroupArn)
	if err != nil {
		klog.Error(err)
		return reconcile.Result{}, err
	}
	endpointIds := obj.Status.EndpointIds
	for i := range obj.Status.EndpointIds {
		id := obj.Status.EndpointIds[i]
		region := cloudaws.GetRegionFromARN(id)
		cloud := cloudaws.NewAWS(region)
		err := cloud.RemoveLBFromEdnpointGroup(ctx, endpoint, id)
		if err != nil {
			return reconcile.Result{}, err
		}
		endpointIds = append(endpointIds[:i], endpointIds[i+1:]...)
	}

	copied := obj.DeepCopy()
	copied.Status.EndpointIds = endpointIds
	copied.Status.ObservedGeneration = obj.Generation
	copied.Finalizers = []string{}
	_, err = c.client.OperatorV1alpha1().EndpointGroupBindings(copied.Namespace).Update(ctx, copied, metav1.UpdateOptions{})
	if err != nil {
		klog.Error(err)
		return reconcile.Result{}, err
	}

	return reconcile.Result{}, nil
}

func (c *EndpointGroupBindingController) reconcileCreate(ctx context.Context, obj *endpointgroupbindingv1alpha1.EndpointGroupBinding, cloud *cloudaws.AWS) (reconcile.Result, error) {

	endpoint, err := cloud.DescribeEndpointGroup(ctx, obj.Spec.EndpointGroupArn)
	if err != nil {
		klog.Error(err)
		return reconcile.Result{}, err
	}

	hostnames := []string{}
	if obj.Spec.ServiceRef != nil {
		service, err := c.serviceLister.Services(obj.Namespace).Get(obj.Spec.ServiceRef.Name)
		if err != nil {
			klog.Error(err)
			return reconcile.Result{}, err
		}
		if len(service.Status.LoadBalancer.Ingress) < 1 {
			klog.Warningf("%s/%s does not have ingress LoadBalancer, so skip it", service.Namespace, service.Name)
			return reconcile.Result{}, nil
		}
		for i := range service.Status.LoadBalancer.Ingress {
			hostnames = append(hostnames, service.Status.LoadBalancer.Ingress[i].Hostname)
		}
	} else if obj.Spec.IngressRef != nil {
		ingress, err := c.ingressLister.Ingresses(obj.Namespace).Get(obj.Spec.IngressRef.Name)
		if err != nil {
			klog.Error(err)
			return reconcile.Result{}, err
		}
		if len(ingress.Status.LoadBalancer.Ingress) < 1 {
			klog.Warningf("%s/%s does not have ingress LoadBalancer, so skip it", ingress.Namespace, ingress.Name)
			return reconcile.Result{}, nil
		}
		for i := range ingress.Status.LoadBalancer.Ingress {
			hostnames = append(hostnames, ingress.Status.LoadBalancer.Ingress[i].Hostname)
		}
	} else {
		klog.Errorf("EndpointGroupBinding %s does not have serviceRef or ingressRef", obj.Name)
		return reconcile.Result{}, nil
	}

	endpointIds := []string{}
	for _, hostname := range hostnames {
		provider, err := cloudprovider.DetectCloudProvider(hostname)
		if err != nil {
			klog.Error(err)
			continue
		}
		switch provider {
		case "aws":
			// Get load balancer name and region from the hostname
			name, region, err := cloudaws.GetLBNameFromHostname(hostname)
			if err != nil {
				klog.Error(err)
				return reconcile.Result{}, err
			}
			cloud := cloudaws.NewAWS(region)
			endpointId, retry, err := cloud.AddLBToEndpointGroup(ctx, endpoint, name, obj.Spec.ClientIPPreservation)
			if err != nil {
				return reconcile.Result{}, err
			}
			if retry > 0 {
				return reconcile.Result{
					Requeue:      true,
					RequeueAfter: retry,
				}, nil
			}
			if endpointId != nil {
				endpointIds = append(endpointIds, *endpointId)
			}

		default:
			klog.Warningf("Not implemented provider: %s", provider)
			continue
		}
	}

	// Update status
	copied := obj.DeepCopy()
	copied.Status.EndpointIds = endpointIds
	copied.Status.ObservedGeneration = obj.Generation
	copied.Finalizers = []string{finalizer}
	_, err = c.client.OperatorV1alpha1().EndpointGroupBindings(copied.Namespace).Update(ctx, copied, metav1.UpdateOptions{})

	if err != nil {
		klog.Error(err)
		return reconcile.Result{}, err
	}

	return reconcile.Result{}, nil
}

func (c *EndpointGroupBindingController) reconcileUpdate(ctx context.Context, obj *endpointgroupbindingv1alpha1.EndpointGroupBinding, cloud *cloudaws.AWS) (reconcile.Result, error) {
	return reconcile.Result{}, nil
}

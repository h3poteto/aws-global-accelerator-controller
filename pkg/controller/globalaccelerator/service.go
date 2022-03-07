package globalaccelerator

import (
	"context"

	"github.com/h3poteto/aws-global-accelerator-controller/pkg/apis"
	cloudaws "github.com/h3poteto/aws-global-accelerator-controller/pkg/cloudprovider/aws"
	pkgerrors "github.com/h3poteto/aws-global-accelerator-controller/pkg/errors"
	"github.com/h3poteto/aws-global-accelerator-controller/pkg/reconcile"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/cache"
	"k8s.io/klog/v2"
)

func wasLoadBalancerService(svc *corev1.Service) bool {
	if svc.Spec.Type != corev1.ServiceTypeLoadBalancer {
		return false
	}

	if _, ok := svc.Annotations["service.beta.kubernetes.io/aws-load-balancer-type"]; !ok || svc.Spec.LoadBalancerClass == nil {
		return false
	}
	return true
}

func (c *GlobalAcceleratorController) processServiceDelete(ctx context.Context, key string) (reconcile.Result, error) {
	klog.Infof("%v has been deleted", key)
	ns, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		return reconcile.Result{}, pkgerrors.NewNoRetryErrorf("invalid resource key: %s", key)
	}

	correspondence, err := c.prepareCorrespondence(ctx)
	if err != nil {
		klog.Errorf("Failed to prepare ConfigMap: %v", err)
		return reconcile.Result{}, err
	}

	cloud := cloudaws.NewAWS("us-west-2")
	accelerators, err := cloud.ListGlobalAcceleratorByTag(ctx, cloudaws.AcceleratorManagedTagValue("service", ns, name))
	if err != nil {
		klog.Error(err)
		return reconcile.Result{}, err
	}
	for _, accelerator := range accelerators {
		if err := cloud.CleanupGlobalAccelerator(ctx, *accelerator.AcceleratorArn); err != nil {
			klog.Error(err)
			return reconcile.Result{}, err
		}
		for key, value := range correspondence {
			if value == *accelerator.AcceleratorArn {
				delete(correspondence, key)
			}
		}
	}
	err = c.updateCorrespondence(ctx, correspondence)
	return reconcile.Result{}, err
}

func (c *GlobalAcceleratorController) processServiceCreateOrUpdate(ctx context.Context, obj runtime.Object) (reconcile.Result, error) {
	svc, ok := obj.(*corev1.Service)
	if !ok {
		return reconcile.Result{}, pkgerrors.NewNoRetryErrorf("object is not Service, it is %T", obj)
	}
	if len(svc.Status.LoadBalancer.Ingress) < 1 {
		klog.Warningf("%s/%s does not have ingress LoadBalancer, so skip it", svc.Namespace, svc.Name)
		return reconcile.Result{}, nil
	}

	correspondence, err := c.prepareCorrespondence(ctx)
	if err != nil {
		klog.Errorf("Failed to prepare ConfigMap: %v", err)
		return reconcile.Result{}, err
	}

	if _, ok := svc.Annotations[apis.AWSGlobalAcceleratorEnabledAnnotation]; !ok {
		deleted := 0
	INGRESS:
		for i := range svc.Status.LoadBalancer.Ingress {
			ingress := svc.Status.LoadBalancer.Ingress[i]
			if acceleratorArn, ok := correspondence[ingress.Hostname]; ok {
				klog.Infof("Service %s/%s does not have annotation, but it is recorded in configmaps: %s", svc.Namespace, svc.Name, ingress.Hostname)
				provider, err := detectCloudProvider(ingress.Hostname)
				if err != nil {
					klog.Error(err)
					continue INGRESS
				}
				switch provider {
				case "aws":
					_, region := cloudaws.GetLBNameFromHostname(ingress.Hostname)
					cloud := cloudaws.NewAWS(region)
					err := cloud.CleanupGlobalAccelerator(ctx, acceleratorArn)
					if err != nil {
						klog.Error(err)
						return reconcile.Result{}, err
					}
					delete(correspondence, ingress.Hostname)
					deleted++
				default:
					klog.Warningf("Not implemented for %s", provider)
					continue INGRESS
				}
			}
		}
		if deleted > 0 {
			if err := c.updateCorrespondence(ctx, correspondence); err != nil {
				klog.Error(err)
				return reconcile.Result{}, err
			}
		} else {
			klog.Infof("%s/%s does not have the annotation, so skip it", svc.Namespace, svc.Name)
		}
		return reconcile.Result{}, nil
	}

	for i := range svc.Status.LoadBalancer.Ingress {
		ingress := svc.Status.LoadBalancer.Ingress[i]
		provider, err := detectCloudProvider(ingress.Hostname)
		if err != nil {
			klog.Error(err)
			continue
		}
		switch provider {
		case "aws":
			// Get load balancer name and region from the hostname
			name, region := cloudaws.GetLBNameFromHostname(ingress.Hostname)
			cloud := cloudaws.NewAWS(region)
			acceleratorArn, retryAfter, err := cloud.EnsureGlobalAcceleratorForService(ctx, svc, &ingress, name, region, correspondence[ingress.Hostname])
			if err != nil {
				return reconcile.Result{}, err
			}
			if retryAfter > 0 {
				return reconcile.Result{
					Requeue:      true,
					RequeueAfter: retryAfter,
				}, nil
			}
			if acceleratorArn != nil {
				correspondence[ingress.Hostname] = *acceleratorArn
			}
		default:
			klog.Warningf("Not implemented for %s", provider)
			continue
		}
	}

	err = c.updateCorrespondence(ctx, correspondence)
	return reconcile.Result{}, err
}

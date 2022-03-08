package globalaccelerator

import (
	"context"

	"github.com/h3poteto/aws-global-accelerator-controller/pkg/apis"
	cloudaws "github.com/h3poteto/aws-global-accelerator-controller/pkg/cloudprovider/aws"
	pkgerrors "github.com/h3poteto/aws-global-accelerator-controller/pkg/errors"
	"github.com/h3poteto/aws-global-accelerator-controller/pkg/reconcile"

	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/cache"
	"k8s.io/klog/v2"
)

func wasALBIngress(ingress *networkingv1.Ingress) bool {
	if *ingress.Spec.IngressClassName == "alb" {
		return true
	}
	if _, ok := ingress.Annotations["kubernetes.io/ingress.class"]; ok {
		return true
	}
	return false
}

func (c *GlobalAcceleratorController) processIngressDelete(ctx context.Context, key string) (reconcile.Result, error) {
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
	accelerators, err := cloud.ListGlobalAcceleratorByTag(ctx, cloudaws.AcceleratorManagedTagValue("ingress", ns, name))
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

func (c *GlobalAcceleratorController) processIngressCreateOrUpdate(ctx context.Context, obj runtime.Object) (reconcile.Result, error) {
	ingress, ok := obj.(*networkingv1.Ingress)
	if !ok {
		return reconcile.Result{}, pkgerrors.NewNoRetryErrorf("object is not Ingress, it is %T", obj)
	}
	if len(ingress.Status.LoadBalancer.Ingress) < 1 {
		klog.Warningf("%s/%s does not have ingress LoadBalancer, so skip it", ingress.Namespace, ingress.Name)
		return reconcile.Result{}, nil
	}

	correspondence, err := c.prepareCorrespondence(ctx)
	if err != nil {
		klog.Errorf("Failed to prepare ConfigMap: %v", err)
		return reconcile.Result{}, err
	}

	if _, ok := ingress.Annotations[apis.AWSGlobalAcceleratorEnabledAnnotation]; !ok {
		deleted := 0
	INGRESS:
		for i := range ingress.Status.LoadBalancer.Ingress {
			lbIngress := ingress.Status.LoadBalancer.Ingress[i]
			if acceleratorArn, ok := correspondence[lbIngress.Hostname]; ok {
				klog.Infof("Ingress %s/%s does not have annotation, but it is recorded in configmaps: %s", ingress.Namespace, ingress.Name, lbIngress.Hostname)
				provider, err := detectCloudProvider(lbIngress.Hostname)
				if err != nil {
					klog.Error(err)
					continue INGRESS
				}
				switch provider {
				case "aws":
					_, region, err := cloudaws.GetLBNameFromHostname(lbIngress.Hostname)
					if err != nil {
						klog.Error(err)
						return reconcile.Result{}, err
					}
					cloud := cloudaws.NewAWS(region)
					err = cloud.CleanupGlobalAccelerator(ctx, acceleratorArn)
					if err != nil {
						klog.Error(err)
						return reconcile.Result{}, err
					}
					delete(correspondence, lbIngress.Hostname)
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
			klog.Infof("%s/%s does not have the annotation, so skip it", ingress.Namespace, ingress.Name)
		}
		return reconcile.Result{}, nil
	}

	for i := range ingress.Status.LoadBalancer.Ingress {
		ing := ingress.Status.LoadBalancer.Ingress[i]
		provider, err := detectCloudProvider(ing.Hostname)
		if err != nil {
			klog.Error(err)
			continue
		}
		switch provider {
		case "aws":
			// Get load balancer name and region from the hostname
			name, region, err := cloudaws.GetLBNameFromHostname(ing.Hostname)
			if err != nil {
				klog.Error(err)
				return reconcile.Result{}, err
			}
			cloud := cloudaws.NewAWS(region)
			acceleratorArn, retryAfter, err := cloud.EnsureGlobalAcceleratorForIngress(ctx, ingress, &ing, name, region, correspondence[ing.Hostname])
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
				correspondence[ing.Hostname] = *acceleratorArn
			}
		default:
			klog.Warningf("Not implemented for %s", provider)
			continue
		}
	}

	err = c.updateCorrespondence(ctx, correspondence)
	return reconcile.Result{}, err
}

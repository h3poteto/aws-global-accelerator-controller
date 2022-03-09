package globalaccelerator

import (
	"context"

	"github.com/h3poteto/aws-global-accelerator-controller/pkg/apis"
	cloudaws "github.com/h3poteto/aws-global-accelerator-controller/pkg/cloudprovider/aws"
	pkgerrors "github.com/h3poteto/aws-global-accelerator-controller/pkg/errors"
	"github.com/h3poteto/aws-global-accelerator-controller/pkg/reconcile"

	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/cache"
	"k8s.io/klog/v2"
)

func wasALBIngress(ingress *networkingv1.Ingress) bool {
	if ingress.Spec.IngressClassName != nil && *ingress.Spec.IngressClassName == "alb" {
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

	cloud := cloudaws.NewAWS("us-west-2")

	accelerators, err := cloud.ListGlobalAcceletaroByResource(ctx, "service", ns, name)
	if err != nil {
		klog.Error(err)
		return reconcile.Result{}, err
	}
	for _, accelerator := range accelerators {
		if err := cloud.CleanupGlobalAccelerator(ctx, *accelerator.AcceleratorArn); err != nil {
			klog.Error(err)
			return reconcile.Result{}, err
		}
	}
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

	if _, ok := ingress.Annotations[apis.AWSGlobalAcceleratorEnabledAnnotation]; !ok {
		deleted := 0
	INGRESS:
		for i := range ingress.Status.LoadBalancer.Ingress {
			lbIngress := ingress.Status.LoadBalancer.Ingress[i]
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
				accelerators, err := cloud.ListGlobalAcceleratorByHostname(ctx, lbIngress.Hostname, "ingress", ingress.Namespace, ingress.Name)
				if err != nil {
					klog.Error(err)
					return reconcile.Result{}, err
				}
				for _, a := range accelerators {
					klog.Infof("Ingress %s/%s does not have the annotation, but Global Accelerator exists, so deleting this", ingress.Namespace, ingress.Name)
					err = cloud.CleanupGlobalAccelerator(ctx, *a.AcceleratorArn)
					if err != nil {
						klog.Error(err)
						return reconcile.Result{}, err
					}
					deleted++
					c.recorder.Eventf(ingress, corev1.EventTypeNormal, "GlobalAcceleratorDeleted", "The annotation does not exist, but Global Acclerator exists, so it is deleted: %s", *a.AcceleratorArn)
				}

			default:
				klog.Warningf("Not implemented for %s", provider)
				continue INGRESS
			}
		}
		if deleted == 0 {
			klog.Infof("%s/%s does not have the annotation, so skip it", ingress.Namespace, ingress.Name)
		}
		return reconcile.Result{}, nil
	}

	for i := range ingress.Status.LoadBalancer.Ingress {
		lbIngress := ingress.Status.LoadBalancer.Ingress[i]
		provider, err := detectCloudProvider(lbIngress.Hostname)
		if err != nil {
			klog.Error(err)
			continue
		}
		switch provider {
		case "aws":
			// Get load balancer name and region from the hostname
			name, region, err := cloudaws.GetLBNameFromHostname(lbIngress.Hostname)
			if err != nil {
				klog.Error(err)
				return reconcile.Result{}, err
			}
			cloud := cloudaws.NewAWS(region)
			arn, created, retryAfter, err := cloud.EnsureGlobalAcceleratorForIngress(ctx, ingress, &lbIngress, name, region)
			if err != nil {
				return reconcile.Result{}, err
			}
			if retryAfter > 0 {
				return reconcile.Result{
					Requeue:      true,
					RequeueAfter: retryAfter,
				}, nil
			}
			if created {
				c.recorder.Eventf(ingress, corev1.EventTypeNormal, "GlobalAcceleratorCreated", "Global Acclerator is created: %s", *arn)
			}
		default:
			klog.Warningf("Not implemented for %s", provider)
			continue
		}
	}

	return reconcile.Result{}, nil
}

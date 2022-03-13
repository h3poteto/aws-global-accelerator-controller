package route53

import (
	"context"
	"strings"

	"github.com/h3poteto/aws-global-accelerator-controller/pkg/apis"
	"github.com/h3poteto/aws-global-accelerator-controller/pkg/cloudprovider"
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
	if _, ok := ingress.Annotations[apis.IngressClassAnnotation]; ok {
		return true
	}
	return false
}

func (c *Route53Controller) processIngressDelete(ctx context.Context, key string) (reconcile.Result, error) {
	klog.Infof("%v has been deleted", key)
	ns, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		return reconcile.Result{}, pkgerrors.NewNoRetryErrorf("invalid resource key: %s", key)
	}
	cloud := cloudaws.NewAWS("us-west-2")
	err = cloud.CleanupRecordSet(ctx, "ingress", ns, name)
	if err != nil {
		klog.Error(err)
		return reconcile.Result{}, err
	}
	return reconcile.Result{}, nil

}

func (c *Route53Controller) processIngressCreateOrUpdate(ctx context.Context, obj runtime.Object) (reconcile.Result, error) {
	ingress, ok := obj.(*networkingv1.Ingress)
	if !ok {
		return reconcile.Result{}, pkgerrors.NewNoRetryErrorf("object is not Ingress, it is %T", obj)
	}

	hostname, ok := ingress.Annotations[apis.Route53HostnameAnnotation]
	if !ok {
		cloud := cloudaws.NewAWS("us-west-2")
		err := cloud.CleanupRecordSet(ctx, "ingress", ingress.Namespace, ingress.Name)
		if err != nil {
			klog.Error(err)
			return reconcile.Result{}, err
		}
		klog.Infof("Delete route53 records for Ingress %s/%s", ingress.Namespace, ingress.Name)
		c.recorder.Event(ingress, corev1.EventTypeNormal, "Route53RecordDeleted", "Route53 record sets are deleted")
		return reconcile.Result{}, nil
	}

	hostnames := strings.Split(hostname, ",")

	for i := range ingress.Status.LoadBalancer.Ingress {
		lbIngress := ingress.Status.LoadBalancer.Ingress[i]
		provider, err := cloudprovider.DetectCloudProvider(lbIngress.Hostname)
		if err != nil {
			klog.Error(err)
			continue
		}
		switch provider {
		case "aws":
			_, region, err := cloudaws.GetLBNameFromHostname(lbIngress.Hostname)
			if err != nil {
				klog.Error(err)
				return reconcile.Result{}, err
			}
			cloud := cloudaws.NewAWS(region)
			created, retryAfter, err := cloud.EnsureRoute53ForIngress(ctx, ingress, &lbIngress, hostnames)
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
				c.recorder.Eventf(ingress, corev1.EventTypeNormal, "Route53RecordCreated", "Route53 record set is created: %v", hostnames)
			}
		default:
			klog.Warningf("Not implemented for %s", provider)
			continue
		}
	}

	return reconcile.Result{}, nil
}

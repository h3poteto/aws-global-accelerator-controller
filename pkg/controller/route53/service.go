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
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/cache"
	"k8s.io/klog/v2"
)

func wasLoadBalancerService(svc *corev1.Service) bool {
	if svc.Spec.Type == corev1.ServiceTypeLoadBalancer {
		if _, ok := svc.Annotations[apis.AWSLoadBalancerTypeAnnotation]; ok || svc.Spec.LoadBalancerClass != nil {
			return true
		}
	}

	return false
}

func hasHostnameAnnotation(svc *corev1.Service) bool {
	_, ok := svc.Annotations[apis.Route53HostnameAnnotation]
	return ok
}

func hostnameAnnotationChanged(old *corev1.Service, new *corev1.Service) bool {
	_, oldHas := old.Annotations[apis.Route53HostnameAnnotation]
	_, newHas := new.Annotations[apis.Route53HostnameAnnotation]
	return oldHas != newHas
}

func (c *Route53Controller) processServiceDelete(ctx context.Context, key string) (reconcile.Result, error) {
	klog.Infof("%v has been deleted", key)
	ns, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		return reconcile.Result{}, pkgerrors.NewNoRetryErrorf("invalid resource key: %s", key)
	}
	cloud := cloudaws.NewAWS("us-west-2")
	err = cloud.CleanupRecordSetForService(ctx, ns, name)
	if err != nil {
		klog.Error(err)
		return reconcile.Result{}, err
	}
	return reconcile.Result{}, nil
}

func (c *Route53Controller) processServiceCreateOrUpdate(ctx context.Context, obj runtime.Object) (reconcile.Result, error) {
	svc, ok := obj.(*corev1.Service)
	if !ok {
		return reconcile.Result{}, pkgerrors.NewNoRetryErrorf("object is not Service, it is %T", obj)
	}

	hostname, ok := svc.Annotations[apis.Route53HostnameAnnotation]
	if !ok {
		cloud := cloudaws.NewAWS("us-west-2")
		err := cloud.CleanupRecordSetForService(ctx, svc.Namespace, svc.Name)
		if err != nil {
			klog.Error(err)
			return reconcile.Result{}, err
		}
		klog.Infof("Delete route53 records for Service %s/%s", svc.Namespace, svc.Name)
		c.recorder.Event(svc, corev1.EventTypeNormal, "Route53RecordDeleted", "Route53 record sets are deleted")
		return reconcile.Result{}, nil
	}

	hostnames := strings.Split(hostname, ",")

	for i := range svc.Status.LoadBalancer.Ingress {
		lbIngress := svc.Status.LoadBalancer.Ingress[i]
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
			created, err := cloud.EnsureRoute53ForService(ctx, svc, &lbIngress, hostnames)
			if err != nil {
				return reconcile.Result{}, err
			}
			if created {
				c.recorder.Eventf(svc, corev1.EventTypeNormal, "Route53RecourdCreated", "Route53 record set is created: %s", hostnames)
			}
		default:
			klog.Warningf("Not impelmented for %s", provider)
			continue
		}
	}
	return reconcile.Result{}, nil
}
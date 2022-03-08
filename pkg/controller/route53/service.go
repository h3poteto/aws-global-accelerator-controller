package route53

import (
	"context"

	"github.com/h3poteto/aws-global-accelerator-controller/pkg/reconcile"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
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

func (c *Route53Controller) processServiceDelete(ctx context.Context, key string) (reconcile.Result, error) {
	// TODO
	return reconcile.Result{}, nil
}

func (c *Route53Controller) processServiceCreateOrUpdate(ctx context.Context, obj runtime.Object) (reconcile.Result, error) {
	// TODO
	return reconcile.Result{}, nil
}

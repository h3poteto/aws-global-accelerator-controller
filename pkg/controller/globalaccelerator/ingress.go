package globalaccelerator

import (
	"context"

	"github.com/h3poteto/aws-global-accelerator-controller/pkg/reconcile"
	"k8s.io/apimachinery/pkg/runtime"
)

func (c *GlobalAcceleratorController) processIngressDelete(ctx context.Context, key string) (reconcile.Result, error) {
	// TODO: Delete logic
	return reconcile.Result{}, nil
}

func (c *GlobalAcceleratorController) processIngressCreateOrUpdate(ctx context.Context, obj runtime.Object) (reconcile.Result, error) {
	// TODO: Create or Update
	return reconcile.Result{}, nil
}

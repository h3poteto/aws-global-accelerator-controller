package reconcile

import (
	"context"
	"fmt"
	"time"

	pkgerrors "github.com/h3poteto/aws-global-accelerator-controller/pkg/errors"

	kerrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/klog/v2"
)

type Result struct {
	Requeue      bool
	RequeueAfter time.Duration
}

type KeyToObjFunc func(string) (runtime.Object, error)
type ProcessDeleteFunc func(context.Context, string) (Result, error)
type ProcessCreateOrUpdateFunc func(context.Context, runtime.Object) (Result, error)

func ProcessNextWorkItem(workqueue workqueue.RateLimitingInterface, keyToObj KeyToObjFunc, processDelete ProcessDeleteFunc, processCreateOrUpdate ProcessCreateOrUpdateFunc) bool {
	obj, shutdown := workqueue.Get()

	if shutdown {
		return false
	}

	defer workqueue.Done(obj)

	err := reconcileHandler(obj, workqueue, keyToObj, processDelete, processCreateOrUpdate)
	if err != nil {
		utilruntime.HandleError(err)
		return true
	}

	return true
}

func reconcileHandler(req interface{}, workqueue workqueue.RateLimitingInterface, keyToObj KeyToObjFunc, processDelete ProcessDeleteFunc, processCreateOrUpdate ProcessCreateOrUpdateFunc) error {
	var key string
	var ok bool

	if key, ok = req.(string); !ok {
		workqueue.Forget(req)
		return fmt.Errorf("expected string in workqueue but got %#v", req)
	}
	startTime := time.Now()
	defer func() {
		klog.V(4).Infof("Finished syncing %q (%v)", key, time.Since(startTime))
	}()

	ctx := context.Background()

	res := Result{}
	obj, err := keyToObj(key)
	switch {
	case kerrors.IsNotFound(err):
		res, err = processDelete(ctx, key)
	case err != nil:
		return fmt.Errorf("Unable to retrieve %q from store: %v", key, err)
	default:
		res, err = processCreateOrUpdate(ctx, obj.DeepCopyObject())
	}

	switch {
	case err != nil:
		if pkgerrors.IsNoRetry(err) {
			return fmt.Errorf("error syncing %q: %s", key, err.Error())
		} else {
			workqueue.AddRateLimited(req)
			return fmt.Errorf("error syncing %q, and requeued: %s", key, err.Error())
		}

	case res.RequeueAfter > 0:
		workqueue.Forget(req)
		workqueue.AddAfter(req, res.RequeueAfter)
		klog.Infof("Successfully synced %q, but requeued after %v", key, res.RequeueAfter)
	case res.Requeue:
		workqueue.AddRateLimited(req)
		klog.Infof("Successfully synced %q, but requeued", key)
	default:
		workqueue.Forget(req)
		klog.Infof("Successfully synced %q", key)
	}
	return nil
}

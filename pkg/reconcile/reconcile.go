package reconcile

import (
	"context"
	"fmt"
	"time"

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
type ProcessDeleteFunc func(context.Context, string) error
type ProcessCreateOrUpdateFunc func(context.Context, runtime.Object) error

func ProcessNextWorkItem(workqueue workqueue.RateLimitingInterface, keyToObj KeyToObjFunc, processDelete ProcessDeleteFunc, processCreateOrUpdate ProcessCreateOrUpdateFunc) bool {
	obj, shutdown := workqueue.Get()

	if shutdown {
		return false
	}

	err := func(obj interface{}) error {
		defer workqueue.Done(obj)
		var key string
		var ok bool

		if key, ok = obj.(string); !ok {
			workqueue.Forget(obj)
			utilruntime.HandleError(fmt.Errorf("expected string in workqueue but got %#v", obj))
			return nil
		}
		if err := syncHandler(key, keyToObj, processDelete, processCreateOrUpdate); err != nil {
			return fmt.Errorf("error syncing '%s': %s", key, err.Error())
		}
		workqueue.Forget(obj)
		klog.Infof("Successfully synced '%s'", key)
		return nil
	}(obj)

	if err != nil {
		utilruntime.HandleError(err)
		return true
	}

	return true
}

func syncHandler(key string, keyToObj KeyToObjFunc, processDelete ProcessDeleteFunc, processCreateOrUpdate ProcessCreateOrUpdateFunc) error {
	startTime := time.Now()
	defer func() {
		klog.V(4).Infof("Finished syncing %q (%v)", key, time.Since(startTime))
	}()

	ctx := context.Background()

	obj, err := keyToObj(key)
	switch {
	case kerrors.IsNotFound(err):
		err = processDelete(ctx, key)
	case err != nil:
		utilruntime.HandleError(fmt.Errorf("Unable to retrieve %q from store: %v", key, err))
	default:
		err = processCreateOrUpdate(ctx, obj)
	}

	return err
}

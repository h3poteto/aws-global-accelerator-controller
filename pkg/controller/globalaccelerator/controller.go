package globalaccelerator

import (
	"context"
	"fmt"
	"time"

	"github.com/h3poteto/aws-global-accelerator-controller/pkg/apis"
	cloudaws "github.com/h3poteto/aws-global-accelerator-controller/pkg/aws"

	corev1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	typedcorev1 "k8s.io/client-go/kubernetes/typed/core/v1"
	corelisters "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/klog/v2"
	"k8s.io/kubectl/pkg/scheme"
)

const controllerAgentName = "global-accelerator-controller"
const dataConfigMap = "aws-global-accelerator-controller-data"

type GlobalAcceleratorConfig struct {
	Workers   int
	Namespace string
	Region    string
}

type GlobalAcceleratorController struct {
	kubeclient    kubernetes.Interface
	serviceLister corelisters.ServiceLister
	serviceSynced cache.InformerSynced

	workqueue workqueue.RateLimitingInterface

	namespace string

	cloud cloudaws.AWS

	recorder record.EventRecorder
}

func NewGlobalAcceleratorController(kubeclient kubernetes.Interface, informerFactory informers.SharedInformerFactory, config *GlobalAcceleratorConfig) *GlobalAcceleratorController {
	eventBroadcaster := record.NewBroadcaster()
	eventBroadcaster.StartLogging(klog.Infof)
	eventBroadcaster.StartRecordingToSink(&typedcorev1.EventSinkImpl{Interface: kubeclient.CoreV1().Events("")})
	recorder := eventBroadcaster.NewRecorder(scheme.Scheme, corev1.EventSource{Component: controllerAgentName})

	controller := &GlobalAcceleratorController{
		kubeclient: kubeclient,
		recorder:   recorder,
		workqueue:  workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), controllerAgentName),
		cloud:      *cloudaws.NewAWS(config.Region),
		namespace:  config.Namespace,
	}
	{
		f := informerFactory.Core().V1().Services()
		controller.serviceLister = f.Lister()
		controller.serviceSynced = f.Informer().HasSynced
		f.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
			AddFunc: controller.handleService,
			UpdateFunc: func(old, new interface{}) {
				newSvc := new.(*corev1.Service)
				oldSvc := old.(*corev1.Service)
				if newSvc.ResourceVersion == oldSvc.ResourceVersion {
					return
				}
				controller.handleService(new)
			},
			DeleteFunc: controller.handleService,
		})

	}
	return controller
}

func (c *GlobalAcceleratorController) Run(threadiness int, stopCh <-chan struct{}) error {
	defer utilruntime.HandleCrash()
	defer c.workqueue.ShutDown()

	klog.Info("Starting GlobalAccelerator controller")

	klog.Info("Waiting for informer caches to sync")
	if ok := cache.WaitForCacheSync(stopCh, c.serviceSynced); !ok {
		return fmt.Errorf("failed to wait for caches to sync")
	}

	klog.Info("Starting workers")
	for i := 0; i < threadiness; i++ {
		go wait.Until(c.runWorker, time.Second, stopCh)
	}

	klog.Info("Started workers")
	<-stopCh
	klog.Info("Shutting down workers")

	return nil
}

func (c *GlobalAcceleratorController) runWorker() {
	for c.processNextWorkItem() {
	}
}

func (c *GlobalAcceleratorController) processNextWorkItem() bool {
	obj, shutdown := c.workqueue.Get()

	if shutdown {
		return false
	}

	err := func(obj interface{}) error {
		defer c.workqueue.Done(obj)
		var key string
		var ok bool

		if key, ok = obj.(string); !ok {
			c.workqueue.Forget(obj)
			utilruntime.HandleError(fmt.Errorf("expected string in workqueue but got %#v", obj))
			return nil
		}
		if err := c.syncHandler(key); err != nil {
			return fmt.Errorf("error syncing '%s': %s", key, err.Error())
		}
		c.workqueue.Forget(obj)
		klog.Infof("Successfully synced '%s'", key)
		return nil
	}(obj)

	if err != nil {
		utilruntime.HandleError(err)
		return true
	}

	return true
}

func (c *GlobalAcceleratorController) syncHandler(key string) error {
	startTime := time.Now()
	defer func() {
		klog.V(4).Infof("Finished syncing service %q (%v)", key, time.Since(startTime))
	}()
	ctx := context.Background()
	ns, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		utilruntime.HandleError(fmt.Errorf("invalid resource key: %s", key))
		return nil
	}

	svc, err := c.serviceLister.Services(ns).Get(name)
	switch {
	case kerrors.IsNotFound(err):
		err = c.processServiceDelete(ctx, key)
	case err != nil:
		utilruntime.HandleError(fmt.Errorf("Unable to retrieve service %v from store: %v", key, err))
	default:
		err = c.processServiceCreateOrUpdate(ctx, svc)
	}

	return err
}

func (c *GlobalAcceleratorController) enqueueService(obj *corev1.Service) {
	var key string
	var err error
	if key, err = cache.MetaNamespaceKeyFunc(obj); err != nil {
		utilruntime.HandleError(err)
		return
	}
	c.workqueue.AddRateLimited(key)
}

func (c *GlobalAcceleratorController) handleService(obj interface{}) {
	var object metav1.Object
	var ok bool
	if object, ok = obj.(metav1.Object); !ok {
		tombstone, ok := obj.(cache.DeletedFinalStateUnknown)
		if !ok {
			utilruntime.HandleError(fmt.Errorf("error decoding object, invalid type"))
			return
		}
		object, ok = tombstone.Obj.(metav1.Object)
		if !ok {
			utilruntime.HandleError(fmt.Errorf("error decoding object tombstone, invalid type"))
			return
		}
		klog.V(4).Infof("Recovered deleted object '%s' from tombstone", object.GetName())
	}
	klog.V(4).Infof("Processing object: %s", object.GetName())
	svc := obj.(*corev1.Service)
	if svc.Spec.Type == corev1.ServiceTypeLoadBalancer {
		c.enqueueService(svc)
	}
}

func (c *GlobalAcceleratorController) processServiceDelete(ctx context.Context, key string) error {
	klog.Infof("%v has been deleted", key)
	// TODO:
	return nil
}

func (c *GlobalAcceleratorController) processServiceCreateOrUpdate(ctx context.Context, svc *corev1.Service) error {
	if _, ok := svc.Annotations[apis.EnableAWSGlobalAcceleratorAnnotation]; !ok {
		klog.Infof("%s/%s does not have the annotation, so skip it", svc.Namespace, svc.Name)
		return nil
	}
	if len(svc.Status.LoadBalancer.Ingress) < 1 {
		klog.Warningf("%s/%s does not have ingress LoadBalancer, so skip it", svc.Namespace, svc.Name)
		return nil
	}

	correspondence, err := c.prepareConfigMap(ctx)
	if err != nil {
		klog.Errorf("Failed to prepare ConfigMap: %v", err)
		return err
	}

	for i := range svc.Status.LoadBalancer.Ingress {
		ingress := svc.Status.LoadBalancer.Ingress[i]
		acceleratorArn, err := c.cloud.EnsureGlobalAccelerator(ctx, svc, &ingress, correspondence)
		if err != nil {
			return err
		}
		if acceleratorArn != nil {
			correspondence[ingress.Hostname] = *acceleratorArn
		}
	}

	return c.updateConfigMap(ctx, correspondence)
}

package route53

import (
	"fmt"
	"reflect"
	"time"

	"github.com/h3poteto/aws-global-accelerator-controller/pkg/apis"
	"github.com/h3poteto/aws-global-accelerator-controller/pkg/reconcile"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
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

const controllerAgentName = "route53-controller"

type Route53Config struct {
	Workers int
}

type Route53Controller struct {
	kubeclint     kubernetes.Interface
	serviceLister corelisters.ServiceLister
	serviceSynced cache.InformerSynced

	serviceQueue workqueue.RateLimitingInterface

	recorder record.EventRecorder
}

func NewRoute53Controller(kubeclient kubernetes.Interface, informerFactory informers.SharedInformerFactory, config *Route53Config) *Route53Controller {
	eventBroadcaster := record.NewBroadcaster()
	eventBroadcaster.StartLogging(klog.Infof)
	eventBroadcaster.StartRecordingToSink(&typedcorev1.EventSinkImpl{Interface: kubeclient.CoreV1().Events("")})
	recorder := eventBroadcaster.NewRecorder(scheme.Scheme, corev1.EventSource{Component: controllerAgentName})

	controller := &Route53Controller{
		kubeclint:    kubeclient,
		recorder:     recorder,
		serviceQueue: workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), controllerAgentName+"-service"),
	}
	{
		f := informerFactory.Core().V1().Services()
		controller.serviceLister = f.Lister()
		controller.serviceSynced = f.Informer().HasSynced
		f.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
			AddFunc:    controller.addServiceNotification,
			UpdateFunc: controller.updateServiceNotification,
			DeleteFunc: controller.deleteServiceNotification,
		})
	}

	return controller
}

func (c *Route53Controller) addServiceNotification(obj interface{}) {
	svc := obj.(*corev1.Service)
	if wasLoadBalancerService(svc) && hasHostnameAnnotation(svc) {
		klog.V(4).Infof("Service %s/%s is created", svc.Namespace, svc.Name)
		c.enqueueService(svc)
	}
}

func (c *Route53Controller) updateServiceNotification(old, new interface{}) {
	if reflect.DeepEqual(old, new) {
		return
	}
	oldSvc := old.(*corev1.Service)
	newSvc := new.(*corev1.Service)
	if wasLoadBalancerService(newSvc) {
		if hasHostnameAnnotation(newSvc) || hostnameAnnotationChanged(oldSvc, newSvc) {
			klog.V(4).Infof("Service %s/%s is updated", newSvc.Namespace, newSvc.Name)
			c.enqueueService(newSvc)
		}
	}
}

func (c *Route53Controller) deleteServiceNotification(obj interface{}) {
	svc, ok := obj.(*corev1.Service)
	if !ok {
		tombstone, ok := obj.(cache.DeletedFinalStateUnknown)
		if !ok {
			utilruntime.HandleError(fmt.Errorf("error decoding object, invalid type"))
			return
		}
		svc, ok = tombstone.Obj.(*corev1.Service)
		if !ok {
			utilruntime.HandleError(fmt.Errorf("error decoding object tombstone, invalid type"))
			return
		}
		klog.V(4).Infof("Recovered deleted object %q from tombstone", svc.Name)
	}
	if wasLoadBalancerService(svc) {
		klog.V(4).Infof("Deleting Service %s/%s", svc.Namespace, svc.Name)
		c.enqueueService(svc)
	}
}

func (c *Route53Controller) enqueueService(obj *corev1.Service) {
	var key string
	var err error
	if key, err = cache.MetaNamespaceKeyFunc(obj); err != nil {
		utilruntime.HandleError(err)
		return
	}
	c.serviceQueue.AddRateLimited(key)
}

func (c *Route53Controller) Run(threadiness int, stopCh <-chan struct{}) error {
	defer utilruntime.HandleCrash()
	defer c.serviceQueue.ShutDown()

	klog.Info("Starting Route53 controller")

	klog.Info("Waiting for informer caches to sync")
	if ok := cache.WaitForCacheSync(stopCh, c.serviceSynced); !ok {
		return fmt.Errorf("failed to wait for caches to sync")
	}

	klog.Info("Starting workers")
	for i := 0; i < threadiness; i++ {
		go wait.Until(c.runServiceWorker, time.Second, stopCh)
	}

	klog.Info("Started workers")
	<-stopCh
	klog.Info("Shutting down workers")

	return nil
}

func (c *Route53Controller) runServiceWorker() {
	for reconcile.ProcessNextWorkItem(c.serviceQueue, c.keyToService, c.processServiceDelete, c.processServiceCreateOrUpdate) {
	}
}

func (c *Route53Controller) keyToService(key string) (runtime.Object, error) {
	ns, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		return nil, fmt.Errorf("invalid resource key: %s", key)
	}

	return c.serviceLister.Services(ns).Get(name)
}

func hasHostnameAnnotation(obj metav1.Object) bool {
	_, ok := obj.GetAnnotations()[apis.Route53HostnameAnnotation]
	return ok
}

func hostnameAnnotationChanged(old, new metav1.Object) bool {
	_, oldHas := old.GetAnnotations()[apis.Route53HostnameAnnotation]
	_, newHas := new.GetAnnotations()[apis.Route53HostnameAnnotation]
	return oldHas != newHas
}

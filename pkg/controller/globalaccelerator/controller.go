package globalaccelerator

import (
	"fmt"
	"reflect"
	"strings"
	"time"

	"github.com/h3poteto/aws-global-accelerator-controller/pkg/reconcile"

	corev1 "k8s.io/api/core/v1"
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

const controllerAgentName = "global-accelerator-controller"
const dataConfigMap = "aws-global-accelerator-controller-data"

type GlobalAcceleratorConfig struct {
	Workers   int
	Namespace string
}

type GlobalAcceleratorController struct {
	kubeclient    kubernetes.Interface
	serviceLister corelisters.ServiceLister
	serviceSynced cache.InformerSynced

	serviceQueue workqueue.RateLimitingInterface

	namespace string

	recorder record.EventRecorder
}

// +kubebuilder:rbac:groups="",resources=configmaps,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=services,verbs=get;list;watch
// +kubebuilder:rbac:groups=networking.k8s.io,resources=ingress,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=events,verbs=create;patch

func NewGlobalAcceleratorController(kubeclient kubernetes.Interface, informerFactory informers.SharedInformerFactory, config *GlobalAcceleratorConfig) *GlobalAcceleratorController {
	eventBroadcaster := record.NewBroadcaster()
	eventBroadcaster.StartLogging(klog.Infof)
	eventBroadcaster.StartRecordingToSink(&typedcorev1.EventSinkImpl{Interface: kubeclient.CoreV1().Events("")})
	recorder := eventBroadcaster.NewRecorder(scheme.Scheme, corev1.EventSource{Component: controllerAgentName})

	controller := &GlobalAcceleratorController{
		kubeclient:   kubeclient,
		recorder:     recorder,
		serviceQueue: workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), controllerAgentName+"-service"),
		namespace:    config.Namespace,
	}
	{
		f := informerFactory.Core().V1().Services()
		controller.serviceLister = f.Lister()
		controller.serviceSynced = f.Informer().HasSynced
		f.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
			AddFunc:    controller.addSereviceNotification,
			UpdateFunc: controller.updateServiceNotification,
			DeleteFunc: controller.deleteServiceNotification,
		})

	}
	return controller
}

func (c *GlobalAcceleratorController) addSereviceNotification(obj interface{}) {
	svc := obj.(*corev1.Service)
	if wasLoadBalancerService(svc) {
		klog.V(4).Infof("Service %s/%s is created", svc.Namespace, svc.Name)
		c.enqueueService(svc)
	}
}

func (c *GlobalAcceleratorController) updateServiceNotification(old, new interface{}) {
	if reflect.DeepEqual(old, new) {
		return
	}
	svc := new.(*corev1.Service)
	if wasLoadBalancerService(svc) {
		klog.V(4).Infof("Service %s/%s is updated", svc.Namespace, svc.Name)
		c.enqueueService(svc)
	}
}

func (c *GlobalAcceleratorController) deleteServiceNotification(obj interface{}) {
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

func (c *GlobalAcceleratorController) enqueueService(obj *corev1.Service) {
	var key string
	var err error
	if key, err = cache.MetaNamespaceKeyFunc(obj); err != nil {
		utilruntime.HandleError(err)
		return
	}
	c.serviceQueue.AddRateLimited(key)
}

func (c *GlobalAcceleratorController) Run(threadiness int, stopCh <-chan struct{}) error {
	defer utilruntime.HandleCrash()
	defer c.serviceQueue.ShutDown()

	klog.Info("Starting GlobalAccelerator controller")

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

func (c *GlobalAcceleratorController) runServiceWorker() {
	for reconcile.ProcessNextWorkItem(c.serviceQueue, c.keyToObject, c.processServiceDelete, c.processServiceCreateOrUpdate) {
	}
}

func (c *GlobalAcceleratorController) keyToObject(key string) (runtime.Object, error) {
	ns, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		return nil, fmt.Errorf("invalid resource key: %s", key)
	}

	return c.serviceLister.Services(ns).Get(name)
}

func detectCloudProvider(hostname string) (string, error) {
	parts := strings.Split(hostname, ".")
	domain := parts[len(parts)-2] + "." + parts[len(parts)-1]
	switch domain {
	case "amazonaws.com":
		return "aws", nil
	default:
		return "", fmt.Errorf("Unknown cloud provider: %s", domain)
	}
}

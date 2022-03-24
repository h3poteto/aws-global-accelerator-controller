package globalaccelerator

import (
	"fmt"
	"reflect"
	"time"

	"github.com/h3poteto/aws-global-accelerator-controller/pkg/apis"
	"github.com/h3poteto/aws-global-accelerator-controller/pkg/reconcile"

	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	typedcorev1 "k8s.io/client-go/kubernetes/typed/core/v1"
	corelisters "k8s.io/client-go/listers/core/v1"
	networkinglisters "k8s.io/client-go/listers/networking/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/klog/v2"
	"k8s.io/kubectl/pkg/scheme"
)

const controllerAgentName = "global-accelerator-controller"

type GlobalAcceleratorConfig struct {
	Workers int
	APIHost string
}

type GlobalAcceleratorController struct {
	apiHost       string
	kubeclient    kubernetes.Interface
	serviceLister corelisters.ServiceLister
	serviceSynced cache.InformerSynced
	ingressLister networkinglisters.IngressLister
	ingressSynced cache.InformerSynced

	serviceQueue workqueue.RateLimitingInterface
	ingressQueue workqueue.RateLimitingInterface

	recorder record.EventRecorder
}

// +kubebuilder:rbac:groups="",resources=services,verbs=get;list;watch
// +kubebuilder:rbac:groups=networking.k8s.io,resources=ingresses,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=events,verbs=create;patch

func NewGlobalAcceleratorController(kubeclient kubernetes.Interface, informerFactory informers.SharedInformerFactory, config *GlobalAcceleratorConfig) *GlobalAcceleratorController {
	eventBroadcaster := record.NewBroadcaster()
	eventBroadcaster.StartLogging(klog.Infof)
	eventBroadcaster.StartRecordingToSink(&typedcorev1.EventSinkImpl{Interface: kubeclient.CoreV1().Events("")})
	recorder := eventBroadcaster.NewRecorder(scheme.Scheme, corev1.EventSource{Component: controllerAgentName})

	controller := &GlobalAcceleratorController{
		apiHost:      config.APIHost,
		kubeclient:   kubeclient,
		recorder:     recorder,
		serviceQueue: workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), controllerAgentName+"-service"),
		ingressQueue: workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), controllerAgentName+"-ingress"),
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
	{
		f := informerFactory.Networking().V1().Ingresses()
		controller.ingressLister = f.Lister()
		controller.ingressSynced = f.Informer().HasSynced
		f.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
			AddFunc:    controller.addIngressNotification,
			UpdateFunc: controller.updateIngressNotification,
			DeleteFunc: controller.deleteIngressNotification,
		})
	}

	return controller
}

func (c *GlobalAcceleratorController) addSereviceNotification(obj interface{}) {
	svc := obj.(*corev1.Service)
	if wasLoadBalancerService(svc) && hasManagedAnnotation(svc) {
		klog.V(4).Infof("Service %s/%s is created", svc.Namespace, svc.Name)
		c.enqueueService(svc)
	}
}

func (c *GlobalAcceleratorController) updateServiceNotification(old, new interface{}) {
	if reflect.DeepEqual(old, new) {
		return
	}
	oldSvc := old.(*corev1.Service)
	newSvc := new.(*corev1.Service)
	if wasLoadBalancerService(newSvc) {
		if hasManagedAnnotation(newSvc) || managedAnnotationChanged(oldSvc, newSvc) {
			klog.V(4).Infof("Service %s/%s is updated", newSvc.Namespace, newSvc.Name)
			c.enqueueService(newSvc)
		}
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

func (c *GlobalAcceleratorController) addIngressNotification(obj interface{}) {
	ingress := obj.(*networkingv1.Ingress)
	if wasALBIngress(ingress) && hasManagedAnnotation(ingress) {
		klog.V(4).Infof("Ingress %s/%s is created", ingress.Namespace, ingress.Name)
		c.enqueueIngress(ingress)
	}
}

func (c *GlobalAcceleratorController) updateIngressNotification(old, new interface{}) {
	if reflect.DeepEqual(old, new) {
		return
	}
	oldIngress := old.(*networkingv1.Ingress)
	newIngress := new.(*networkingv1.Ingress)
	if wasALBIngress(newIngress) {
		if hasManagedAnnotation(newIngress) || managedAnnotationChanged(oldIngress, newIngress) {
			klog.V(4).Infof("Ingress %s/%s is updated", newIngress.Namespace, newIngress.Name)
			c.enqueueIngress(newIngress)
		}
	}
}

func (c *GlobalAcceleratorController) deleteIngressNotification(obj interface{}) {
	ingress, ok := obj.(*networkingv1.Ingress)
	if !ok {
		tombstone, ok := obj.(cache.DeletedFinalStateUnknown)
		if !ok {
			utilruntime.HandleError(fmt.Errorf("error decoding object, invalid type"))
			return
		}
		ingress, ok = tombstone.Obj.(*networkingv1.Ingress)
		if !ok {
			utilruntime.HandleError(fmt.Errorf("error decoding object tombstone, invalid type"))
			return
		}
		klog.V(4).Infof("Recovered deleted object %q from tombstone", ingress.Name)
	}
	klog.V(4).Infof("Deleting Ingress %s/%s", ingress.Namespace, ingress.Name)
	c.enqueueIngress(ingress)
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

func (c *GlobalAcceleratorController) enqueueIngress(obj *networkingv1.Ingress) {
	var key string
	var err error
	if key, err = cache.MetaNamespaceKeyFunc(obj); err != nil {
		utilruntime.HandleError(err)
		return
	}
	c.ingressQueue.AddRateLimited(key)
}

func (c *GlobalAcceleratorController) Run(threadiness int, stopCh <-chan struct{}) error {
	defer utilruntime.HandleCrash()
	defer c.serviceQueue.ShutDown()
	defer c.ingressQueue.ShutDown()

	klog.Info("Starting GlobalAccelerator controller")

	klog.Info("Waiting for informer caches to sync")
	if ok := cache.WaitForCacheSync(stopCh, c.serviceSynced); !ok {
		return fmt.Errorf("failed to wait for caches to sync")
	}

	klog.Info("Starting workers")
	for i := 0; i < threadiness; i++ {
		go wait.Until(c.runServiceWorker, time.Second, stopCh)
	}
	for i := 0; i < threadiness; i++ {
		go wait.Until(c.runIngressWorker, time.Second, stopCh)
	}

	klog.Info("Started workers")
	<-stopCh
	klog.Info("Shutting down workers")

	return nil
}

func (c *GlobalAcceleratorController) runServiceWorker() {
	for reconcile.ProcessNextWorkItem(c.serviceQueue, c.keyToService, c.processServiceDelete, c.processServiceCreateOrUpdate) {
	}
}

func (c *GlobalAcceleratorController) runIngressWorker() {
	for reconcile.ProcessNextWorkItem(c.ingressQueue, c.keyToIngress, c.processIngressDelete, c.processIngressCreateOrUpdate) {
	}
}

func (c *GlobalAcceleratorController) keyToService(key string) (runtime.Object, error) {
	ns, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		return nil, fmt.Errorf("invalid resource key: %s", key)
	}

	return c.serviceLister.Services(ns).Get(name)
}

func (c *GlobalAcceleratorController) keyToIngress(key string) (runtime.Object, error) {
	ns, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		return nil, fmt.Errorf("invalid resource key: %s", key)
	}

	return c.ingressLister.Ingresses(ns).Get(name)
}

func hasManagedAnnotation(obj metav1.Object) bool {
	_, ok := obj.GetAnnotations()[apis.AWSGlobalAcceleratorManagedAnnotation]
	return ok
}

func managedAnnotationChanged(old, new metav1.Object) bool {
	_, oldHas := old.GetAnnotations()[apis.AWSGlobalAcceleratorManagedAnnotation]
	_, newHas := new.GetAnnotations()[apis.AWSGlobalAcceleratorManagedAnnotation]
	return oldHas != newHas
}

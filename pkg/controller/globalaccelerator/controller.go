package globalaccelerator

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"time"

	"github.com/h3poteto/aws-global-accelerator-controller/pkg/apis"
	cloudaws "github.com/h3poteto/aws-global-accelerator-controller/pkg/cloudprovider/aws"
	pkgerrors "github.com/h3poteto/aws-global-accelerator-controller/pkg/errors"
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

	workqueue workqueue.RateLimitingInterface

	namespace string

	recorder record.EventRecorder
}

// +kubebuilder:rbac:groups="",resources=configmaps,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=services,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=events,verbs=create;patch

func NewGlobalAcceleratorController(kubeclient kubernetes.Interface, informerFactory informers.SharedInformerFactory, config *GlobalAcceleratorConfig) *GlobalAcceleratorController {
	eventBroadcaster := record.NewBroadcaster()
	eventBroadcaster.StartLogging(klog.Infof)
	eventBroadcaster.StartRecordingToSink(&typedcorev1.EventSinkImpl{Interface: kubeclient.CoreV1().Events("")})
	recorder := eventBroadcaster.NewRecorder(scheme.Scheme, corev1.EventSource{Component: controllerAgentName})

	controller := &GlobalAcceleratorController{
		kubeclient: kubeclient,
		recorder:   recorder,
		workqueue:  workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), controllerAgentName),
		namespace:  config.Namespace,
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
	// TODO: Now we don't have any methods to retry even if some errors occur.
	// So call reconcile when resyncing. If we implement ErrorWithRetry to retry when error,
	// please uncomment these lines.
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
	c.workqueue.AddRateLimited(key)
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
	for reconcile.ProcessNextWorkItem(c.workqueue, c.keyToObject, c.processServiceDelete, c.processServiceCreateOrUpdate) {
	}
}

func (c *GlobalAcceleratorController) keyToObject(key string) (runtime.Object, error) {
	ns, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		return nil, fmt.Errorf("invalid resource key: %s", key)
	}

	return c.serviceLister.Services(ns).Get(name)
}

func (c *GlobalAcceleratorController) processServiceDelete(ctx context.Context, key string) (reconcile.Result, error) {
	klog.Infof("%v has been deleted", key)
	ns, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		return reconcile.Result{}, pkgerrors.NewNoRetryErrorf("invalid resource key: %s", key)
	}

	correspondence, err := c.prepareCorrespondence(ctx)
	if err != nil {
		klog.Errorf("Failed to prepare ConfigMap: %v", err)
		return reconcile.Result{}, err
	}

	cloud := cloudaws.NewAWS("us-west-2")
	accelerators, err := cloud.ListGlobalAcceleratorByTag(ctx, ns+"-"+name)
	if err != nil {
		klog.Error(err)
		return reconcile.Result{}, err
	}
	for _, accelerator := range accelerators {
		if err := cloud.CleanupGlobalAccelerator(ctx, *accelerator.AcceleratorArn); err != nil {
			klog.Error(err)
			return reconcile.Result{}, err
		}
		for key, value := range correspondence {
			if value == *accelerator.AcceleratorArn {
				delete(correspondence, key)
			}
		}
	}
	err = c.updateCorrespondence(ctx, correspondence)
	return reconcile.Result{}, err
}

func (c *GlobalAcceleratorController) processServiceCreateOrUpdate(ctx context.Context, obj runtime.Object) (reconcile.Result, error) {
	svc, ok := obj.(*corev1.Service)
	if !ok {
		return reconcile.Result{}, pkgerrors.NewNoRetryErrorf("object is not Service, it is %T", obj)
	}
	if len(svc.Status.LoadBalancer.Ingress) < 1 {
		klog.Warningf("%s/%s does not have ingress LoadBalancer, so skip it", svc.Namespace, svc.Name)
		return reconcile.Result{}, nil
	}

	correspondence, err := c.prepareCorrespondence(ctx)
	if err != nil {
		klog.Errorf("Failed to prepare ConfigMap: %v", err)
		return reconcile.Result{}, err
	}

	if _, ok := svc.Annotations[apis.AWSGlobalAcceleratorEnabledAnnotation]; !ok {
		deleted := 0
	INGRESS:
		for i := range svc.Status.LoadBalancer.Ingress {
			ingress := svc.Status.LoadBalancer.Ingress[i]
			if acceleratorArn, ok := correspondence[ingress.Hostname]; ok {
				klog.Infof("Service %s/%s does not have annotation, but it is recorded in configmaps: %s", svc.Namespace, svc.Name, ingress.Hostname)
				provider, err := detectCloudProvider(ingress.Hostname)
				if err != nil {
					klog.Error(err)
					continue INGRESS
				}
				switch provider {
				case "aws":
					_, region := cloudaws.GetLBNameFromHostname(ingress.Hostname)
					cloud := cloudaws.NewAWS(region)
					err := cloud.CleanupGlobalAccelerator(ctx, acceleratorArn)
					if err != nil {
						klog.Error(err)
						return reconcile.Result{}, err
					}
					delete(correspondence, ingress.Hostname)
					deleted++
				default:
					klog.Warningf("Not implemented for %s", provider)
					continue INGRESS
				}
			}
		}
		if deleted > 0 {
			if err := c.updateCorrespondence(ctx, correspondence); err != nil {
				klog.Error(err)
				return reconcile.Result{}, err
			}
		} else {
			klog.Infof("%s/%s does not have the annotation, so skip it", svc.Namespace, svc.Name)
		}
		return reconcile.Result{}, nil
	}

	for i := range svc.Status.LoadBalancer.Ingress {
		ingress := svc.Status.LoadBalancer.Ingress[i]
		provider, err := detectCloudProvider(ingress.Hostname)
		if err != nil {
			klog.Error(err)
			continue
		}
		switch provider {
		case "aws":
			// Get load balancer name and region from hostname
			name, region := cloudaws.GetLBNameFromHostname(ingress.Hostname)
			cloud := cloudaws.NewAWS(region)
			acceleratorArn, retryAfter, err := cloud.EnsureGlobalAccelerator(ctx, svc, &ingress, name, region, correspondence[ingress.Hostname])
			if err != nil {
				return reconcile.Result{}, err
			}
			if retryAfter > 0 {
				return reconcile.Result{
					Requeue:      true,
					RequeueAfter: retryAfter,
				}, nil
			}
			if acceleratorArn != nil {
				correspondence[ingress.Hostname] = *acceleratorArn
			}
		default:
			klog.Warningf("Not implemented for %s", provider)
			continue
		}
	}

	err = c.updateCorrespondence(ctx, correspondence)
	return reconcile.Result{}, err
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

func wasLoadBalancerService(svc *corev1.Service) bool {
	return svc.Spec.Type == corev1.ServiceTypeLoadBalancer
}

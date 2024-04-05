package endpointgroupbinding

import (
	"context"
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	typedcorev1 "k8s.io/client-go/kubernetes/typed/core/v1"
	corelisters "k8s.io/client-go/listers/core/v1"
	networkinglisters "k8s.io/client-go/listers/networking/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/klog/v2"

	endpointgroupbindingv1alpha1 "github.com/h3poteto/aws-global-accelerator-controller/pkg/apis/endpointgroupbinding/v1alpha1"
	ownclientset "github.com/h3poteto/aws-global-accelerator-controller/pkg/client/clientset/versioned"
	ownscheme "github.com/h3poteto/aws-global-accelerator-controller/pkg/client/clientset/versioned/scheme"
	owninformers "github.com/h3poteto/aws-global-accelerator-controller/pkg/client/informers/externalversions"
	ownlisters "github.com/h3poteto/aws-global-accelerator-controller/pkg/client/listers/endpointgroupbinding/v1alpha1"
)

const controllerAgentName = "endpoint-group-binding-controller"

type EndpointGroupBindingConfig struct {
	Workers int
}

type EndpointGroupBindingController struct {
	kubeclient kubernetes.Interface
	client     ownclientset.Interface

	serviceLister              corelisters.ServiceLister
	serviceSynced              cache.InformerSynced
	ingressLister              networkinglisters.IngressLister
	ingressSynced              cache.InformerSynced
	endpointGroupBindingLister ownlisters.EndpointGroupBindingLister
	endpointGroupBindingSynced cache.InformerSynced

	workqueue workqueue.RateLimitingInterface
	recorder  record.EventRecorder
}

// +kubebuilder:rbac:groups=operator.aws.h3poteto.dev,resources=endpointgroupbindings,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=operator.aws.h3poteto.dev,resources=endpointgroupbindings/status,verbs=get;update;patch

func NewEndpointGroupBindingController(kubeclient kubernetes.Interface, ownclientset ownclientset.Interface, informerFactory informers.SharedInformerFactory, ownInformerFactory owninformers.SharedInformerFactory, config *EndpointGroupBindingConfig) *EndpointGroupBindingController {
	eventBroadcaster := record.NewBroadcaster()
	eventBroadcaster.StartLogging(klog.Infof)
	eventBroadcaster.StartRecordingToSink(&typedcorev1.EventSinkImpl{Interface: kubeclient.CoreV1().Events("")})
	recorder := eventBroadcaster.NewRecorder(scheme.Scheme, corev1.EventSource{Component: controllerAgentName})

	err := ownscheme.AddToScheme(scheme.Scheme)
	if err != nil {
		klog.Error(err)
		return nil
	}

	endpoingGroupBindingInformer := ownInformerFactory.Operator().V1alpha1().EndpointGroupBindings()

	controller := &EndpointGroupBindingController{
		kubeclient:                 kubeclient,
		client:                     ownclientset,
		serviceLister:              informerFactory.Core().V1().Services().Lister(),
		serviceSynced:              informerFactory.Core().V1().Services().Informer().HasSynced,
		ingressLister:              informerFactory.Networking().V1().Ingresses().Lister(),
		ingressSynced:              informerFactory.Networking().V1().Ingresses().Informer().HasSynced,
		endpointGroupBindingLister: endpoingGroupBindingInformer.Lister(),
		endpointGroupBindingSynced: endpoingGroupBindingInformer.Informer().HasSynced,
		workqueue:                  workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "EndpointGroupBinding"),
		recorder:                   recorder,
	}

	klog.Info("Setting up event handlers")
	endpoingGroupBindingInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: controller.enqueue,
		UpdateFunc: func(old, new interface{}) {
			// TODO: Block changing spec.EndpointGroupArn field in ValidatingWebhook
			oldEG := old.(*endpointgroupbindingv1alpha1.EndpointGroupBinding)
			newEG := new.(*endpointgroupbindingv1alpha1.EndpointGroupBinding)
			if oldEG.Spec.EndpointGroupArn != newEG.Spec.EndpointGroupArn {
				klog.Error("Do not allow changing EndpointGroupArn field")
				return
			}
			controller.enqueue(new)
		},
	})

	return controller
}

func (c *EndpointGroupBindingController) Run(threadiness int, stopCh <-chan struct{}) error {
	defer utilruntime.HandleCrash()
	defer c.workqueue.ShutDown()

	klog.Info("Starting EndpointGroupBinding controller")

	klog.Info("Waiting for informer caches to sync")
	if ok := cache.WaitForCacheSync(stopCh, c.endpointGroupBindingSynced, c.serviceSynced, c.ingressSynced); !ok {
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

func (c *EndpointGroupBindingController) runWorker() {
	for c.processNextWorkItem() {
	}
}

func (c *EndpointGroupBindingController) processNextWorkItem() bool {
	key, quit := c.workqueue.Get()
	if quit {
		return false
	}
	defer c.workqueue.Done(key)

	err := c.syncHandler(key.(string))
	if err != nil {
		utilruntime.HandleError(err)
		return true
	}

	return true
}

func (c *EndpointGroupBindingController) syncHandler(key string) error {
	ctx := context.Background()
	namespace, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		utilruntime.HandleError(fmt.Errorf("invalid resource key: %s", key))
		return nil
	}

	endpointGroupBinding, err := c.endpointGroupBindingLister.EndpointGroupBindings(namespace).Get(name)
	if err != nil {
		if errors.IsNotFound(err) {
			klog.Infof("EndpointGroupBinding %s has been deleted", key)
			return nil
		}

		return err
	}

	res, err := c.reconcile(ctx, endpointGroupBinding)
	switch {
	case err != nil:
		return err
	case res.RequeueAfter > 0:
		c.workqueue.Forget(key)
		c.workqueue.AddAfter(key, res.RequeueAfter)
		klog.Infof("Successfully synced %q, but requeued after %v", key, res.RequeueAfter)
	case res.Requeue:
		c.workqueue.AddRateLimited(key)
		klog.Infof("Successfully synced %q, but requeued", key)
	default:
		c.workqueue.Forget(key)
		klog.Infof("Successfully synced %q", key)
	}

	return nil
}

func (c *EndpointGroupBindingController) enqueue(obj interface{}) {
	key, err := cache.MetaNamespaceKeyFunc(obj)
	if err != nil {
		utilruntime.HandleError(err)
		return
	}
	c.workqueue.AddRateLimited(key)
}

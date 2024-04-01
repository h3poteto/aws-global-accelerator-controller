package manager

import (
	"context"
	"sync"
	"time"

	clientset "github.com/h3poteto/aws-global-accelerator-controller/pkg/client/clientset/versioned"
	ownInformers "github.com/h3poteto/aws-global-accelerator-controller/pkg/client/informers/externalversions"
	"github.com/h3poteto/aws-global-accelerator-controller/pkg/controller/endpointgroupbinding"
	"github.com/h3poteto/aws-global-accelerator-controller/pkg/controller/globalaccelerator"
	"github.com/h3poteto/aws-global-accelerator-controller/pkg/controller/route53"

	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/klog/v2"
)

type manager struct{}

type ControllerConfig struct {
	GlobalAccelerator    *globalaccelerator.GlobalAcceleratorConfig
	Route53              *route53.Route53Config
	EndpointGroupBinding *endpointgroupbinding.EndpointGroupBindingConfig
}

func NewManager() *manager {
	return &manager{}
}

type InitFunc func(kubeClient kubernetes.Interface, ownClient clientset.Interface, informerFactory informers.SharedInformerFactory, ownInformers ownInformers.SharedInformerFactory, config *ControllerConfig, stopCh <-chan struct{}, done func()) (bool, error)

func NewControllerInitializers() map[string]InitFunc {
	controllers := map[string]InitFunc{}
	controllers["global-accelerator-controller"] = startGlobalAcceleratorController
	controllers["route53-controller"] = startRoute53Controller
	controllers["endpoint-group-binding-controller"] = startEndpointGroupBindingController
	return controllers
}

func (m *manager) Run(ctx context.Context, clientConfig *rest.Config, config *ControllerConfig, stopCh <-chan struct{}) error {
	kubeClient, err := kubernetes.NewForConfig(clientConfig)
	if err != nil {
		return err
	}
	ownClient, err := clientset.NewForConfig(clientConfig)
	if err != nil {
		return err
	}

	informerFactory := informers.NewSharedInformerFactory(kubeClient, time.Second*30)
	ownInformerFactory := ownInformers.NewSharedInformerFactory(ownClient, time.Second*30)

	controllers := NewControllerInitializers()
	var wg sync.WaitGroup
	for name, initFn := range controllers {
		wg.Add(1)
		klog.Infof("Starting %s", name)
		started, err := initFn(kubeClient, ownClient, informerFactory, ownInformerFactory, config, stopCh, wg.Done)
		if err != nil {
			return err
		}
		if !started {
			klog.Warningf("Skippping %s", name)
			continue
		}
		klog.Infof("Started %s", name)
	}

	go informerFactory.Start(stopCh)

	wg.Wait()

	return nil
}

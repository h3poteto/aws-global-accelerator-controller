package manager

import (
	"context"
	"sync"
	"time"

	"github.com/h3poteto/aws-global-accelerator-controller/pkg/controller/globalaccelerator"
	"github.com/h3poteto/aws-global-accelerator-controller/pkg/controller/route53"

	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/klog/v2"
)

type manager struct{}

type ControllerConfig struct {
	GlobalAccelerator *globalaccelerator.GlobalAcceleratorConfig
	Route53           *route53.Route53Config
}

func NewManager() *manager {
	return &manager{}
}

type InitFunc func(kubeClient kubernetes.Interface, informerFactory informers.SharedInformerFactory, config *ControllerConfig, stopCh <-chan struct{}, done func()) (bool, error)

func NewControllerInitializers() map[string]InitFunc {
	controllers := map[string]InitFunc{}
	controllers["global-accelerator-controller"] = startGlobalAcceleratorController
	controllers["route53-controller"] = startRoute53Controller
	return controllers
}

func (m *manager) Run(ctx context.Context, clientConfig *rest.Config, config *ControllerConfig, stopCh <-chan struct{}) error {
	kubeClient, err := kubernetes.NewForConfig(clientConfig)
	if err != nil {
		return err
	}
	informerFactory := informers.NewSharedInformerFactory(kubeClient, time.Second*30)

	controllers := NewControllerInitializers()
	var wg sync.WaitGroup
	for name, initFn := range controllers {
		wg.Add(1)
		klog.Infof("Starting %s", name)
		started, err := initFn(kubeClient, informerFactory, config, stopCh, wg.Done)
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

package manager

import (
	"context"
	"sync"
	"time"

	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/klog/v2"
)

type manager struct{}

func NewManager() *manager {
	return &manager{}
}

type InitFunc func(kubeClient kubernetes.Interface, informerFactory informers.SharedInformerFactory, stopCh <-chan struct{}, done func()) (bool, error)

func NewControllerInitializers() map[string]InitFunc {
	controllers := map[string]InitFunc{}
	controllers["global-accelerator-controller"] = startGlobalAcceleratorController
	return controllers
}

func (m *manager) Run(ctx context.Context, clientConfig *rest.Config, stopCh <-chan struct{}) error {
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
		started, err := initFn(kubeClient, informerFactory, stopCh, wg.Done)
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

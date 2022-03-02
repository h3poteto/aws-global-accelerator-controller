package manager

import (
	"github.com/h3poteto/aws-global-accelerator-controller/pkg/controller/globalaccelerator"

	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
)

func startGlobalAcceleratorController(kubeclient kubernetes.Interface, informerFactory informers.SharedInformerFactory, namespace, region string, stopCh <-chan struct{}, done func()) (bool, error) {
	c := globalaccelerator.NewGlobalAcceleratorController(kubeclient, informerFactory, namespace, region)
	go func() {
		defer done()
		c.Run(2, stopCh)
	}()
	return true, nil
}

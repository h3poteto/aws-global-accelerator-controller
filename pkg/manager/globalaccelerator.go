package manager

import (
	clientset "github.com/h3poteto/aws-global-accelerator-controller/pkg/client/clientset/versioned"
	ownInformers "github.com/h3poteto/aws-global-accelerator-controller/pkg/client/informers/externalversions"
	"github.com/h3poteto/aws-global-accelerator-controller/pkg/controller/globalaccelerator"

	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
)

func startGlobalAcceleratorController(kubeclient kubernetes.Interface, _ clientset.Interface, informerFactory informers.SharedInformerFactory, _ ownInformers.SharedInformerFactory, config *ControllerConfig, stopCh <-chan struct{}, done func()) (bool, error) {
	c := globalaccelerator.NewGlobalAcceleratorController(kubeclient, informerFactory, config.GlobalAccelerator)
	go func() {
		defer done()
		c.Run(config.GlobalAccelerator.Workers, stopCh)
	}()
	return true, nil
}

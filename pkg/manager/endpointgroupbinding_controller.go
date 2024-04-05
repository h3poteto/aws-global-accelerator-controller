package manager

import (
	clientset "github.com/h3poteto/aws-global-accelerator-controller/pkg/client/clientset/versioned"
	ownInformers "github.com/h3poteto/aws-global-accelerator-controller/pkg/client/informers/externalversions"
	"github.com/h3poteto/aws-global-accelerator-controller/pkg/controller/endpointgroupbinding"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
)

func startEndpointGroupBindingController(kubeclient kubernetes.Interface, ownClient clientset.Interface, informerFactory informers.SharedInformerFactory, ownInformerFactory ownInformers.SharedInformerFactory, config *ControllerConfig, stopCh <-chan struct{}, done func()) (bool, error) {
	c := endpointgroupbinding.NewEndpointGroupBindingController(kubeclient, ownClient, informerFactory, ownInformerFactory, config.EndpointGroupBinding)
	go func() {
		defer done()
		c.Run(config.EndpointGroupBinding.Workers, stopCh)
	}()
	return true, nil
}

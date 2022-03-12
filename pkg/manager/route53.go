package manager

import (
	"github.com/h3poteto/aws-global-accelerator-controller/pkg/controller/route53"

	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
)

func startRoute53Controller(kubeClient kubernetes.Interface, informerFactory informers.SharedInformerFactory, config *ControllerConfig, stopCh <-chan struct{}, done func()) (bool, error) {
	c := route53.NewRoute53Controller(kubeClient, informerFactory, config.Route53)
	go func() {
		defer done()
		c.Run(config.Route53.Workers, stopCh)
	}()
	return true, nil
}

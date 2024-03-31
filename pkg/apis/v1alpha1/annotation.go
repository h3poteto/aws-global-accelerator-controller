package apis

const (
	AWSGlobalAcceleratorManagedAnnotation = "aws-global-accelerator-controller.h3poteto.dev/global-accelerator-managed"
	Route53HostnameAnnotation             = "aws-global-accelerator-controller.h3poteto.dev/route53-hostname"
	ClientIPPreservationAnnotation        = "aws-global-accelerator-controller.h3poteto.dev/client-ip-preservation"

	AWSLoadBalancerTypeAnnotation = "service.beta.kubernetes.io/aws-load-balancer-type"
	IngressClassAnnotation        = "kubernetes.io/ingress.class"
)

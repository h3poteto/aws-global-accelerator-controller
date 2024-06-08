package fixture

import endpointgroupbindingv1alpha1 "github.com/h3poteto/aws-global-accelerator-controller/pkg/apis/endpointgroupbinding/v1alpha1"

func EndpointGroupBinding(clientIPPreservation bool, service string, weight *int32, arn string) endpointgroupbindingv1alpha1.EndpointGroupBinding {
	return endpointgroupbindingv1alpha1.EndpointGroupBinding{
		Spec: endpointgroupbindingv1alpha1.EndpointGroupBindingSpec{
			EndpointGroupArn:     arn,
			ClientIPPreservation: clientIPPreservation,
			Weight:               weight,
			ServiceRef: &endpointgroupbindingv1alpha1.ServiceReference{
				Name: service,
			},
		},
	}
}

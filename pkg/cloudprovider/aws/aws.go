package aws

import (
	"context"

	"github.com/aws/aws-sdk-go-v2/config"
	elbv2 "github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2"
	"github.com/aws/aws-sdk-go-v2/service/globalaccelerator"
	"github.com/aws/aws-sdk-go-v2/service/route53"
)

type AWS struct {
	lb      *elbv2.Client
	ga      *globalaccelerator.Client
	route53 *route53.Client
}

func NewAWS(region string) (*AWS, error) {
	cfg, err := config.LoadDefaultConfig(context.Background())
	if err != nil {
		return nil, err
	}
	lb := elbv2.NewFromConfig(cfg, func(o *elbv2.Options) {
		o.Region = region
	})
	// Global Accelerator requires us-west-2 region, because it is global object.
	ga := globalaccelerator.NewFromConfig(cfg, func(o *globalaccelerator.Options) {
		o.Region = "us-west-2"
	})
	route53 := route53.NewFromConfig(cfg, func(o *route53.Options) {
		o.Region = "us-west-2"
	})
	return &AWS{
		lb:      lb,
		ga:      ga,
		route53: route53,
	}, nil
}

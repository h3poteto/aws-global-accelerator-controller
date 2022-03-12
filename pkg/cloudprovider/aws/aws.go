package aws

import (
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/elbv2"
	"github.com/aws/aws-sdk-go/service/elbv2/elbv2iface"
	"github.com/aws/aws-sdk-go/service/globalaccelerator"
	"github.com/aws/aws-sdk-go/service/globalaccelerator/globalacceleratoriface"
	"github.com/aws/aws-sdk-go/service/route53"
	"github.com/aws/aws-sdk-go/service/route53/route53iface"
)

type AWS struct {
	lb      elbv2iface.ELBV2API
	ga      globalacceleratoriface.GlobalAcceleratorAPI
	route53 route53iface.Route53API
}

func NewAWS(region string) *AWS {
	sess := session.Must(session.NewSessionWithOptions(session.Options{
		SharedConfigState: session.SharedConfigEnable,
	}))
	lb := elbv2.New(sess, aws.NewConfig().WithRegion(region))
	// Global Accelerator requires us-west-2 region, because it is global object.
	ga := globalaccelerator.New(sess, aws.NewConfig().WithRegion("us-west-2"))
	route53 := route53.New(sess, aws.NewConfig().WithRegion("us-west-2"))
	return &AWS{
		lb:      lb,
		ga:      ga,
		route53: route53,
	}
}

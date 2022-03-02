package aws

import (
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/elbv2"
	"github.com/aws/aws-sdk-go/service/elbv2/elbv2iface"
	"github.com/aws/aws-sdk-go/service/globalaccelerator"
	"github.com/aws/aws-sdk-go/service/globalaccelerator/globalacceleratoriface"
)

type AWS struct {
	lb elbv2iface.ELBV2API
	ga globalacceleratoriface.GlobalAcceleratorAPI
}

func NewAWS(region string) *AWS {
	sess := session.Must(session.NewSessionWithOptions(session.Options{
		SharedConfigState: session.SharedConfigEnable,
	}))
	lb := elbv2.New(sess, aws.NewConfig().WithRegion(region))
	ga := globalaccelerator.New(sess, aws.NewConfig().WithRegion("us-west-2"))
	return &AWS{
		lb: lb,
		ga: ga,
	}
}

func findRegion() string {
	// TODO
	return "ap-northeast-1"
}

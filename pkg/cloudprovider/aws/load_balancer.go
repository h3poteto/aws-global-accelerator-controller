package aws

import (
	"context"
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/elbv2"
)

func (a *AWS) getLoadBalancer(ctx context.Context, name string) (*elbv2.LoadBalancer, error) {
	input := &elbv2.DescribeLoadBalancersInput{
		Names: []*string{
			aws.String(name),
		},
	}
	res, err := a.lb.DescribeLoadBalancersWithContext(ctx, input)
	if err != nil {
		return nil, err
	}
	for _, lb := range res.LoadBalancers {
		if *lb.LoadBalancerName == name {
			return lb, nil
		}
	}

	return nil, fmt.Errorf("Could not find LoadBalancer: %s", name)
}

func GetLBNameFromHostname(hostname string) (string, string) {
	slice := strings.Split(hostname, ".")
	subdomain := slice[0]
	region := slice[2]
	slice = strings.Split(subdomain, "-")
	return slice[0], region
}

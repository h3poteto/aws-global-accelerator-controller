package aws

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/elbv2"
)

func (a *AWS) GetLoadBalancer(ctx context.Context, name string) (*elbv2.LoadBalancer, error) {
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

func GetLBNameFromHostname(hostname string) (string, string, error) {
	albReg := regexp.MustCompile(`\.elb\.amazonaws\.com$`)
	nlbReg := regexp.MustCompile(`\.elb\..+\.amazonaws\.com$`)
	switch {
	case albReg.MatchString(hostname):
		return matchALBHostname(hostname)
	case nlbReg.MatchString(hostname):
		return matchNLBHostname(hostname)
	default:
		return "", "", fmt.Errorf("%s is not Elastic Load Balancer", hostname)
	}

}

func matchALBHostname(hostname string) (string, string, error) {
	slice := strings.Split(hostname, ".")
	subdomain := slice[0]
	region := slice[1]
	publicReg := regexp.MustCompile(`^internal-`)
	if publicReg.MatchString(subdomain) {
		name, err := internalALBName(subdomain)
		return name, region, err
	} else {
		name, err := publicALBName(subdomain)
		return name, region, err
	}
}

func internalALBName(subdomain string) (string, error) {
	nameReg := regexp.MustCompile(`^internal\-([\w\-]+)\-[\w]+$`)
	result := nameReg.FindAllStringSubmatch(subdomain, -1)
	if len(result) != 1 || len(result[0]) != 2 {
		return "", fmt.Errorf("Failed to parse subdomain for internal ALB: %s", subdomain)
	}
	return result[0][1], nil
}

func publicALBName(subdomain string) (string, error) {
	nameReg := regexp.MustCompile(`^([\w\-]+)\-[\w]+$`)
	result := nameReg.FindAllStringSubmatch(subdomain, -1)
	if len(result) != 1 || len(result[0]) != 2 {
		return "", fmt.Errorf("Failed to parse subdomain for public ALB: %s", subdomain)
	}
	return result[0][1], nil
}

func matchNLBHostname(hostname string) (string, string, error) {
	slice := strings.Split(hostname, ".")
	subdomain := slice[0]
	region := slice[2]
	name, err := nlbName(subdomain)
	return name, region, err
}

func nlbName(subdomain string) (string, error) {
	nameReg := regexp.MustCompile(`^([\w\-]+)\-[\w]+$`)
	result := nameReg.FindAllStringSubmatch(subdomain, -1)
	if len(result) != 1 || len(result[0]) != 2 {
		return "", fmt.Errorf("Failed to parse subdomain for NLB: %s", subdomain)
	}
	return result[0][1], nil
}

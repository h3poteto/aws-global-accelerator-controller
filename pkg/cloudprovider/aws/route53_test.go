package aws

import (
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	gatypes "github.com/aws/aws-sdk-go-v2/service/globalaccelerator/types"
	route53types "github.com/aws/aws-sdk-go-v2/service/route53/types"
	"github.com/stretchr/testify/assert"
)

func TestFindARecord(t *testing.T) {
	cases := []struct {
		title    string
		records  []route53types.ResourceRecordSet
		hostname string
		expected *route53types.ResourceRecordSet
	}{
		{
			title: "Does not contain A record",
			records: []route53types.ResourceRecordSet{
				route53types.ResourceRecordSet{
					Name: aws.String("foo.example.com."),
					Type: route53types.RRTypeCname,
				},
				route53types.ResourceRecordSet{
					Name: aws.String("bar.example.com."),
					Type: route53types.RRTypeCname,
				},
			},
			hostname: "foo.example.com",
			expected: nil,
		},
		{
			title: "Does not contain hostname",
			records: []route53types.ResourceRecordSet{
				route53types.ResourceRecordSet{
					Name: aws.String("foo.example.com."),
					Type: route53types.RRTypeA,
				},
				route53types.ResourceRecordSet{
					Name: aws.String("bar.example.com."),
					Type: route53types.RRTypeA,
				},
			},
			hostname: "baz.example.com",
			expected: nil,
		},
		{
			title: "Contains hostname",
			records: []route53types.ResourceRecordSet{
				route53types.ResourceRecordSet{
					Name: aws.String("foo.example.com."),
					Type: route53types.RRTypeA,
				},
				route53types.ResourceRecordSet{
					Name: aws.String("bar.example.com."),
					Type: route53types.RRTypeA,
				},
			},
			hostname: "bar.example.com",
			expected: &route53types.ResourceRecordSet{
				Name: aws.String("bar.example.com."),
				Type: route53types.RRTypeA,
			},
		},
		{
			title: "Contains wildcard record",
			records: []route53types.ResourceRecordSet{
				route53types.ResourceRecordSet{
					Name: aws.String("\\052.example.com."),
					Type: route53types.RRTypeA,
				},
				route53types.ResourceRecordSet{
					Name: aws.String("bar.example.com."),
					Type: route53types.RRTypeA,
				},
			},
			hostname: "*.example.com",
			expected: &route53types.ResourceRecordSet{
				Name: aws.String("\\052.example.com."),
				Type: route53types.RRTypeA,
			},
		},
	}
	for _, c := range cases {
		t.Run(c.title, func(tt *testing.T) {
			result := findARecord(c.records, c.hostname)
			assert.Equal(tt, c.expected, result)
		})
	}
}

func TestNeedRecordsUpdate(t *testing.T) {
	cases := []struct {
		title       string
		record      *route53types.ResourceRecordSet
		accelerator *gatypes.Accelerator
		expected    bool
	}{
		{
			title: "Alias is nil",
			record: &route53types.ResourceRecordSet{
				Name: aws.String("foo.example.com"),
			},
			accelerator: &gatypes.Accelerator{},
			expected:    true,
		},
		{
			title: "Alias DNS name is not matched",
			record: &route53types.ResourceRecordSet{
				Name: aws.String("foo.example.com"),
				AliasTarget: &route53types.AliasTarget{
					DNSName: aws.String("foo.example.com."),
				},
			},
			accelerator: &gatypes.Accelerator{
				DnsName: aws.String("bar.example.com"),
			},
			expected: true,
		},
		{
			title: "Alias DNS name is matched",
			record: &route53types.ResourceRecordSet{
				Name: aws.String("foo.example.com"),
				AliasTarget: &route53types.AliasTarget{
					DNSName: aws.String("foo.example.com."),
				},
			},
			accelerator: &gatypes.Accelerator{
				DnsName: aws.String("foo.example.com"),
			},
			expected: false,
		},
	}
	for _, c := range cases {
		t.Run(c.title, func(tt *testing.T) {
			result := needRecordsUpdate(c.record, c.accelerator)
			assert.Equal(tt, c.expected, result)
		})
	}
}

func TestParentDomain(t *testing.T) {
	cases := []struct {
		title    string
		hostname string
		expected string
	}{
		{
			title:    "Hostname is subdomain",
			hostname: "h3poteto-test.example.com",
			expected: "example.com",
		},
		{
			title:    "Hostname is sub-subdomain",
			hostname: "h3poteto-test.foo.example.com",
			expected: "foo.example.com",
		},
		{
			title:    "Hostname is domain",
			hostname: "example.com",
			expected: "com",
		},
		{
			title:    "Hostname is top-level domain",
			hostname: "com",
			expected: "",
		},
		{
			title:    "Hostname is a dot",
			hostname: ".",
			expected: "",
		},
	}

	for _, c := range cases {
		t.Run(c.title, func(tt *testing.T) {
			result := parentDomain(c.hostname)
			assert.Equal(tt, c.expected, result)
		})
	}
}

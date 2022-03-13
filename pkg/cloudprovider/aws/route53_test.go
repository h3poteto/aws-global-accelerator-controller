package aws

import (
	"testing"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/globalaccelerator"
	"github.com/aws/aws-sdk-go/service/route53"
	"github.com/stretchr/testify/assert"
)

func TestFindARecord(t *testing.T) {
	cases := []struct {
		title    string
		records  []*route53.ResourceRecordSet
		hostname string
		expected *route53.ResourceRecordSet
	}{
		{
			title: "Does not contain A record",
			records: []*route53.ResourceRecordSet{
				&route53.ResourceRecordSet{
					Name: aws.String("foo.example.com."),
					Type: aws.String(route53.RRTypeCname),
				},
				&route53.ResourceRecordSet{
					Name: aws.String("bar.example.com."),
					Type: aws.String(route53.RRTypeCname),
				},
			},
			hostname: "foo.example.com",
			expected: nil,
		},
		{
			title: "Does not contain hostname",
			records: []*route53.ResourceRecordSet{
				&route53.ResourceRecordSet{
					Name: aws.String("foo.example.com."),
					Type: aws.String(route53.RRTypeA),
				},
				&route53.ResourceRecordSet{
					Name: aws.String("bar.example.com."),
					Type: aws.String(route53.RRTypeA),
				},
			},
			hostname: "baz.example.com",
			expected: nil,
		},
		{
			title: "Contains hostname",
			records: []*route53.ResourceRecordSet{
				&route53.ResourceRecordSet{
					Name: aws.String("foo.example.com."),
					Type: aws.String(route53.RRTypeA),
				},
				&route53.ResourceRecordSet{
					Name: aws.String("bar.example.com."),
					Type: aws.String(route53.RRTypeA),
				},
			},
			hostname: "bar.example.com",
			expected: &route53.ResourceRecordSet{
				Name: aws.String("bar.example.com."),
				Type: aws.String(route53.RRTypeA),
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
		record      *route53.ResourceRecordSet
		accelerator *globalaccelerator.Accelerator
		expected    bool
	}{
		{
			title: "Alias is nil",
			record: &route53.ResourceRecordSet{
				Name: aws.String("foo.example.com"),
			},
			accelerator: &globalaccelerator.Accelerator{},
			expected:    true,
		},
		{
			title: "Alias DNS name is not matched",
			record: &route53.ResourceRecordSet{
				Name: aws.String("foo.example.com"),
				AliasTarget: &route53.AliasTarget{
					DNSName: aws.String("foo.example.com."),
				},
			},
			accelerator: &globalaccelerator.Accelerator{
				DnsName: aws.String("bar.example.com"),
			},
			expected: true,
		},
		{
			title: "Alias DNS name is matched",
			record: &route53.ResourceRecordSet{
				Name: aws.String("foo.example.com"),
				AliasTarget: &route53.AliasTarget{
					DNSName: aws.String("foo.example.com."),
				},
			},
			accelerator: &globalaccelerator.Accelerator{
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
	}

	for _, c := range cases {
		t.Run(c.title, func(tt *testing.T) {
			result := parentDomain(c.hostname)
			assert.Equal(tt, c.expected, result)
		})
	}
}

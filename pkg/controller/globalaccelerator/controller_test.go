package globalaccelerator

import (
	"log"
	"testing"
)

func TestDetectCloudProvider(t *testing.T) {
	cases := []struct {
		title            string
		hostname         string
		expectedProvider string
	}{
		{
			title:            "NetworkLoadBalancer in AWS",
			hostname:         "aa5849cde256f49faa7487bb433155b7-3f43353a6cb6f633.elb.ap-northeast-1.amazonaws.com",
			expectedProvider: "aws",
		},
	}
	for _, c := range cases {
		log.Printf("Running CASE: %s", c.title)
		provider, err := detectCloudProvider(c.hostname)
		if err != nil {
			t.Errorf("CASE %s: error has occur: %v", c.title, err)
			continue
		}
		if provider != c.expectedProvider {
			t.Errorf("CASE %s: provider is not matched, expected %s, but actual %s", c.title, c.expectedProvider, provider)
		}
	}

}

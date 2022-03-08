package aws

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetLBNameFromHostname(t *testing.T) {
	cases := []struct {
		title          string
		hostname       string
		expectedName   string
		expectedRegion string
	}{
		{
			title:          "ELB is public NLB",
			hostname:       "aa5849cde256f49faa7487bb433155b7-3f43353a6cb6f633.elb.ap-northeast-1.amazonaws.com",
			expectedName:   "aa5849cde256f49faa7487bb433155b7",
			expectedRegion: "ap-northeast-1",
		},
		{
			title:          "ELB is internal NLB",
			hostname:       "test-b6cdc5fbd1d6fa43.elb.ap-northeast-1.amazonaws.com",
			expectedName:   "test",
			expectedRegion: "ap-northeast-1",
		},
		{
			title:          "ELB is public ALB",
			hostname:       "k8s-default-h3poteto-f1f41628db-201899272.ap-northeast-1.elb.amazonaws.com",
			expectedName:   "k8s-default-h3poteto-f1f41628db",
			expectedRegion: "ap-northeast-1",
		},
		{
			title:          "ELB is internal ALB",
			hostname:       "internal-k8s-default-h3poteto-35ca57562f-777774719.ap-northeast-1.elb.amazonaws.com",
			expectedName:   "k8s-default-h3poteto-35ca57562f",
			expectedRegion: "ap-northeast-1",
		},
	}

	for _, c := range cases {
		t.Run(c.title, func(tt *testing.T) {
			name, region, err := GetLBNameFromHostname(c.hostname)
			assert.NoError(tt, err)
			assert.Equal(tt, c.expectedName, name)
			assert.Equal(tt, c.expectedRegion, region)
		})
	}
}

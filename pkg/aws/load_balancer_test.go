package aws

import "testing"

func TestGetLBNameFromHostname(t *testing.T) {
	dns := "aa5849cde256f49faa7487bb433155b7-3f43353a6cb6f633.elb.ap-northeast-1.amazonaws.com"
	name := "aa5849cde256f49faa7487bb433155b7"
	region := "ap-northeast-1"
	n, r := GetLBNameFromHostname(dns)
	if n != name {
		t.Errorf("Expect %v, but actual %v", name, n)
	}
	if r != region {
		t.Errorf("Expect %v, but actual %v", region, n)
	}
}

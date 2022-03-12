package cloudprovider

import (
	"fmt"
	"strings"
)

func DetectCloudProvider(hostname string) (string, error) {
	parts := strings.Split(hostname, ".")
	domain := parts[len(parts)-2] + "." + parts[len(parts)-1]
	switch domain {
	case "amazonaws.com":
		return "aws", nil
	default:
		return "", fmt.Errorf("Unknown cloud provider: %s", domain)
	}
}

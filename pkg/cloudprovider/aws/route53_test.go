package aws

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

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

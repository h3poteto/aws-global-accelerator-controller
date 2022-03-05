package errors

import (
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsNoRetry(t *testing.T) {
	cases := []struct {
		title    string
		err      error
		expected bool
	}{
		{
			title: "Error is NoRetryError",
			err: &NoRetryError{
				msg: "hoge",
			},
			expected: true,
		},
		{
			title: "Error is NoRetryError with wrapped",
			err: fmt.Errorf("my error %w", &NoRetryError{
				msg: "hoge",
			}),
			expected: true,
		},
		{
			title:    "Error is not NoRetryError",
			err:      errors.New("my error"),
			expected: false,
		},
	}

	for _, c := range cases {
		t.Run(c.title, func(tt *testing.T) {
			actual := IsNoRetry(c.err)
			assert.Equal(tt, c.expected, actual)
		})
	}
}

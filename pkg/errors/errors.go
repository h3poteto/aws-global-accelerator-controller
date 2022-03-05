package errors

import (
	"errors"
	"fmt"
)

type NoRetryError struct {
	msg string
	err error
}

func NewNoRetryError(msg string) *NoRetryError {
	return &NoRetryError{
		msg: msg,
	}
}

func NewNoRetryErrorf(format string, v ...interface{}) *NoRetryError {
	return &NoRetryError{
		msg: fmt.Sprintf(format, v...),
	}
}

func (e *NoRetryError) Error() string {
	return e.msg
}

func (e *NoRetryError) Unwrap() error {
	return e.err
}

func IsNoRetry(err error) bool {
	noRetry := &NoRetryError{}
	if errors.As(err, &noRetry) {
		return true
	}
	return false
}

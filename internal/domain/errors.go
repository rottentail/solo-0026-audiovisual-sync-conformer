package domain

import (
	"errors"
	"fmt"
)

// Error is the stable failure contract shared by media, service, and transport.
type Error struct {
	Code    string
	Message string
	Status  int
}

func (e *Error) Error() string {
	if e == nil {
		return ""
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

func NewError(code string, status int, format string, args ...any) *Error {
	return &Error{
		Code:    code,
		Message: fmt.Sprintf(format, args...),
		Status:  status,
	}
}

func AsError(err error) *Error {
	if err == nil {
		return nil
	}
	var target *Error
	if errors.As(err, &target) {
		return target
	}
	return NewError("internal_error", 500, "unexpected service failure")
}

func IsErrorCode(err error, code string) bool {
	target := AsError(err)
	return target != nil && target.Code == code
}

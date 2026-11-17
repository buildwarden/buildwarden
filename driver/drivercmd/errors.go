package drivercmd

import (
	"errors"
	"fmt"

	"github.com/buildwarden/buildwarden/driver"
)

// The codes of an error object (v1.md, Errors).
const (
	CodeFixable             = "fixable"
	CodeBusy                = "busy"
	CodeInvalidRequest      = "invalid-request"
	CodeUnsupportedCommand  = "unsupported-command"
	CodeUnsupportedContract = "unsupported-contract"
	CodeInternal            = "internal"
)

// Error is the contract's error object. A driver function may return one to
// choose its code, such as busy; Main maps any other error with ErrorObject.
type Error struct {
	Code    string         `json:"code"`
	Msg     string         `json:"msg"`
	Fix     string         `json:"fix,omitempty"`
	Details map[string]any `json:"details,omitempty"`
}

// Error is the message.
func (e *Error) Error() string { return e.Msg }

// ErrorObject maps err to the error object the contract sends: an *Error as
// it is, a *driver.FixableError as fixable with its fix, and anything else
// as internal.
func ErrorObject(err error) *Error {
	var e *Error
	if errors.As(err, &e) {
		return e
	}
	var fe *driver.FixableError
	if errors.As(err, &fe) {
		return &Error{Code: CodeFixable, Msg: fe.Msg, Fix: fe.Fix}
	}
	return &Error{Code: CodeInternal, Msg: err.Error()}
}

// invalid is an invalid-request error, whose message is formatted from
// format and args.
func invalid(format string, args ...any) *Error {
	return &Error{Code: CodeInvalidRequest, Msg: fmt.Sprintf(format, args...)}
}

// exitCode is the contract's exit code for an error of code (DRV-21).
func exitCode(code string) int {
	switch code {
	case CodeInvalidRequest:
		return 2
	case CodeUnsupportedCommand:
		return 3
	case CodeUnsupportedContract:
		return 4
	default:
		return 1
	}
}

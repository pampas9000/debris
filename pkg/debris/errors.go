package debris

import (
	"errors"
	"fmt"
)

// Standard machine-readable error codes returned by the Debris SDK and JSON-RPC layer.
const (
	CodeInvalidRequest    = "INVALID_REQUEST"
	CodeUnsupportedFormat = "UNSUPPORTED_FORMAT"
	CodeNotFound          = "NOT_FOUND"
	CodeProbeFailed       = "PROBE_FAILED"
	CodeNotImplemented    = "NOT_IMPLEMENTED"
	CodeCanceled          = "CANCELED"
	CodeIO                = "IO_ERROR"
	CodeInternal          = "INTERNAL_ERROR"
)

// Error is a structured, JSON-serializable error suitable for both Go SDK
// callers and stdio JSON-RPC responses.
type Error struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Detail  string `json:"detail,omitempty"`
	Cause   error  `json:"-"`
}

func (e *Error) Error() string {
	if e == nil {
		return "<nil>"
	}
	if e.Detail != "" {
		return fmt.Sprintf("%s: %s (%s)", e.Code, e.Message, e.Detail)
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

func (e *Error) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}

// NewError constructs a structured Error with the given code, message, and detail.
func NewError(code, message, detail string) *Error {
	return &Error{
		Code:    code,
		Message: message,
		Detail:  detail,
	}
}

// WrapError wraps an underlying error into a structured Error.
func WrapError(code, message string, err error) *Error {
	var detail string
	if err != nil {
		detail = err.Error()
	}
	return &Error{
		Code:    code,
		Message: message,
		Detail:  detail,
		Cause:   err,
	}
}

// AsError extracts a *Error from err, or wraps err as CodeInternal if it is
// not already a *Error. Returns nil when err is nil.
func AsError(err error) *Error {
	if err == nil {
		return nil
	}
	var structured *Error
	if errors.As(err, &structured) {
		return structured
	}
	return WrapError(CodeInternal, err.Error(), err)
}

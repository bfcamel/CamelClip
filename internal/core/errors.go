package core

import (
	"context"
	"errors"
	"net"
)

type ErrorCode string

const (
	CodeBackend       ErrorCode = "backend"
	CodeContract      ErrorCode = "adapter_contract"
	CodeValidation    ErrorCode = "validation"
	CodeConflict      ErrorCode = "conflict"
	CodeSourceChanged ErrorCode = "source_changed"
	CodeCanceled      ErrorCode = "canceled"
	CodeNotFound      ErrorCode = "not_found"
)

// Failure separates stable machine-readable behavior from human diagnostics.
type Failure struct {
	Code      ErrorCode
	Message   string
	Temporary bool
	Cause     error
}

func (e *Failure) Error() string {
	if e.Message != "" {
		return e.Message
	}
	return string(e.Code)
}
func (e *Failure) Unwrap() error          { return e.Cause }
func (e *Failure) Retryable() bool        { return e.Temporary }
func (e *Failure) FailureCode() ErrorCode { return e.Code }
func CodeOf(err error) ErrorCode {
	if err == nil {
		return ""
	}
	var coded interface{ FailureCode() ErrorCode }
	if errors.As(err, &coded) {
		return coded.FailureCode()
	}
	if errors.Is(err, ErrCanceled) || errors.Is(err, context.Canceled) {
		return CodeCanceled
	}
	return CodeBackend
}
func Retryable(err error) bool {
	if err == nil || errors.Is(err, context.Canceled) || errors.Is(err, ErrCanceled) {
		return false
	}
	var known interface{ Retryable() bool }
	if errors.As(err, &known) {
		return known.Retryable()
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	var n net.Error
	return errors.As(err, &n) && n.Timeout()
}
func ContractError(message string) error { return &Failure{Code: CodeContract, Message: message} }

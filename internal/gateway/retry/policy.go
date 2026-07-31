package retry

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"
)

type ErrorClass string

const (
	ErrorClassLocalValidation  ErrorClass = "local_validation"
	ErrorClassModelNotFound    ErrorClass = "model_not_found"
	ErrorClassConfigInvalid    ErrorClass = "config_invalid"
	ErrorClassAuthInvalid      ErrorClass = "auth_invalid"
	ErrorClassPermissionDenied ErrorClass = "permission_denied"
	ErrorClassRateLimited      ErrorClass = "rate_limited"
	ErrorClassTransient        ErrorClass = "transient"
	ErrorClassContextWindow    ErrorClass = "context_window_exceeded"
	ErrorClassRequestTooLarge  ErrorClass = "request_too_large"
	ErrorClassClientCanceled   ErrorClass = "client_canceled"
	ErrorClassDeadlineExceeded ErrorClass = "deadline_exceeded"
	ErrorClassProtocol         ErrorClass = "protocol_error"
	ErrorClassUnknown          ErrorClass = "unknown"
)

type UpstreamError struct {
	Class       ErrorClass
	StatusCode  int
	RetryAfter  time.Duration
	Local       bool
	Cause       error
	Description string
}

func (e *UpstreamError) Error() string {
	if e == nil {
		return ""
	}
	if e.Description != "" {
		return e.Description
	}
	if e.Cause != nil {
		return e.Cause.Error()
	}
	return string(e.Class)
}

func (e *UpstreamError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}

type DecisionContext struct {
	Attempt          int
	MaxAttempts      int
	OutputStarted    bool
	UpstreamAccepted bool
}

type Decision struct {
	Retry     bool
	Cooldown  bool
	Delay     time.Duration
	FinalCode int
	Reason    string
}

type Policy struct {
	BaseCooldown time.Duration
	MaxCooldown  time.Duration
}

func DefaultPolicy() Policy {
	return Policy{
		BaseCooldown: time.Second,
		MaxCooldown:  30 * time.Second,
	}
}

func (p Policy) Decide(err *UpstreamError, decisionContext DecisionContext) Decision {
	if err == nil {
		return Decision{FinalCode: http.StatusOK, Reason: "success"}
	}

	finalCode := err.StatusCode
	if finalCode == 0 {
		finalCode = http.StatusBadGateway
	}
	if decisionContext.OutputStarted {
		return Decision{FinalCode: finalCode, Reason: "output_started"}
	}
	if decisionContext.UpstreamAccepted {
		return Decision{FinalCode: finalCode, Reason: "upstream_accepted"}
	}
	if decisionContext.MaxAttempts <= 0 || decisionContext.Attempt >= decisionContext.MaxAttempts {
		return Decision{FinalCode: finalCode, Reason: "attempts_exhausted"}
	}

	switch err.Class {
	case ErrorClassAuthInvalid, ErrorClassPermissionDenied:
		return Decision{Retry: true, Cooldown: true, Delay: p.cooldown(err), FinalCode: finalCode, Reason: string(err.Class)}
	case ErrorClassRateLimited:
		return Decision{Retry: true, Cooldown: true, Delay: p.cooldown(err), FinalCode: finalCode, Reason: string(err.Class)}
	case ErrorClassTransient, ErrorClassDeadlineExceeded, ErrorClassRequestTooLarge:
		return Decision{Retry: true, Cooldown: true, Delay: p.cooldown(err), FinalCode: finalCode, Reason: string(err.Class)}
	case ErrorClassLocalValidation, ErrorClassModelNotFound, ErrorClassConfigInvalid,
		ErrorClassContextWindow, ErrorClassClientCanceled, ErrorClassProtocol:
		return Decision{FinalCode: finalCode, Reason: string(err.Class)}
	default:
		return Decision{FinalCode: finalCode, Reason: string(ErrorClassUnknown)}
	}
}

func (p Policy) cooldown(err *UpstreamError) time.Duration {
	delay := err.RetryAfter
	if delay <= 0 {
		delay = p.BaseCooldown
	}
	if p.MaxCooldown > 0 && delay > p.MaxCooldown {
		return p.MaxCooldown
	}
	return delay
}

func ClassifyHTTP(statusCode int, cause error) *UpstreamError {
	if errors.Is(cause, context.Canceled) {
		return &UpstreamError{
			Class:      ErrorClassClientCanceled,
			StatusCode: statusCode,
			Cause:      cause,
		}
	}
	if errors.Is(cause, context.DeadlineExceeded) {
		return &UpstreamError{
			Class:      ErrorClassDeadlineExceeded,
			StatusCode: statusCode,
			Cause:      cause,
		}
	}

	class := ErrorClassUnknown
	switch statusCode {
	case http.StatusBadRequest, http.StatusUnprocessableEntity:
		class = ErrorClassLocalValidation
	case http.StatusUnauthorized:
		class = ErrorClassAuthInvalid
	case http.StatusForbidden, http.StatusPaymentRequired:
		class = ErrorClassPermissionDenied
	case http.StatusNotFound:
		class = ErrorClassModelNotFound
	case http.StatusRequestEntityTooLarge:
		class = ErrorClassRequestTooLarge
	case http.StatusTooManyRequests:
		class = ErrorClassRateLimited
	case http.StatusRequestTimeout:
		class = ErrorClassDeadlineExceeded
	default:
		if statusCode == 529 || statusCode >= http.StatusInternalServerError {
			class = ErrorClassTransient
		}
	}

	return &UpstreamError{
		Class:       class,
		StatusCode:  statusCode,
		Cause:       cause,
		Description: fmt.Sprintf("upstream request failed with status %d", statusCode),
	}
}

package transport

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strings"
)

// ErrorKind is the client-side meaning of a failed request. The UI switches on
// it instead of parsing human-readable messages.
type ErrorKind string

const (
	KindNetwork      ErrorKind = "network"
	KindTimeout      ErrorKind = "timeout"
	KindUnauthorized ErrorKind = "unauthorized"
	KindForbidden    ErrorKind = "forbidden"
	KindNotFound     ErrorKind = "not_found"
	KindConflict     ErrorKind = "conflict"
	KindUnavailable  ErrorKind = "unavailable"
	KindUnsupported  ErrorKind = "unsupported"
	KindInvalid      ErrorKind = "invalid"
	KindServer       ErrorKind = "server"
	KindCanceled     ErrorKind = "canceled"
)

// APIError is one failed HTTP exchange. Code is the desktop error code when
// the body carries one, so callers never depend on the message text.
type APIError struct {
	Status  int
	Code    string
	Message string
	Kind    ErrorKind
	Err     error
}

func (e *APIError) Error() string {
	if e == nil {
		return "<nil>"
	}
	parts := make([]string, 0, 3)
	if e.Status > 0 {
		parts = append(parts, fmt.Sprintf("HTTP %d", e.Status))
	}
	if e.Code != "" {
		parts = append(parts, e.Code)
	}
	if e.Message != "" {
		parts = append(parts, e.Message)
	}
	if len(parts) == 0 && e.Err != nil {
		return e.Err.Error()
	}
	if e.Err != nil {
		return strings.Join(parts, ": ") + ": " + e.Err.Error()
	}
	return strings.Join(parts, ": ")
}

// Unwrap exposes the transport failure, if any.
func (e *APIError) Unwrap() error { return e.Err }

// KindOf returns the error's kind, defaulting to network for unknown errors.
func KindOf(err error) ErrorKind {
	var api *APIError
	if errors.As(err, &api) {
		return api.Kind
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return KindCanceled
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return KindTimeout
	}
	return KindNetwork
}

// IsUnauthorized reports a session that must be re-paired.
func IsUnauthorized(err error) bool { return KindOf(err) == KindUnauthorized }

// IsUnsupported reports an endpoint the desktop build does not expose.
func IsUnsupported(err error) bool { return KindOf(err) == KindUnsupported }

func classifyStatus(status int, code, message string) ErrorKind {
	switch {
	case status == 401:
		return KindUnauthorized
	case status == 403:
		return KindForbidden
	case status == 404:
		return KindNotFound
	case status == 409:
		return KindConflict
	case status == 503:
		return KindUnavailable
	case status >= 400 && status < 500:
		if code == "NO_ACTIVE_LIBRARY" {
			return KindUnavailable
		}
		return KindInvalid
	default:
		return KindServer
	}
}

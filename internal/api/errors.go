package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"

	"github.com/Bookie-Breaker/bookie-breaker-cli/internal/ui"
)

// Exit codes returned by the CLI.
const (
	ExitOK         = 0
	ExitAPIError   = 1
	ExitUsage      = 2
	ExitConnection = 3
)

// APIError is a structured error returned by a backend service, decoded
// from the standard `{"error": {"code", "message", "details"}, "meta"}`
// envelope or a FastAPI validation payload.
type APIError struct {
	StatusCode int
	Code       string
	Message    string
	Details    map[string]any
}

func (e *APIError) Error() string {
	msg := e.Message
	if msg == "" {
		msg = http.StatusText(e.StatusCode)
	}
	if e.Code != "" {
		return fmt.Sprintf("%s: %s", e.Code, msg)
	}
	return fmt.Sprintf("HTTP %d: %s", e.StatusCode, msg)
}

// UsageError marks command-line usage mistakes so Execute can map them to
// exit code 2.
type UsageError struct {
	Err error
}

func (e *UsageError) Error() string { return e.Err.Error() }
func (e *UsageError) Unwrap() error { return e.Err }

// errorEnvelope is the shared `{"error": {...}}` failure envelope.
type errorEnvelope struct {
	Error struct {
		Code    string         `json:"code"`
		Message string         `json:"message"`
		Details map[string]any `json:"details"`
	} `json:"error"`
}

// validationEnvelope is FastAPI's 422 validation payload.
type validationEnvelope struct {
	Detail []struct {
		Loc []any  `json:"loc"`
		Msg string `json:"msg"`
	} `json:"detail"`
}

// ErrorFromResponse decodes a non-success HTTP response body into an
// APIError, understanding both the standard error envelope and FastAPI
// validation errors.
func ErrorFromResponse(statusCode int, body []byte) *APIError {
	apiErr := &APIError{StatusCode: statusCode}

	var env errorEnvelope
	if err := json.Unmarshal(body, &env); err == nil && env.Error.Message != "" {
		apiErr.Code = env.Error.Code
		apiErr.Message = env.Error.Message
		apiErr.Details = env.Error.Details
		return apiErr
	}

	var val validationEnvelope
	if err := json.Unmarshal(body, &val); err == nil && len(val.Detail) > 0 {
		msgs := make([]string, 0, len(val.Detail))
		for _, d := range val.Detail {
			loc := make([]string, 0, len(d.Loc))
			for _, l := range d.Loc {
				loc = append(loc, fmt.Sprint(l))
			}
			if len(loc) > 0 {
				msgs = append(msgs, fmt.Sprintf("%s: %s", strings.Join(loc, "."), d.Msg))
			} else {
				msgs = append(msgs, d.Msg)
			}
		}
		apiErr.Code = "VALIDATION_ERROR"
		apiErr.Message = strings.Join(msgs, "; ")
		return apiErr
	}

	return apiErr
}

// ExitCode maps an error to the CLI exit code: 1 for API errors, 2 for
// usage mistakes, 3 for connection or timeout failures.
func ExitCode(err error) int {
	if err == nil {
		return ExitOK
	}

	var usageErr *UsageError
	if errors.As(err, &usageErr) {
		return ExitUsage
	}
	if isCobraUsageError(err) {
		return ExitUsage
	}

	var apiErr *APIError
	if errors.As(err, &apiErr) {
		return ExitAPIError
	}

	if isConnectionError(err) {
		return ExitConnection
	}
	return ExitAPIError
}

// isCobraUsageError recognizes the usage errors cobra generates itself
// (unknown commands, missing required flags, wrong argument counts).
func isCobraUsageError(err error) bool {
	msg := err.Error()
	prefixes := []string{
		"unknown command",
		"unknown flag",
		"unknown shorthand flag",
		"required flag(s)",
		"flag needs an argument",
		"invalid argument",
		"accepts ",
		"requires at least",
	}
	for _, p := range prefixes {
		if strings.HasPrefix(msg, p) {
			return true
		}
	}
	return false
}

func isConnectionError(err error) bool {
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	var urlErr *url.Error
	if errors.As(err, &urlErr) {
		return true
	}
	var netErr net.Error
	return errors.As(err, &netErr)
}

// RenderError writes a styled error line to w (normally stderr).
func RenderError(w io.Writer, err error) {
	ui.Println(w, ui.Red.Render("Error: "+err.Error()))
}

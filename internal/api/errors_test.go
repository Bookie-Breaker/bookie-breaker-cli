package api

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

func TestMain(m *testing.M) {
	lipgloss.SetColorProfile(termenv.Ascii)
	os.Exit(m.Run())
}

func TestErrorFromResponseEnvelope(t *testing.T) {
	body := []byte(`{
		"error": {"code": "NOT_FOUND", "message": "game not found", "details": {"game_id": "abc"}},
		"meta": {"timestamp": "2026-07-04T00:00:00Z", "request_id": "r1"}
	}`)

	apiErr := ErrorFromResponse(404, body)
	if apiErr.Code != "NOT_FOUND" {
		t.Errorf("Code = %q", apiErr.Code)
	}
	if apiErr.Message != "game not found" {
		t.Errorf("Message = %q", apiErr.Message)
	}
	if apiErr.StatusCode != 404 {
		t.Errorf("StatusCode = %d", apiErr.StatusCode)
	}
	if apiErr.Details["game_id"] != "abc" {
		t.Errorf("Details = %v", apiErr.Details)
	}
	if got := apiErr.Error(); got != "NOT_FOUND: game not found" {
		t.Errorf("Error() = %q", got)
	}
}

func TestErrorFromResponseValidation(t *testing.T) {
	body := []byte(`{"detail": [{"loc": ["body", "stake"], "msg": "field required", "type": "missing"}]}`)

	apiErr := ErrorFromResponse(422, body)
	if apiErr.Code != "VALIDATION_ERROR" {
		t.Errorf("Code = %q", apiErr.Code)
	}
	if !strings.Contains(apiErr.Message, "body.stake: field required") {
		t.Errorf("Message = %q", apiErr.Message)
	}
}

func TestErrorFromResponseOpaqueBody(t *testing.T) {
	apiErr := ErrorFromResponse(500, []byte("boom"))
	if got := apiErr.Error(); got != "HTTP 500: Internal Server Error" {
		t.Errorf("Error() = %q", got)
	}
}

func TestUsageError(t *testing.T) {
	inner := errors.New("bad flag")
	usageErr := &UsageError{Err: inner}
	if got := usageErr.Error(); got != "bad flag" {
		t.Errorf("Error() = %q", got)
	}
	if !errors.Is(usageErr, inner) {
		t.Error("errors.Is(usageErr, inner) = false, want unwrap to inner")
	}
}

func TestExitCode(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want int
	}{
		{"nil", nil, ExitOK},
		{"deadline exceeded", fmt.Errorf("fetching edges: %w", context.DeadlineExceeded), ExitConnection},
		{"api error", &APIError{StatusCode: 404}, ExitAPIError},
		{"usage error", &UsageError{Err: errors.New("bad flag")}, ExitUsage},
		{"cobra unknown command", errors.New(`unknown command "bogus" for "bb"`), ExitUsage},
		{"cobra required flag", errors.New(`required flag(s) "game" not set`), ExitUsage},
		{"url error", &url.Error{Op: "Get", URL: "http://x", Err: errors.New("connection refused")}, ExitConnection},
		{"net error", &net.OpError{Op: "dial", Err: errors.New("refused")}, ExitConnection},
		{"generic", errors.New("something else"), ExitAPIError},
	}
	for _, tc := range cases {
		if got := ExitCode(tc.err); got != tc.want {
			t.Errorf("%s: ExitCode = %d, want %d", tc.name, got, tc.want)
		}
	}
}

func TestRenderError(t *testing.T) {
	var buf bytes.Buffer
	RenderError(&buf, errors.New("kaput"))
	if got := buf.String(); got != "Error: kaput\n" {
		t.Errorf("RenderError wrote %q", got)
	}
}

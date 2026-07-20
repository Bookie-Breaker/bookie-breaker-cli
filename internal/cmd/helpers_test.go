package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"os"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

func TestMain(m *testing.M) {
	// Force plain output so table assertions are byte-exact.
	lipgloss.SetColorProfile(termenv.Ascii)
	os.Exit(m.Run())
}

type cliResult struct {
	stdout string
	stderr string
	code   int
}

// runBB executes the root command with args in a hermetic environment.
// env keys override service URLs (AGENT_URL etc.).
func runBB(t *testing.T, env map[string]string, args ...string) cliResult {
	t.Helper()
	return runBBContext(t, context.Background(), env, args...)
}

// runBBContext is runBB with a caller-supplied context, for commands
// whose loops exit on context cancellation (bb live --watch).
func runBBContext(t *testing.T, ctx context.Context, env map[string]string, args ...string) cliResult {
	t.Helper()

	// Keep the developer's real config and env out of the test.
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	for _, key := range []string{
		"AGENT_URL", "LINES_SERVICE_URL", "STATISTICS_SERVICE_URL",
		"BOOKIE_EMULATOR_URL", "PREDICTION_ENGINE_URL",
	} {
		t.Setenv(key, "")
	}
	for k, v := range env {
		t.Setenv(k, v)
	}

	root := NewRootCmd()
	root.SetContext(ctx)
	var out, errOut bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&errOut)
	root.SetArgs(args)

	code := run(root, &errOut)
	return cliResult{stdout: out.String(), stderr: errOut.String(), code: code}
}

// writeEnvelope writes `{"data": data, "meta": {...}}` as the response.
func writeEnvelope(t *testing.T, w http.ResponseWriter, status int, data any) {
	t.Helper()
	writeJSON(t, w, status, map[string]any{
		"data": data,
		"meta": map[string]any{
			"timestamp":  "2026-07-04T12:00:00Z",
			"request_id": "11111111-1111-1111-1111-111111111111",
		},
	})
}

// writePagedEnvelope adds pagination metadata to the envelope.
func writePagedEnvelope(t *testing.T, w http.ResponseWriter, status int, data any) {
	t.Helper()
	writeJSON(t, w, status, map[string]any{
		"data": data,
		"meta": map[string]any{
			"timestamp":  "2026-07-04T12:00:00Z",
			"request_id": "11111111-1111-1111-1111-111111111111",
			"pagination": map[string]any{"limit": 50, "has_more": false},
		},
	})
}

// writeErrorEnvelope writes the standard `{"error": {...}}` failure body.
func writeErrorEnvelope(t *testing.T, w http.ResponseWriter, status int, code, message string) {
	t.Helper()
	writeJSON(t, w, status, map[string]any{
		"error": map[string]any{"code": code, "message": message},
		"meta": map[string]any{
			"timestamp":  "2026-07-04T12:00:00Z",
			"request_id": "11111111-1111-1111-1111-111111111111",
		},
	})
}

func writeJSON(t *testing.T, w http.ResponseWriter, status int, v any) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		t.Errorf("encoding fixture response: %v", err)
	}
}

// mustJSONEqual asserts that got (a JSON document) equals want when both
// are compared structurally.
func mustJSONEqual(t *testing.T, got string, want any) {
	t.Helper()

	wantRaw, err := json.Marshal(want)
	if err != nil {
		t.Fatalf("marshaling want: %v", err)
	}
	var wantAny, gotAny any
	if err := json.Unmarshal(wantRaw, &wantAny); err != nil {
		t.Fatalf("unmarshaling want: %v", err)
	}
	if err := json.Unmarshal([]byte(got), &gotAny); err != nil {
		t.Fatalf("unmarshaling got %q: %v", got, err)
	}

	gotNorm, _ := json.Marshal(gotAny)
	wantNorm, _ := json.Marshal(wantAny)
	if string(gotNorm) != string(wantNorm) {
		t.Errorf("JSON mismatch:\ngot:  %s\nwant: %s", gotNorm, wantNorm)
	}
}

// ptr returns a pointer to v, for building fixture structs.
func ptr[T any](v T) *T { return &v }

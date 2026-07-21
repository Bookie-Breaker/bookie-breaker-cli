package cmd

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Bookie-Breaker/bookie-breaker-cli/internal/api"
)

// healthServer answers all five health endpoints from one httptest server;
// paths present in unhealthy report a failing status.
func healthServer(t *testing.T, unhealthy map[string]bool) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/agent/health", "/api/v1/emulator/health", "/api/v1/predict/health":
			// Python services wrap health in the envelope.
			status := "healthy"
			if unhealthy[r.URL.Path] {
				status = "degraded"
			}
			writeEnvelope(t, w, http.StatusOK, map[string]any{"status": status, "version": "1.0.0"})
		case "/api/v1/lines/health", "/api/v1/stats/health":
			// Go services return the bare health document.
			code := http.StatusOK
			status := "healthy"
			if unhealthy[r.URL.Path] {
				code = http.StatusServiceUnavailable
				status = "unhealthy"
			}
			writeJSON(t, w, code, map[string]any{"status": status, "service": "x", "version": "1.0.0"})
		default:
			t.Errorf("unexpected health path %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
}

func allServiceEnv(url string) map[string]string {
	return map[string]string{
		"AGENT_URL":              url,
		"LINES_SERVICE_URL":      url,
		"STATISTICS_SERVICE_URL": url,
		"BOOKIE_EMULATOR_URL":    url,
		"PREDICTION_ENGINE_URL":  url,
	}
}

func TestHealthAllHealthy(t *testing.T) {
	srv := healthServer(t, nil)
	defer srv.Close()

	res := runBB(t, allServiceEnv(srv.URL), "health")
	if res.code != api.ExitOK {
		t.Fatalf("exit code = %d, stderr: %s", res.code, res.stderr)
	}

	for _, want := range []string{
		"SERVICE", "URL", "STATUS", "LATENCY",
		"agent", "lines-service", "statistics-service", "bookie-emulator", "prediction-engine",
		"healthy", "ms",
	} {
		if !strings.Contains(res.stdout, want) {
			t.Errorf("stdout missing %q:\n%s", want, res.stdout)
		}
	}
}

func TestHealthUnhealthyService(t *testing.T) {
	srv := healthServer(t, map[string]bool{"/api/v1/lines/health": true})
	defer srv.Close()

	res := runBB(t, allServiceEnv(srv.URL), "health")
	if res.code != api.ExitAPIError {
		t.Fatalf("exit code = %d, want %d", res.code, api.ExitAPIError)
	}
	if !strings.Contains(res.stdout, "unhealthy") {
		t.Errorf("stdout missing unhealthy status:\n%s", res.stdout)
	}
	if !strings.Contains(res.stderr, "unhealthy") {
		t.Errorf("stderr = %q", res.stderr)
	}
}

func TestHealthUnreachableService(t *testing.T) {
	srv := healthServer(t, nil)
	defer srv.Close()

	env := allServiceEnv(srv.URL)
	env["PREDICTION_ENGINE_URL"] = "http://127.0.0.1:1" // nothing listens here

	res := runBB(t, env, "health")
	if res.code != api.ExitAPIError {
		t.Fatalf("exit code = %d, want %d", res.code, api.ExitAPIError)
	}
	if !strings.Contains(res.stdout, "unreachable") {
		t.Errorf("stdout missing unreachable status:\n%s", res.stdout)
	}
}

func TestHealthJSON(t *testing.T) {
	srv := healthServer(t, nil)
	defer srv.Close()

	res := runBB(t, allServiceEnv(srv.URL), "health", "--format", "json")
	if res.code != api.ExitOK {
		t.Fatalf("exit code = %d, stderr: %s", res.code, res.stderr)
	}

	var results []map[string]any
	if err := json.Unmarshal([]byte(res.stdout), &results); err != nil {
		t.Fatalf("stdout not JSON: %v\n%s", err, res.stdout)
	}
	if len(results) != 5 {
		t.Fatalf("got %d results, want 5", len(results))
	}
	for _, r := range results {
		if r["healthy"] != true {
			t.Errorf("service %v not healthy: %v", r["service"], r)
		}
	}
}

func TestParseHealthStatus(t *testing.T) {
	cases := []struct {
		name string
		body string
		want string
	}{
		{"envelope status", `{"data": {"status": "healthy"}}`, "healthy"},
		{"top-level status", `{"status": "degraded"}`, "degraded"},
		{"no status fields", `{}`, ""},
		{"not JSON", `<html>oops</html>`, ""},
	}
	for _, tc := range cases {
		if got := parseHealthStatus(strings.NewReader(tc.body)); got != tc.want {
			t.Errorf("%s: parseHealthStatus = %q, want %q", tc.name, got, tc.want)
		}
	}
}

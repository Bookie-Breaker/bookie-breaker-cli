package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Bookie-Breaker/bookie-breaker-cli/internal/config"
)

func TestNewClients(t *testing.T) {
	cfg := config.Default()
	cfg.Timeout = 7 * time.Second
	cfg.AnalysisTimeout = 90 * time.Second

	clients, err := NewClients(cfg)
	if err != nil {
		t.Fatalf("NewClients: %v", err)
	}

	if clients.Agent == nil || clients.AgentAnalysis == nil || clients.Lines == nil ||
		clients.Emulator == nil || clients.Prediction == nil {
		t.Fatalf("NewClients left a client nil: %+v", clients)
	}
	if clients.HTTP.Timeout != cfg.Timeout {
		t.Errorf("HTTP.Timeout = %v, want %v", clients.HTTP.Timeout, cfg.Timeout)
	}
}

// TestNewClientsSetsUserAgent drives one request through the bundle to
// verify the request editor stamps the CLI User-Agent header.
func TestNewClientsSetsUserAgent(t *testing.T) {
	var gotUA string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotUA = r.Header.Get("User-Agent")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"data": {"status": "healthy"}, "meta": {"timestamp": "2026-07-04T12:00:00Z", "request_id": "r1"}}`))
	}))
	defer srv.Close()

	cfg := config.Default()
	cfg.AgentURL = srv.URL

	clients, err := NewClients(cfg)
	if err != nil {
		t.Fatalf("NewClients: %v", err)
	}
	if _, err := clients.Agent.GetHealthApiV1AgentHealthGetWithResponse(context.Background()); err != nil {
		t.Fatalf("health request: %v", err)
	}
	if gotUA != UserAgent {
		t.Errorf("User-Agent = %q, want %q", gotUA, UserAgent)
	}
}

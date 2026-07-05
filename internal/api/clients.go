// Package api bundles the generated OpenAPI clients behind one HTTP client
// and translates service failures into CLI-friendly errors and exit codes.
package api

import (
	"context"
	"fmt"
	"net/http"

	"github.com/Bookie-Breaker/bookie-breaker-cli/internal/client/agentservice"
	"github.com/Bookie-Breaker/bookie-breaker-cli/internal/client/bookieemulator"
	"github.com/Bookie-Breaker/bookie-breaker-cli/internal/client/linesservice"
	"github.com/Bookie-Breaker/bookie-breaker-cli/internal/client/predictionengine"
	"github.com/Bookie-Breaker/bookie-breaker-cli/internal/config"
)

// UserAgent identifies the CLI to backend services.
const UserAgent = "bookie-breaker-cli/bb"

// Clients bundles the generated *WithResponses clients over a single
// http.Client with the configured timeout. AgentAnalysis is the same agent
// client over a longer-timeout http.Client: LLM analysis calls (bb ask)
// routinely exceed the default request timeout.
type Clients struct {
	Agent         *agentservice.ClientWithResponses
	AgentAnalysis *agentservice.ClientWithResponses
	Lines         *linesservice.ClientWithResponses
	Emulator      *bookieemulator.ClientWithResponses
	Prediction    *predictionengine.ClientWithResponses
	HTTP          *http.Client
}

// NewClients builds the client bundle from the resolved configuration.
func NewClients(cfg *config.Config) (*Clients, error) {
	httpClient := &http.Client{Timeout: cfg.Timeout}

	userAgent := func(_ context.Context, req *http.Request) error {
		req.Header.Set("User-Agent", UserAgent)
		return nil
	}

	agent, err := agentservice.NewClientWithResponses(cfg.AgentURL,
		agentservice.WithHTTPClient(httpClient),
		agentservice.WithRequestEditorFn(userAgent))
	if err != nil {
		return nil, fmt.Errorf("building agent client: %w", err)
	}

	// A plain http.Client timeout caps requests regardless of context, so
	// slow LLM calls need their own client.
	analysisHTTP := &http.Client{Timeout: cfg.AnalysisTimeout}
	agentAnalysis, err := agentservice.NewClientWithResponses(cfg.AgentURL,
		agentservice.WithHTTPClient(analysisHTTP),
		agentservice.WithRequestEditorFn(userAgent))
	if err != nil {
		return nil, fmt.Errorf("building agent analysis client: %w", err)
	}

	lines, err := linesservice.NewClientWithResponses(cfg.LinesServiceURL,
		linesservice.WithHTTPClient(httpClient),
		linesservice.WithRequestEditorFn(userAgent))
	if err != nil {
		return nil, fmt.Errorf("building lines-service client: %w", err)
	}

	emulator, err := bookieemulator.NewClientWithResponses(cfg.BookieEmulatorURL,
		bookieemulator.WithHTTPClient(httpClient),
		bookieemulator.WithRequestEditorFn(userAgent))
	if err != nil {
		return nil, fmt.Errorf("building bookie-emulator client: %w", err)
	}

	prediction, err := predictionengine.NewClientWithResponses(cfg.PredictionEngineURL,
		predictionengine.WithHTTPClient(httpClient),
		predictionengine.WithRequestEditorFn(userAgent))
	if err != nil {
		return nil, fmt.Errorf("building prediction-engine client: %w", err)
	}

	return &Clients{
		Agent:         agent,
		AgentAnalysis: agentAnalysis,
		Lines:         lines,
		Emulator:      emulator,
		Prediction:    prediction,
		HTTP:          httpClient,
	}, nil
}

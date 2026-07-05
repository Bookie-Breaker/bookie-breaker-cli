package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/spf13/cobra"

	"github.com/Bookie-Breaker/bookie-breaker-cli/internal/ui"
)

// healthResult is one service's health probe outcome.
type healthResult struct {
	Service   string `json:"service"`
	URL       string `json:"url"`
	Status    string `json:"status"`
	LatencyMs int64  `json:"latency_ms"`
	Healthy   bool   `json:"healthy"`
}

// healthBody matches both the enveloped ({"data":{"status":...}}) and the
// bare ({"status":...}) health payload shapes.
type healthBody struct {
	Status string `json:"status"`
	Data   struct {
		Status string `json:"status"`
	} `json:"data"`
}

// NewHealthCmd probes every configured service's health endpoint
// concurrently. Exits 1 when any service is unreachable or unhealthy.
func NewHealthCmd(a *app) *cobra.Command {
	return &cobra.Command{
		Use:   "health",
		Short: "Check the health of all backend services",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			targets := []struct {
				service string
				base    string
				path    string
			}{
				{"agent", a.cfg.AgentURL, "/api/v1/agent/health"},
				{"lines-service", a.cfg.LinesServiceURL, "/api/v1/lines/health"},
				{"statistics-service", a.cfg.StatisticsServiceURL, "/api/v1/stats/health"},
				{"bookie-emulator", a.cfg.BookieEmulatorURL, "/api/v1/emulator/health"},
				{"prediction-engine", a.cfg.PredictionEngineURL, "/api/v1/predict/health"},
			}

			results := make([]healthResult, len(targets))
			var wg sync.WaitGroup
			for i, t := range targets {
				wg.Add(1)
				go func() {
					defer wg.Done()
					results[i] = probeHealth(a.clients.HTTP, t.service, t.base, t.path)
				}()
			}
			wg.Wait()

			if a.jsonOutput() {
				if err := ui.PrintJSON(cmd.OutOrStdout(), results); err != nil {
					return err
				}
			} else {
				rows := make([][]string, 0, len(results))
				for _, r := range results {
					status := ui.Red.Render(r.Status)
					if r.Healthy {
						status = ui.Green.Render(r.Status)
					}
					rows = append(rows, []string{
						r.Service,
						r.URL,
						status,
						fmt.Sprintf("%dms", r.LatencyMs),
					})
				}
				headers := []string{"SERVICE", "URL", "STATUS", "LATENCY"}
				ui.Println(cmd.OutOrStdout(), ui.Table(headers, rows))
			}

			for _, r := range results {
				if !r.Healthy {
					return errors.New("one or more services are unhealthy")
				}
			}
			return nil
		},
	}
}

// probeHealth issues a single GET and classifies the outcome.
func probeHealth(client *http.Client, service, base, path string) healthResult {
	result := healthResult{Service: service, URL: base + path}

	start := time.Now()
	resp, err := client.Get(base + path)
	result.LatencyMs = time.Since(start).Milliseconds()
	if err != nil {
		result.Status = "unreachable"
		return result
	}
	defer func() { _ = resp.Body.Close() }()

	result.Status = parseHealthStatus(resp.Body)
	if result.Status == "" {
		result.Status = "unhealthy"
		if resp.StatusCode == http.StatusOK {
			result.Status = "healthy"
		}
	}
	result.Healthy = resp.StatusCode == http.StatusOK && result.Status == "healthy"
	return result
}

func parseHealthStatus(body io.Reader) string {
	var hb healthBody
	if err := json.NewDecoder(body).Decode(&hb); err != nil {
		return ""
	}
	if hb.Data.Status != "" {
		return hb.Data.Status
	}
	return hb.Status
}

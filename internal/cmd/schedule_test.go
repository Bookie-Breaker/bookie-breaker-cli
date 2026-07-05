package cmd

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func scheduleData(league string) map[string]any {
	return map[string]any{
		"id":                 "77777777-6666-5555-4444-333333333333",
		"league":             league,
		"cron_expression":    "0 10,14,18 * * *",
		"timezone":           "America/New_York",
		"description":        "Thrice daily",
		"enabled":            true,
		"last_run_at":        nil,
		"next_run_at":        "2026-07-04T18:00:00Z",
		"simulation_config":  map[string]any{"iterations": 10000},
		"auto_bet":           true,
		"min_edge_threshold": 4.0,
	}
}

func TestScheduleList(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/api/v1/agent/schedule" {
			writeEnvelope(t, w, http.StatusOK, map[string]any{
				"schedules": []any{scheduleData("NBA"), scheduleData("MLB")},
			})
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	result := runBB(t, map[string]string{"AGENT_URL": server.URL}, "pipeline", "schedule", "list")
	if result.code != 0 {
		t.Fatalf("exit code = %d, stderr = %q", result.code, result.stderr)
	}
	for _, want := range []string{"NBA", "MLB", "0 10,14,18 * * *", "America/New_York", "4.0%", "2026-07-04T18:00:00Z"} {
		if !strings.Contains(result.stdout, want) {
			t.Errorf("stdout missing %q:\n%s", want, result.stdout)
		}
	}
}

func TestScheduleSetCreatesAndUpdates(t *testing.T) {
	var captured map[string]any
	var respond int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/api/v1/agent/schedule" {
			if err := json.NewDecoder(r.Body).Decode(&captured); err != nil {
				t.Fatalf("decoding schedule request: %v", err)
			}
			writeEnvelope(t, w, respond, scheduleData("NBA"))
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	respond = http.StatusCreated
	result := runBB(t, map[string]string{"AGENT_URL": server.URL},
		"pipeline", "schedule", "set", "--league", "NBA", "--cron", "0 10,14,18 * * *",
		"--timezone", "America/New_York", "--min-edge", "4.0")
	if result.code != 0 {
		t.Fatalf("exit code = %d, stderr = %q", result.code, result.stderr)
	}
	if !strings.Contains(result.stdout, "created") {
		t.Errorf("stdout missing 'created':\n%s", result.stdout)
	}
	if captured["league"] != "NBA" || captured["cron_expression"] != "0 10,14,18 * * *" {
		t.Errorf("request body = %v", captured)
	}
	if captured["timezone"] != "America/New_York" {
		t.Errorf("timezone = %v", captured["timezone"])
	}
	if captured["min_edge_threshold"] != 4.0 {
		t.Errorf("min_edge_threshold = %v", captured["min_edge_threshold"])
	}

	respond = http.StatusOK
	result = runBB(t, map[string]string{"AGENT_URL": server.URL},
		"pipeline", "schedule", "set", "--league", "NBA", "--cron", "0 9 * * *", "--enabled=false")
	if result.code != 0 {
		t.Fatalf("exit code = %d, stderr = %q", result.code, result.stderr)
	}
	if !strings.Contains(result.stdout, "updated") {
		t.Errorf("stdout missing 'updated':\n%s", result.stdout)
	}
	if captured["enabled"] != false {
		t.Errorf("enabled = %v, want false", captured["enabled"])
	}
}

func TestScheduleSetUsageErrors(t *testing.T) {
	// no league (and no default league), then no cron
	result := runBB(t, nil, "pipeline", "schedule", "set", "--cron", "0 9 * * *")
	if result.code != 2 {
		t.Errorf("missing league: exit code = %d, want 2 (stderr %q)", result.code, result.stderr)
	}
	result = runBB(t, nil, "pipeline", "schedule", "set", "--league", "NBA")
	if result.code != 2 {
		t.Errorf("missing cron: exit code = %d, want 2 (stderr %q)", result.code, result.stderr)
	}
}

func TestScheduleSetValidationErrorFromAgent(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeErrorEnvelope(t, w, http.StatusUnprocessableEntity, "UNPROCESSABLE_ENTITY", "Invalid cron expression")
	}))
	defer server.Close()

	result := runBB(t, map[string]string{"AGENT_URL": server.URL},
		"pipeline", "schedule", "set", "--league", "NBA", "--cron", "bogus")
	if result.code != 1 {
		t.Errorf("exit code = %d, want 1", result.code)
	}
	if !strings.Contains(result.stderr, "Invalid cron expression") {
		t.Errorf("stderr missing agent message: %q", result.stderr)
	}
}

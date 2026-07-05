package cmd

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/Bookie-Breaker/bookie-breaker-cli/internal/api"
	"github.com/Bookie-Breaker/bookie-breaker-cli/internal/client/agentservice"
)

const testRunID = "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"

func TestPipelineRun(t *testing.T) {
	accepted := agentservice.PipelineRunAcceptedData{
		PipelineRunId: testRunID,
		Status:        "running",
		League:        ptr("NFL"),
		GamesQueued:   3,
		StartedAt:     "2026-07-04T12:00:00Z",
		Steps: map[string]string{
			"fetch_stats": "pending",
			"simulate":    "pending",
		},
	}

	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1/agent/pipeline/run" {
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		raw, _ := io.ReadAll(r.Body)
		if err := json.Unmarshal(raw, &gotBody); err != nil {
			t.Errorf("request body not JSON: %v", err)
		}
		writeEnvelope(t, w, http.StatusAccepted, accepted)
	}))
	defer srv.Close()

	res := runBB(t, map[string]string{"AGENT_URL": srv.URL},
		"pipeline", "run", "--league", "NFL",
		"--games", "22222222-2222-2222-2222-222222222222,33333333-3333-3333-3333-333333333333",
		"--force-refresh")
	if res.code != api.ExitOK {
		t.Fatalf("exit code = %d, stderr: %s", res.code, res.stderr)
	}

	if gotBody["league"] != "NFL" {
		t.Errorf("body league = %v", gotBody["league"])
	}
	if gotBody["auto_bet"] != true {
		t.Errorf("body auto_bet = %v, want default true", gotBody["auto_bet"])
	}
	if gotBody["force_refresh"] != true {
		t.Errorf("body force_refresh = %v", gotBody["force_refresh"])
	}
	wantGames := []any{
		"22222222-2222-2222-2222-222222222222",
		"33333333-3333-3333-3333-333333333333",
	}
	if !reflect.DeepEqual(gotBody["game_ids"], wantGames) {
		t.Errorf("body game_ids = %v", gotBody["game_ids"])
	}

	for _, want := range []string{
		testRunID, "running", "NFL", "3",
		"STEP", "STATE", "fetch_stats", "simulate", "pending",
	} {
		if !strings.Contains(res.stdout, want) {
			t.Errorf("stdout missing %q:\n%s", want, res.stdout)
		}
	}
}

func TestPipelineRunJSON(t *testing.T) {
	accepted := agentservice.PipelineRunAcceptedData{
		PipelineRunId: testRunID,
		Status:        "running",
		League:        nil,
		GamesQueued:   0,
		StartedAt:     "2026-07-04T12:00:00Z",
		Steps:         map[string]string{},
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeEnvelope(t, w, http.StatusAccepted, accepted)
	}))
	defer srv.Close()

	res := runBB(t, map[string]string{"AGENT_URL": srv.URL}, "pipeline", "run", "--format", "json")
	if res.code != api.ExitOK {
		t.Fatalf("exit code = %d, stderr: %s", res.code, res.stderr)
	}
	mustJSONEqual(t, res.stdout, accepted)
}

func TestPipelineStatus(t *testing.T) {
	runData := agentservice.PipelineRunData{
		PipelineRunId:  testRunID,
		Status:         "completed",
		Trigger:        "manual",
		League:         ptr("NFL"),
		Params:         map[string]any{"auto_bet": true},
		GamesProcessed: 3,
		EdgesFound:     2,
		BetsPlaced:     1,
		StartedAt:      "2026-07-04T12:00:00Z",
		FinishedAt:     ptr("2026-07-04T12:05:00Z"),
		Steps: map[string]any{
			"fetch_stats": "completed",
			"simulate":    map[string]any{"status": "completed", "duration_ms": 5100},
			"detect":      "failed",
		},
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		wantPath := "/api/v1/agent/pipeline/runs/" + testRunID
		if r.URL.Path != wantPath {
			t.Errorf("path = %s, want %s", r.URL.Path, wantPath)
		}
		writeEnvelope(t, w, http.StatusOK, runData)
	}))
	defer srv.Close()

	res := runBB(t, map[string]string{"AGENT_URL": srv.URL}, "pipeline", "status", testRunID)
	if res.code != api.ExitOK {
		t.Fatalf("exit code = %d, stderr: %s", res.code, res.stderr)
	}

	for _, want := range []string{
		testRunID, "completed", "manual", "NFL",
		"Games processed: 3", "Edges found: 2", "Bets placed: 1",
		"STEP", "STATE", "fetch_stats", "simulate", "detect", "failed",
	} {
		if !strings.Contains(res.stdout, want) {
			t.Errorf("stdout missing %q:\n%s", want, res.stdout)
		}
	}
}

func TestPipelineStatusInvalidRunID(t *testing.T) {
	res := runBB(t, nil, "pipeline", "status", "nope")
	if res.code != api.ExitUsage {
		t.Fatalf("exit code = %d, want %d", res.code, api.ExitUsage)
	}
}

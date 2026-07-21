package cmd

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Bookie-Breaker/bookie-breaker-cli/internal/api"
	"github.com/Bookie-Breaker/bookie-breaker-cli/internal/client/agentservice"
)

const (
	askGameID  = "11111111-2222-3333-4444-555555555555"
	testEdgeID = "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"
)

func analysisData(analysisType string) map[string]any {
	return map[string]any{
		"id":            "99999999-8888-7777-6666-555555555555",
		"analysis_type": analysisType,
		"game_id":       nil,
		"edge_id":       nil,
		"title":         "Edge Analysis: Over 220.5 in LAL vs BOS",
		"content":       "## Summary\n\nThe model likes the over.",
		"model_used":    "claude-opus-4-8",
		"input_summary": "Edge, Game, Market lines",
		"created_at":    "2026-07-04T12:00:00Z",
	}
}

// askServer records the analysis request body and replies 201.
func askServer(t *testing.T, captured *map[string]any) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/api/v1/agent/analysis" {
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatalf("decoding analysis request: %v", err)
			}
			*captured = body
			analysisType, _ := body["analysis_type"].(string)
			writeEnvelope(t, w, http.StatusCreated, analysisData(analysisType))
			return
		}
		http.NotFound(w, r)
	}))
}

func TestAskInfersAnalysisType(t *testing.T) {
	cases := []struct {
		name     string
		args     []string
		wantType string
	}{
		{"edge scope wins", []string{"ask", "why?", "--edge", testEdgeID, "--game", askGameID}, "EDGE_BREAKDOWN"},
		{"game scope", []string{"ask", "preview?", "--game", askGameID}, "GAME_PREVIEW"},
		{"unscoped", []string{"ask", "how", "are", "we", "doing?"}, "PERFORMANCE_REVIEW"},
		{"explicit type", []string{"ask", "why?", "--game", askGameID, "--type", "edge_breakdown"}, "EDGE_BREAKDOWN"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var captured map[string]any
			server := askServer(t, &captured)
			defer server.Close()

			result := runBB(t, map[string]string{"AGENT_URL": server.URL}, tc.args...)
			if result.code != 0 {
				t.Fatalf("exit code = %d, stderr = %q", result.code, result.stderr)
			}
			if got := captured["analysis_type"]; got != tc.wantType {
				t.Errorf("analysis_type = %v, want %s", got, tc.wantType)
			}
		})
	}
}

func TestAskJoinsQuestionWords(t *testing.T) {
	var captured map[string]any
	server := askServer(t, &captured)
	defer server.Close()

	result := runBB(t, map[string]string{"AGENT_URL": server.URL}, "ask", "why", "the", "over", "tonight?")
	if result.code != 0 {
		t.Fatalf("exit code = %d, stderr = %q", result.code, result.stderr)
	}
	if got := captured["question"]; got != "why the over tonight?" {
		t.Errorf("question = %v", got)
	}
}

func TestAskRendersAnalysis(t *testing.T) {
	var captured map[string]any
	server := askServer(t, &captured)
	defer server.Close()

	result := runBB(t, map[string]string{"AGENT_URL": server.URL}, "ask", "why?")
	if result.code != 0 {
		t.Fatalf("exit code = %d, stderr = %q", result.code, result.stderr)
	}
	for _, want := range []string{"Edge Analysis: Over 220.5", "the model likes the over", "claude-opus-4-8"} {
		if !strings.Contains(strings.ToLower(result.stdout), strings.ToLower(want)) {
			t.Errorf("stdout missing %q:\n%s", want, result.stdout)
		}
	}
}

func TestAskCachedAnalysisIsAccepted(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 200 = reused cached analysis
		writeEnvelope(t, w, http.StatusOK, analysisData("EDGE_BREAKDOWN"))
	}))
	defer server.Close()

	result := runBB(t, map[string]string{"AGENT_URL": server.URL}, "ask", "why?", "--edge", testEdgeID)
	if result.code != 0 {
		t.Fatalf("exit code = %d, stderr = %q", result.code, result.stderr)
	}
	if !strings.Contains(result.stdout, "Edge Analysis") {
		t.Errorf("stdout missing analysis title:\n%s", result.stdout)
	}
}

func TestAskJSONOutput(t *testing.T) {
	var captured map[string]any
	server := askServer(t, &captured)
	defer server.Close()

	result := runBB(t, map[string]string{"AGENT_URL": server.URL}, "ask", "why?", "--format", "json")
	if result.code != 0 {
		t.Fatalf("exit code = %d, stderr = %q", result.code, result.stderr)
	}
	var data map[string]any
	if err := json.Unmarshal([]byte(result.stdout), &data); err != nil {
		t.Fatalf("stdout is not JSON: %v\n%s", err, result.stdout)
	}
	if data["model_used"] != "claude-opus-4-8" {
		t.Errorf("model_used = %v", data["model_used"])
	}
}

func TestAskUsageErrors(t *testing.T) {
	cases := []struct {
		name string
		args []string
	}{
		{"no question", []string{"ask"}},
		{"bad game uuid", []string{"ask", "why?", "--game", "not-a-uuid"}},
		{"bad edge uuid", []string{"ask", "why?", "--edge", "not-a-uuid"}},
		{"bad type", []string{"ask", "why?", "--type", "HOT_TAKE"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			result := runBB(t, nil, tc.args...)
			if result.code != 2 {
				t.Errorf("exit code = %d, want 2 (stderr %q)", result.code, result.stderr)
			}
		})
	}
}

func TestAskAPIErrorExitCode(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeErrorEnvelope(t, w, http.StatusBadGateway, "DEPENDENCY_ERROR", "LLM analysis failed")
	}))
	defer server.Close()

	result := runBB(t, map[string]string{"AGENT_URL": server.URL}, "ask", "why?")
	if result.code != 1 {
		t.Errorf("exit code = %d, want 1", result.code)
	}
	if !strings.Contains(result.stderr, "LLM analysis failed") {
		t.Errorf("stderr missing upstream message: %q", result.stderr)
	}
}

func TestResolveAnalysisType(t *testing.T) {
	cases := []struct {
		name     string
		override string
		gameID   string
		edgeID   string
		want     agentservice.AnalysisRequestAnalysisType
	}{
		{"explicit preview", "game_preview", "", "", agentservice.GAMEPREVIEW},
		{"explicit breakdown", "EDGE_BREAKDOWN", "", "", agentservice.EDGEBREAKDOWN},
		{"explicit review", "performance_review", "", "", agentservice.PERFORMANCEREVIEW},
		{"edge scope wins", "", askGameID, testEdgeID, agentservice.EDGEBREAKDOWN},
		{"game scope", "", askGameID, "", agentservice.GAMEPREVIEW},
		{"no scope", "", "", "", agentservice.PERFORMANCEREVIEW},
	}
	for _, tc := range cases {
		got, err := resolveAnalysisType(tc.override, tc.gameID, tc.edgeID)
		if err != nil {
			t.Errorf("%s: resolveAnalysisType: %v", tc.name, err)
			continue
		}
		if got != tc.want {
			t.Errorf("%s: resolveAnalysisType = %q, want %q", tc.name, got, tc.want)
		}
	}
}

func TestResolveAnalysisTypeInvalid(t *testing.T) {
	_, err := resolveAnalysisType("VIBES", "", "")
	if err == nil {
		t.Fatal("resolveAnalysisType(VIBES) = nil error, want usage error")
	}
	var usageErr *api.UsageError
	if !errors.As(err, &usageErr) {
		t.Errorf("error type = %T, want *api.UsageError", err)
	}
	if !strings.Contains(err.Error(), "invalid --type") {
		t.Errorf("error = %q, want invalid --type message", err)
	}
}

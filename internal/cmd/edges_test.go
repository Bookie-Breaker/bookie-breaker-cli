package cmd

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Bookie-Breaker/bookie-breaker-cli/internal/api"
	"github.com/Bookie-Breaker/bookie-breaker-cli/internal/client/agentservice"
)

func edgesFixture() []agentservice.EdgeListItem {
	return []agentservice.EdgeListItem{
		{
			Id:                   "edge-small",
			GameId:               "22222222-2222-2222-2222-222222222222",
			League:               "NFL",
			ScheduledStart:       "2026-07-05T00:20:00Z",
			MarketType:           "TOTAL",
			Selection:            "OVER 47.5",
			PredictedProbability: 0.55,
			ImpliedProbability:   0.524,
			EdgePercentage:       2.6,
			ExpectedValue:        0.05,
			OddsAmerican:         -110,
			SportsbookKey:        "fanduel",
			KellyFraction:        0.02,
			RecommendedStake:     0.5,
			DetectedAt:           "2026-07-04T15:00:00Z",
			ExpiresAt:            "2026-07-05T00:20:00Z",
			HomeTeam:             ptr("PHI"),
			AwayTeam:             ptr("DAL"),
		},
		{
			Id:                   "edge-big",
			GameId:               "33333333-3333-3333-3333-333333333333",
			League:               "NFL",
			ScheduledStart:       "2026-07-05T00:20:00Z",
			MarketType:           "SPREAD",
			Selection:            "DEN -3.5",
			PredictedProbability: 0.58,
			ImpliedProbability:   0.51,
			EdgePercentage:       7.0,
			ExpectedValue:        0.12,
			OddsAmerican:         102,
			SportsbookKey:        "draftkings",
			KellyFraction:        0.05,
			RecommendedStake:     1.5,
			DetectedAt:           "2026-07-04T16:00:00Z",
			ExpiresAt:            "2026-07-05T00:20:00Z",
			HomeTeam:             ptr("DEN"),
			AwayTeam:             ptr("KC"),
		},
	}
}

func TestEdgesTable(t *testing.T) {
	var gotQuery map[string][]string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/agent/edges" {
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		gotQuery = r.URL.Query()
		writePagedEnvelope(t, w, http.StatusOK, edgesFixture())
	}))
	defer srv.Close()

	res := runBB(t, map[string]string{"AGENT_URL": srv.URL},
		"edges", "--league", "NFL", "--market", "SPREAD", "--min-edge", "2", "--limit", "10")
	if res.code != api.ExitOK {
		t.Fatalf("exit code = %d, stderr: %s", res.code, res.stderr)
	}

	if got := gotQuery["league"]; len(got) != 1 || got[0] != "NFL" {
		t.Errorf("league query = %v", got)
	}
	if got := gotQuery["market_type"]; len(got) != 1 || got[0] != "SPREAD" {
		t.Errorf("market_type query = %v", got)
	}
	if got := gotQuery["min_edge"]; len(got) != 1 || got[0] != "2" {
		t.Errorf("min_edge query = %v", got)
	}
	if got := gotQuery["limit"]; len(got) != 1 || got[0] != "10" {
		t.Errorf("limit query = %v", got)
	}

	for _, want := range []string{
		"GAME", "MARKET", "SELECTION", "ODDS", "MODEL %", "IMPLIED %", "EDGE", "BOOK", "DETECTED",
		"KC @ DEN", "DAL @ PHI", "DEN -3.5", "OVER 47.5",
		"+102", "-110", "58.0%", "51.0%", "+7.0%", "+2.6%", "draftkings", "fanduel",
	} {
		if !strings.Contains(res.stdout, want) {
			t.Errorf("stdout missing %q:\n%s", want, res.stdout)
		}
	}

	// Sorted by edge descending: the +7.0% row comes first.
	if strings.Index(res.stdout, "+7.0%") > strings.Index(res.stdout, "+2.6%") {
		t.Errorf("edges not sorted by edge desc:\n%s", res.stdout)
	}
}

func TestEdgesJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writePagedEnvelope(t, w, http.StatusOK, edgesFixture())
	}))
	defer srv.Close()

	res := runBB(t, map[string]string{"AGENT_URL": srv.URL}, "edges", "--format", "json")
	if res.code != api.ExitOK {
		t.Fatalf("exit code = %d, stderr: %s", res.code, res.stderr)
	}

	// JSON output is the unwrapped data array, sorted by edge descending.
	want := edgesFixture()
	want[0], want[1] = want[1], want[0]
	mustJSONEqual(t, res.stdout, want)
}

func TestEdgesAPIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeErrorEnvelope(t, w, http.StatusInternalServerError, "INTERNAL", "edge store unavailable")
	}))
	defer srv.Close()

	res := runBB(t, map[string]string{"AGENT_URL": srv.URL}, "edges")
	if res.code != api.ExitAPIError {
		t.Fatalf("exit code = %d, want %d", res.code, api.ExitAPIError)
	}
	if !strings.Contains(res.stderr, "INTERNAL: edge store unavailable") {
		t.Errorf("stderr = %q", res.stderr)
	}
	if res.stdout != "" {
		t.Errorf("stdout should be empty on error, got %q", res.stdout)
	}
}

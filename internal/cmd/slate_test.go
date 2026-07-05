package cmd

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Bookie-Breaker/bookie-breaker-cli/internal/api"
	"github.com/Bookie-Breaker/bookie-breaker-cli/internal/client/agentservice"
)

func slateFixture() agentservice.SlateData {
	return agentservice.SlateData{
		Date: "2026-07-04",
		Games: []agentservice.SlateGame{
			{
				GameId:         "22222222-2222-2222-2222-222222222222",
				League:         "NFL",
				HomeTeam:       agentservice.SlateTeam{Id: "t1", Name: "Eagles", Abbreviation: "PHI"},
				AwayTeam:       agentservice.SlateTeam{Id: "t2", Name: "Cowboys", Abbreviation: "DAL"},
				ScheduledStart: "2026-07-04T23:00:00Z",
				Status:         "SCHEDULED",
				Prediction: &agentservice.SlatePrediction{
					Id:                   "p1",
					MarketType:           "SPREAD",
					Selection:            "PHI -2.5",
					PredictedProbability: 0.56,
					PredictedAt:          "2026-07-04T12:00:00Z",
				},
				Edges: []agentservice.SlateEdge{
					{Id: "e1", MarketType: "SPREAD", Selection: "PHI -2.5", EdgePercentage: 3.1, SportsbookKey: "draftkings"},
					{Id: "e2", MarketType: "TOTAL", Selection: "UNDER 44.5", EdgePercentage: 5.4, SportsbookKey: "fanduel"},
				},
			},
			{
				GameId:         "33333333-3333-3333-3333-333333333333",
				League:         "NFL",
				HomeTeam:       agentservice.SlateTeam{Id: "t3", Name: "Broncos", Abbreviation: "DEN"},
				AwayTeam:       agentservice.SlateTeam{Id: "t4", Name: "Chiefs", Abbreviation: "KC"},
				ScheduledStart: "2026-07-05T02:15:00Z",
				Status:         "SCHEDULED",
				Prediction:     nil,
				Edges:          []agentservice.SlateEdge{},
			},
		},
	}
}

func TestSlateTable(t *testing.T) {
	var gotQuery map[string][]string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/agent/slate" {
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		gotQuery = r.URL.Query()
		writeEnvelope(t, w, http.StatusOK, slateFixture())
	}))
	defer srv.Close()

	res := runBB(t, map[string]string{"AGENT_URL": srv.URL},
		"slate", "--league", "NFL", "--date", "2026-07-04")
	if res.code != api.ExitOK {
		t.Fatalf("exit code = %d, stderr: %s", res.code, res.stderr)
	}
	if got := gotQuery["date"]; len(got) != 1 || got[0] != "2026-07-04" {
		t.Errorf("date query = %v", got)
	}

	for _, want := range []string{
		"TIME", "MATCHUP", "STATUS", "PREDICTION", "BEST EDGE",
		"DAL @ PHI", "KC @ DEN", "SCHEDULED",
		"PHI -2.5 56.0%", "+5.4% TOTAL", "—",
	} {
		if !strings.Contains(res.stdout, want) {
			t.Errorf("stdout missing %q:\n%s", want, res.stdout)
		}
	}
}

func TestSlateJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeEnvelope(t, w, http.StatusOK, slateFixture())
	}))
	defer srv.Close()

	res := runBB(t, map[string]string{"AGENT_URL": srv.URL}, "slate", "--format", "json")
	if res.code != api.ExitOK {
		t.Fatalf("exit code = %d, stderr: %s", res.code, res.stderr)
	}
	mustJSONEqual(t, res.stdout, slateFixture())
}

func TestSlateBadDate(t *testing.T) {
	res := runBB(t, nil, "slate", "--date", "tomorrow")
	if res.code != api.ExitUsage {
		t.Fatalf("exit code = %d, want %d", res.code, api.ExitUsage)
	}
	if !strings.Contains(res.stderr, "invalid --date") {
		t.Errorf("stderr = %q", res.stderr)
	}
}

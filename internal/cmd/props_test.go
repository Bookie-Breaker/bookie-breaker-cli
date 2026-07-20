package cmd

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/Bookie-Breaker/bookie-breaker-cli/internal/api"
	"github.com/Bookie-Breaker/bookie-breaker-cli/internal/client/agentservice"
	"github.com/Bookie-Breaker/bookie-breaker-cli/internal/client/linesservice"
)

// propEdgesFixture mixes two player prop edges (one OVER/UNDER, one
// YES/NO) with a game-market edge that a lax backend could return
// despite the market_type filter.
func propEdgesFixture() []agentservice.EdgeListItem {
	base := agentservice.EdgeListItem{
		GameId:         "33333333-3333-3333-3333-333333333333",
		League:         "EPL",
		ScheduledStart: "2026-07-19T19:00:00Z",
		DetectedAt:     "2026-07-19T17:00:00Z",
		ExpiresAt:      "2026-07-19T19:00:00Z",
		HomeTeam:       ptr("ARS"),
		AwayTeam:       ptr("CHE"),
	}

	shots := base
	shots.Id = "edge-shots"
	shots.MarketType = "PLAYER_PROP"
	shots.Selection = "OVER 2.5"
	shots.PlayerExternalId = ptr("erling-haaland")
	shots.StatType = ptr("player_shots_on_target")
	shots.PropType = ptr("OVER_UNDER")
	shots.PredictedProbability = 0.58
	shots.ImpliedProbability = 0.524
	shots.EdgePercentage = 5.6
	shots.ExpectedValue = 0.10
	shots.OddsAmerican = -110
	shots.SportsbookKey = "draftkings"

	scorer := base
	scorer.Id = "edge-scorer"
	scorer.MarketType = "PLAYER_PROP"
	scorer.Selection = "YES"
	scorer.PlayerExternalId = ptr("bukayo-saka")
	scorer.StatType = ptr("player_goal_scorer_anytime")
	scorer.PropType = ptr("YES_NO")
	scorer.PredictedProbability = 0.43
	scorer.ImpliedProbability = 0.40
	scorer.EdgePercentage = 3.0
	scorer.ExpectedValue = 0.07
	scorer.OddsAmerican = 150
	scorer.SportsbookKey = "fanduel"

	spread := base
	spread.Id = "edge-spread"
	spread.MarketType = "SPREAD"
	spread.Selection = "ARS -0.5"
	spread.EdgePercentage = 9.9
	spread.OddsAmerican = -105
	spread.SportsbookKey = "pinnacle"

	return []agentservice.EdgeListItem{scorer, shots, spread}
}

// propsAgentStub serves the agent edges endpoint and records the query.
func propsAgentStub(t *testing.T, edges any) (env map[string]string, query *map[string][]string) {
	t.Helper()
	query = &map[string][]string{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/agent/edges" {
			t.Errorf("agent path = %s, want /api/v1/agent/edges", r.URL.Path)
		}
		*query = r.URL.Query()
		writePagedEnvelope(t, w, http.StatusOK, edges)
	}))
	t.Cleanup(srv.Close)
	return map[string]string{"AGENT_URL": srv.URL}, query
}

// propsLinesStub serves the lines-service current-lines endpoint.
func propsLinesStub(t *testing.T, lines any) (env map[string]string, query *map[string][]string) {
	t.Helper()
	query = &map[string][]string{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/lines/current" {
			t.Errorf("lines path = %s, want /api/v1/lines/current", r.URL.Path)
		}
		*query = r.URL.Query()
		writeEnvelope(t, w, http.StatusOK, lines)
	}))
	t.Cleanup(srv.Close)
	return map[string]string{"LINES_SERVICE_URL": srv.URL}, query
}

func TestPropsTable(t *testing.T) {
	env, query := propsAgentStub(t, propEdgesFixture())

	res := runBB(t, env, "props", "--league", "EPL")
	if res.code != api.ExitOK {
		t.Fatalf("exit code = %d, stderr: %s", res.code, res.stderr)
	}

	if got := (*query)["market_type"]; len(got) != 1 || got[0] != "PLAYER_PROP" {
		t.Errorf("market_type query = %v, want [PLAYER_PROP]", got)
	}
	if got := (*query)["league"]; len(got) != 1 || got[0] != "EPL" {
		t.Errorf("league query = %v, want [EPL]", got)
	}

	for _, want := range []string{
		"PLAYER", "STAT", "SIDE", "LINE", "ODDS", "EV%", "BOOK",
		"Erling Haaland", "Shots on target", "OVER", "2.5", "-110", "+5.6%", "draftkings",
		"Bukayo Saka", "Anytime goalscorer", "YES", "+150", "+3.0%", "fanduel",
	} {
		if !strings.Contains(res.stdout, want) {
			t.Errorf("stdout missing %q:\n%s", want, res.stdout)
		}
	}

	// The non-prop edge is dropped client-side even though the backend
	// returned it.
	if strings.Contains(res.stdout, "ARS -0.5") || strings.Contains(res.stdout, "pinnacle") {
		t.Errorf("stdout contains non-prop edge:\n%s", res.stdout)
	}

	// Sorted by edge, highest first: the shots edge (5.6) precedes the
	// scorer edge (3.0) even though the fixture lists it second.
	if i, j := strings.Index(res.stdout, "Erling Haaland"), strings.Index(res.stdout, "Bukayo Saka"); i > j {
		t.Errorf("edges not sorted by edge percentage:\n%s", res.stdout)
	}
}

func TestPropsEmptyState(t *testing.T) {
	env, _ := propsAgentStub(t, []agentservice.EdgeListItem{})

	res := runBB(t, env, "props")
	if res.code != api.ExitOK {
		t.Fatalf("exit code = %d, stderr: %s", res.code, res.stderr)
	}
	if !strings.Contains(res.stdout, "No player prop edges right now.") {
		t.Errorf("stdout missing empty-state message:\n%s", res.stdout)
	}
}

func TestPropsJSON(t *testing.T) {
	env, _ := propsAgentStub(t, propEdgesFixture())

	res := runBB(t, env, "props", "--format", "json")
	if res.code != api.ExitOK {
		t.Fatalf("exit code = %d, stderr: %s", res.code, res.stderr)
	}

	var got []map[string]any
	if err := json.Unmarshal([]byte(res.stdout), &got); err != nil {
		t.Fatalf("stdout not JSON: %v\n%s", err, res.stdout)
	}
	if len(got) != 2 {
		t.Fatalf("edges count = %d, want 2 (non-prop edge filtered)", len(got))
	}
	if got[0]["id"] != "edge-shots" || got[1]["id"] != "edge-scorer" {
		t.Errorf("edge order = [%v %v], want [edge-shots edge-scorer]", got[0]["id"], got[1]["id"])
	}
	if got[0]["player_external_id"] != "erling-haaland" || got[0]["stat_type"] != "player_shots_on_target" {
		t.Errorf("prop fields not preserved in JSON: %v", got[0])
	}
}

// propLinesFixture returns snapshots deliberately out of order across
// two games and two players so grouping is observable.
func propLinesFixture() []linesservice.LineSnapshot {
	ts := time.Date(2026, 7, 19, 18, 30, 0, 0, time.UTC)
	snap := func(id, gameID, player, stat, selection string, side linesservice.LineSnapshotSide, line *float32, odds int) linesservice.LineSnapshot {
		return linesservice.LineSnapshot{
			Id:                 uuid.MustParse(id),
			GameId:             gameID,
			SportsbookId:       uuid.MustParse("55555555-5555-5555-5555-555555555555"),
			SportsbookKey:      "draftkings",
			MarketType:         "PLAYER_PROP",
			Selection:          selection,
			Side:               ptr(side),
			LineValue:          line,
			OddsAmerican:       odds,
			OddsDecimal:        1.9,
			ImpliedProbability: 0.5,
			PlayerId:           ptr(player),
			StatType:           ptr(stat),
			PropType:           ptr(linesservice.LineSnapshotPropType("OVER_UNDER")),
			Timestamp:          ts,
		}
	}

	saka := snap("aaaaaaaa-1111-1111-1111-111111111111",
		"99999999-9999-9999-9999-999999999999", "bukayo-saka", "player_goal_scorer_anytime",
		"YES", "YES", nil, 150)
	saka.PropType = ptr(linesservice.LineSnapshotPropType("YES_NO"))

	return []linesservice.LineSnapshot{
		// Game 9999 first in the payload; sorts after game 1111.
		saka,
		snap("aaaaaaaa-2222-2222-2222-222222222222",
			"11111111-1111-1111-1111-111111111111", "mohamed-salah", "player_shots",
			"OVER 3.5", "OVER", ptr(float32(3.5)), -105),
		snap("aaaaaaaa-3333-3333-3333-333333333333",
			"11111111-1111-1111-1111-111111111111", "erling-haaland", "player_shots_on_target",
			"UNDER 1.5", "UNDER", ptr(float32(1.5)), 120),
	}
}

func TestPropsLinesTable(t *testing.T) {
	env, query := propsLinesStub(t, propLinesFixture())

	res := runBB(t, env, "props", "--lines", "--league", "EPL")
	if res.code != api.ExitOK {
		t.Fatalf("exit code = %d, stderr: %s", res.code, res.stderr)
	}

	if got := (*query)["market_type"]; len(got) != 1 || got[0] != "PLAYER_PROP" {
		t.Errorf("market_type query = %v, want [PLAYER_PROP]", got)
	}
	if got := (*query)["league"]; len(got) != 1 || got[0] != "EPL" {
		t.Errorf("league query = %v, want [EPL]", got)
	}

	for _, want := range []string{
		"GAME", "PLAYER", "STAT", "SIDE", "LINE", "ODDS", "BOOK", "UPDATED",
		"11111111", "Mohamed Salah", "Shots", "OVER", "3.5", "-105",
		"Erling Haaland", "Shots on target", "UNDER", "1.5", "+120",
		"99999999", "Bukayo Saka", "Anytime goalscorer", "YES", "+150",
	} {
		if !strings.Contains(res.stdout, want) {
			t.Errorf("stdout missing %q:\n%s", want, res.stdout)
		}
	}

	// Grouped by game then player: both game-1111 rows precede the
	// game-9999 row, and within game 1111 erling-haaland sorts before
	// mohamed-salah.
	haaland := strings.Index(res.stdout, "Erling Haaland")
	salah := strings.Index(res.stdout, "Mohamed Salah")
	saka := strings.Index(res.stdout, "Bukayo Saka")
	if haaland >= salah || salah >= saka {
		t.Errorf("lines not grouped by game then player (haaland=%d salah=%d saka=%d):\n%s",
			haaland, salah, saka, res.stdout)
	}
}

func TestPropsLinesEmptyState(t *testing.T) {
	env, _ := propsLinesStub(t, []linesservice.LineSnapshot{})

	res := runBB(t, env, "props", "--lines")
	if res.code != api.ExitOK {
		t.Fatalf("exit code = %d, stderr: %s", res.code, res.stderr)
	}
	if !strings.Contains(res.stdout, "No player prop lines right now.") {
		t.Errorf("stdout missing empty-state message:\n%s", res.stdout)
	}
}

func TestStatLabelFallbackAndSelectionParsing(t *testing.T) {
	if got := statLabel(ptr("player_first_td")); got != "Player First Td" {
		t.Errorf("statLabel fallback = %q, want %q", got, "Player First Td")
	}
	if got := statLabel(nil); got != "—" {
		t.Errorf("statLabel(nil) = %q, want dash", got)
	}
	if got := playerLabel(ptr("de-bruyne")); got != "De Bruyne" {
		t.Errorf("playerLabel = %q, want %q", got, "De Bruyne")
	}

	cases := []struct {
		selection, side, line string
	}{
		{"OVER 2.5", "OVER", "2.5"},
		{"Erling Haaland UNDER 1.5", "UNDER", "1.5"},
		{"YES", "YES", "—"},
		{"Bukayo Saka NO", "NO", "—"},
		{"ARS -0.5", "—", "—"},
	}
	for _, c := range cases {
		side, line := propSelectionSideLine(c.selection)
		if side != c.side || line != c.line {
			t.Errorf("propSelectionSideLine(%q) = (%q, %q), want (%q, %q)",
				c.selection, side, line, c.side, c.line)
		}
	}
}

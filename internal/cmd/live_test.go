package cmd

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/Bookie-Breaker/bookie-breaker-cli/internal/api"
	"github.com/Bookie-Breaker/bookie-breaker-cli/internal/client/linesservice"
)

func liveLinesFixture() []linesservice.LineSnapshot {
	ts := time.Date(2026, 7, 19, 20, 15, 0, 0, time.UTC)
	return []linesservice.LineSnapshot{
		{
			Id:                 uuid.MustParse("aaaaaaaa-1111-1111-1111-111111111111"),
			GameId:             "33333333-3333-3333-3333-333333333333",
			SportsbookId:       uuid.MustParse("55555555-5555-5555-5555-555555555555"),
			SportsbookKey:      "draftkings",
			MarketType:         "SPREAD",
			Selection:          "DEN -1.5",
			Side:               ptr(linesservice.LineSnapshotSide("HOME")),
			LineValue:          ptr(float32(-1.5)),
			OddsAmerican:       -115,
			OddsDecimal:        1.87,
			ImpliedProbability: 0.535,
			Timestamp:          ts,
			IsLive:             ptr(true),
		},
		{
			Id:                 uuid.MustParse("aaaaaaaa-2222-2222-2222-222222222222"),
			GameId:             "22222222-2222-2222-2222-222222222222",
			SportsbookKey:      "pinnacle",
			SportsbookId:       uuid.MustParse("55555555-5555-5555-5555-555555555555"),
			MarketType:         "TOTAL",
			Selection:          "OVER 44.5",
			Side:               ptr(linesservice.LineSnapshotSide("OVER")),
			LineValue:          ptr(float32(44.5)),
			OddsAmerican:       102,
			OddsDecimal:        2.02,
			ImpliedProbability: 0.495,
			Timestamp:          ts,
			IsLive:             ptr(true),
		},
	}
}

// liveEdgesRawFixture mixes one live and one pregame edge. Raw maps
// rather than agentservice.EdgeListItem because the generated struct
// does not carry is_live yet (agent spec unchanged this wave).
func liveEdgesRawFixture() []map[string]any {
	return []map[string]any{
		{
			"id":                    "edge-live",
			"game_id":               "33333333-3333-3333-3333-333333333333",
			"league":                "NFL",
			"scheduled_start":       "2026-07-19T20:00:00Z",
			"market_type":           "SPREAD",
			"selection":             "DEN -1.5",
			"predicted_probability": 0.60,
			"implied_probability":   0.535,
			"edge_percentage":       6.5,
			"expected_value":        0.11,
			"odds_american":         -115,
			"sportsbook_key":        "draftkings",
			"kelly_fraction":        0.04,
			"recommended_stake":     1.2,
			"detected_at":           "2026-07-19T20:14:00Z",
			"expires_at":            "2026-07-19T20:17:00Z",
			"is_stale":              false,
			"has_paper_bet":         false,
			"home_team":             "DEN",
			"away_team":             "KC",
			"is_live":               true,
		},
		{
			"id":                    "edge-pregame",
			"game_id":               "44444444-4444-4444-4444-444444444444",
			"league":                "NFL",
			"scheduled_start":       "2026-07-20T00:20:00Z",
			"market_type":           "TOTAL",
			"selection":             "UNDER 51.5",
			"predicted_probability": 0.55,
			"implied_probability":   0.52,
			"edge_percentage":       3.0,
			"expected_value":        0.06,
			"odds_american":         -108,
			"sportsbook_key":        "fanduel",
			"kelly_fraction":        0.02,
			"recommended_stake":     0.5,
			"detected_at":           "2026-07-19T18:00:00Z",
			"expires_at":            "2026-07-20T00:20:00Z",
			"is_stale":              false,
			"has_paper_bet":         false,
			"home_team":             "PHI",
			"away_team":             "DAL",
		},
	}
}

// liveStubs starts lines-service and agent stubs and returns their env
// plus per-service request counters.
func liveStubs(t *testing.T, lines any, edges any) (env map[string]string, linesCalls, edgesCalls *atomic.Int32, linesQuery, edgesQuery *map[string][]string) {
	t.Helper()
	linesCalls, edgesCalls = &atomic.Int32{}, &atomic.Int32{}
	linesQuery, edgesQuery = &map[string][]string{}, &map[string][]string{}

	linesSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/lines/current" {
			t.Errorf("lines path = %s, want /api/v1/lines/current", r.URL.Path)
		}
		linesCalls.Add(1)
		*linesQuery = r.URL.Query()
		writeEnvelope(t, w, http.StatusOK, lines)
	}))
	t.Cleanup(linesSrv.Close)

	agentSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/agent/edges" {
			t.Errorf("agent path = %s, want /api/v1/agent/edges", r.URL.Path)
		}
		edgesCalls.Add(1)
		*edgesQuery = r.URL.Query()
		writePagedEnvelope(t, w, http.StatusOK, edges)
	}))
	t.Cleanup(agentSrv.Close)

	env = map[string]string{"LINES_SERVICE_URL": linesSrv.URL, "AGENT_URL": agentSrv.URL}
	return env, linesCalls, edgesCalls, linesQuery, edgesQuery
}

func TestLiveTable(t *testing.T) {
	env, _, _, linesQuery, edgesQuery := liveStubs(t, liveLinesFixture(), liveEdgesRawFixture())

	res := runBB(t, env, "live", "--league", "NFL")
	if res.code != api.ExitOK {
		t.Fatalf("exit code = %d, stderr: %s", res.code, res.stderr)
	}

	if got := (*linesQuery)["is_live"]; len(got) != 1 || got[0] != "true" {
		t.Errorf("lines is_live query = %v, want [true]", got)
	}
	if got := (*linesQuery)["league"]; len(got) != 1 || got[0] != "NFL" {
		t.Errorf("lines league query = %v, want [NFL]", got)
	}
	if got := (*edgesQuery)["league"]; len(got) != 1 || got[0] != "NFL" {
		t.Errorf("edges league query = %v, want [NFL]", got)
	}

	for _, want := range []string{
		"LIVE LINES", "LEAGUE", "GAME", "MARKET", "SELECTION", "SIDE", "LINE", "ODDS", "BOOK", "UPDATED",
		"33333333", "DEN -1.5", "HOME", "-115", "draftkings",
		"22222222", "OVER 44.5", "OVER", "44.5", "+102", "pinnacle",
		"LIVE EDGES", "MODEL %", "EDGE", "EXPIRES",
		"KC @ DEN", "60.0%", "+6.5%",
	} {
		if !strings.Contains(res.stdout, want) {
			t.Errorf("stdout missing %q:\n%s", want, res.stdout)
		}
	}

	// Grouped by game: the TOTAL game (2222...) sorts before the SPREAD game.
	if i, j := strings.Index(res.stdout, "22222222"), strings.Index(res.stdout, "33333333"); i > j {
		t.Errorf("lines not grouped/sorted by game id:\n%s", res.stdout)
	}
	// The pregame edge is filtered out client-side.
	if strings.Contains(res.stdout, "UNDER 51.5") || strings.Contains(res.stdout, "DAL @ PHI") {
		t.Errorf("stdout contains pregame edge:\n%s", res.stdout)
	}
}

func TestLiveEmptyState(t *testing.T) {
	env, _, _, _, _ := liveStubs(t, []linesservice.LineSnapshot{}, []map[string]any{})

	res := runBB(t, env, "live")
	if res.code != api.ExitOK {
		t.Fatalf("exit code = %d, stderr: %s", res.code, res.stderr)
	}
	if !strings.Contains(res.stdout, "Nothing is live right now.") {
		t.Errorf("stdout missing empty-state message:\n%s", res.stdout)
	}
	if strings.Contains(res.stdout, "LIVE LINES") {
		t.Errorf("stdout should not render section headers when nothing is live:\n%s", res.stdout)
	}
}

func TestLiveJSON(t *testing.T) {
	env, _, _, _, _ := liveStubs(t, liveLinesFixture(), liveEdgesRawFixture())

	res := runBB(t, env, "live", "--format", "json")
	if res.code != api.ExitOK {
		t.Fatalf("exit code = %d, stderr: %s", res.code, res.stderr)
	}

	var got struct {
		LiveLines []map[string]any `json:"live_lines"`
		LiveEdges []map[string]any `json:"live_edges"`
	}
	if err := json.Unmarshal([]byte(res.stdout), &got); err != nil {
		t.Fatalf("stdout not JSON: %v\n%s", err, res.stdout)
	}
	if len(got.LiveLines) != 2 {
		t.Errorf("live_lines count = %d, want 2", len(got.LiveLines))
	}
	if len(got.LiveEdges) != 1 {
		t.Fatalf("live_edges count = %d, want 1 (pregame edge filtered)", len(got.LiveEdges))
	}
	// Raw JSON passthrough preserves is_live, which the typed client drops.
	if got.LiveEdges[0]["id"] != "edge-live" || got.LiveEdges[0]["is_live"] != true {
		t.Errorf("live_edges[0] = %v", got.LiveEdges[0])
	}
}

func TestLiveWatchIntervalTooSmall(t *testing.T) {
	res := runBB(t, nil, "live", "--watch", "2")
	if res.code != api.ExitUsage {
		t.Fatalf("exit code = %d, want %d", res.code, api.ExitUsage)
	}
	if !strings.Contains(res.stderr, "minimum interval is 5 seconds") {
		t.Errorf("stderr = %q", res.stderr)
	}
}

// TestLiveWatchSingleIteration cancels the watch context after the first
// render: the loop must render once, then exit cleanly on ctx.Done
// without waiting out the 5s poll interval.
func TestLiveWatchSingleIteration(t *testing.T) {
	env, linesCalls, edgesCalls, _, _ := liveStubs(t, liveLinesFixture(), liveEdgesRawFixture())

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	// The first render (two localhost round trips) completes in
	// milliseconds; the next poll is 5s away, so canceling at 1s exits
	// the loop after exactly one iteration.
	timer := time.AfterFunc(time.Second, cancel)
	defer timer.Stop()

	start := time.Now()
	res := runBBContext(t, ctx, env, "live", "--watch", "5")
	if res.code != api.ExitOK {
		t.Fatalf("exit code = %d, stderr: %s", res.code, res.stderr)
	}
	if elapsed := time.Since(start); elapsed >= 5*time.Second {
		t.Errorf("watch loop did not exit on cancellation (took %s)", elapsed)
	}

	if got := linesCalls.Load(); got != 1 {
		t.Errorf("lines requests = %d, want 1", got)
	}
	if got := edgesCalls.Load(); got != 1 {
		t.Errorf("edges requests = %d, want 1", got)
	}
	if !strings.Contains(res.stdout, "\033[H\033[2J") {
		t.Errorf("stdout missing clear-screen sequence:\n%q", res.stdout)
	}
	if got := strings.Count(res.stdout, "LIVE LINES"); got != 1 {
		t.Errorf("LIVE LINES rendered %d times, want 1:\n%s", got, res.stdout)
	}
}

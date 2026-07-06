package cmd

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/Bookie-Breaker/bookie-breaker-cli/internal/api"
	"github.com/Bookie-Breaker/bookie-breaker-cli/internal/client/linesservice"
)

const testGameID = "22222222-2222-2222-2222-222222222222"

func lineSnapshotsFixture() []linesservice.LineSnapshot {
	ts := time.Date(2026, 7, 4, 15, 30, 0, 0, time.UTC)
	return []linesservice.LineSnapshot{
		{
			Id:                 uuid.MustParse("44444444-4444-4444-4444-444444444444"),
			GameId:             testGameID,
			SportsbookId:       uuid.MustParse("55555555-5555-5555-5555-555555555555"),
			SportsbookKey:      "draftkings",
			MarketType:         "SPREAD",
			Selection:          "PHI -2.5",
			LineValue:          ptr(float32(-2.5)),
			OddsAmerican:       -110,
			OddsDecimal:        1.91,
			ImpliedProbability: 0.524,
			Timestamp:          ts,
		},
		{
			Id:                 uuid.MustParse("66666666-6666-6666-6666-666666666666"),
			GameId:             testGameID,
			SportsbookId:       uuid.MustParse("55555555-5555-5555-5555-555555555555"),
			SportsbookKey:      "pinnacle",
			MarketType:         "MONEYLINE",
			Selection:          "PHI",
			LineValue:          nil,
			OddsAmerican:       135,
			OddsDecimal:        2.35,
			ImpliedProbability: 0.4255,
			Timestamp:          ts,
		},
		// Three-way soccer moneyline outcome (ADR-027): side DRAW.
		{
			Id:                 uuid.MustParse("99999999-9999-9999-9999-999999999999"),
			GameId:             testGameID,
			SportsbookId:       uuid.MustParse("55555555-5555-5555-5555-555555555555"),
			SportsbookKey:      "pinnacle",
			MarketType:         "MONEYLINE",
			Selection:          "Draw",
			Side:               ptr(linesservice.DRAW),
			LineValue:          nil,
			OddsAmerican:       240,
			OddsDecimal:        3.4,
			ImpliedProbability: 0.294,
			Timestamp:          ts,
		},
	}
}

func TestLinesTable(t *testing.T) {
	var gotQuery map[string][]string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		wantPath := "/api/v1/lines/game/" + testGameID
		if r.URL.Path != wantPath {
			t.Errorf("path = %s, want %s", r.URL.Path, wantPath)
		}
		gotQuery = r.URL.Query()
		writeEnvelope(t, w, http.StatusOK, lineSnapshotsFixture())
	}))
	defer srv.Close()

	res := runBB(t, map[string]string{"LINES_SERVICE_URL": srv.URL},
		"lines", testGameID, "--market", "SPREAD", "--book", "draftkings", "--side", "home", "--limit", "5")
	if res.code != api.ExitOK {
		t.Fatalf("exit code = %d, stderr: %s", res.code, res.stderr)
	}
	if got := gotQuery["market_type"]; len(got) != 1 || got[0] != "SPREAD" {
		t.Errorf("market_type query = %v", got)
	}
	if got := gotQuery["sportsbook"]; len(got) != 1 || got[0] != "draftkings" {
		t.Errorf("sportsbook query = %v", got)
	}
	if got := gotQuery["side"]; len(got) != 1 || got[0] != "home" {
		t.Errorf("side query = %v", got)
	}

	for _, want := range []string{
		"BOOK", "MARKET", "SELECTION", "LINE", "ODDS", "IMPLIED %", "UPDATED",
		"draftkings", "SPREAD", "PHI -2.5", "-2.5", "-110", "52.4%",
		"pinnacle", "MONEYLINE", "+135", "42.6%", "—",
		"Draw", "+240", "29.4%",
	} {
		if !strings.Contains(res.stdout, want) {
			t.Errorf("stdout missing %q:\n%s", want, res.stdout)
		}
	}
}

func TestLinesMovementTable(t *testing.T) {
	openTS := time.Date(2026, 7, 1, 12, 0, 0, 0, time.UTC)
	laterTS := time.Date(2026, 7, 4, 15, 0, 0, 0, time.UTC)
	movements := []linesservice.LineMovement{
		{
			GameId:        ptr(testGameID),
			SportsbookKey: ptr("draftkings"),
			MarketType:    ptr("SPREAD"),
			Selection:     ptr("PHI -2.5"),
			LineSnapshots: ptr([]linesservice.LineMovementSnapshot{
				{LineValue: ptr(float32(-2)), OddsAmerican: ptr(-108), Timestamp: &openTS, IsOpening: ptr(true)},
				{LineValue: ptr(float32(-2.5)), OddsAmerican: ptr(-110), Timestamp: &laterTS, IsOpening: ptr(false)},
			}),
		},
	}

	var gotQuery map[string][]string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		wantPath := "/api/v1/lines/game/" + testGameID + "/movement"
		if r.URL.Path != wantPath {
			t.Errorf("path = %s, want %s", r.URL.Path, wantPath)
		}
		gotQuery = r.URL.Query()
		writeEnvelope(t, w, http.StatusOK, movements)
	}))
	defer srv.Close()

	res := runBB(t, map[string]string{"LINES_SERVICE_URL": srv.URL},
		"lines", testGameID, "--movement", "--book", "draftkings", "--selection", "PHI -2.5")
	if res.code != api.ExitOK {
		t.Fatalf("exit code = %d, stderr: %s", res.code, res.stderr)
	}
	// --market defaults to SPREAD in movement mode.
	if got := gotQuery["market_type"]; len(got) != 1 || got[0] != "SPREAD" {
		t.Errorf("market_type query = %v", got)
	}

	for _, want := range []string{
		"TIME", "BOOK", "SELECTION", "LINE", "ODDS",
		"(open)", "draftkings", "PHI -2.5", "-2.0", "-108", "-2.5", "-110",
	} {
		if !strings.Contains(res.stdout, want) {
			t.Errorf("stdout missing %q:\n%s", want, res.stdout)
		}
	}
}

func TestLinesInvalidGameID(t *testing.T) {
	res := runBB(t, nil, "lines", "not-a-uuid")
	if res.code != api.ExitUsage {
		t.Fatalf("exit code = %d, want %d", res.code, api.ExitUsage)
	}
	if !strings.Contains(res.stderr, "invalid game id") {
		t.Errorf("stderr = %q", res.stderr)
	}
}

func TestLinesNotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeErrorEnvelope(t, w, http.StatusNotFound, "NOT_FOUND", "game not found")
	}))
	defer srv.Close()

	res := runBB(t, map[string]string{"LINES_SERVICE_URL": srv.URL}, "lines", testGameID)
	if res.code != api.ExitAPIError {
		t.Fatalf("exit code = %d, want %d", res.code, api.ExitAPIError)
	}
	if !strings.Contains(res.stderr, "NOT_FOUND: game not found") {
		t.Errorf("stderr = %q", res.stderr)
	}
}

package cmd

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/Bookie-Breaker/bookie-breaker-cli/internal/api"
	"github.com/Bookie-Breaker/bookie-breaker-cli/internal/client/bookieemulator"
)

func placedBetFixture() bookieemulator.BetData {
	return bookieemulator.BetData{
		Id:                   uuid.MustParse("77777777-7777-7777-7777-777777777777"),
		GameId:               ptr(uuid.MustParse(testGameID)),
		GameExternalId:       "DAL@PHI-2026-07-04",
		SportsbookKey:        "draftkings",
		MarketType:           "SPREAD",
		Selection:            "PHI -2.5",
		Side:                 ptr("HOME"),
		LineValue:            ptr(float32(-2.5)),
		OddsAmerican:         -110,
		OddsDecimal:          1.91,
		Stake:                1.5,
		StakeDollars:         150,
		PredictedProbability: 0.56,
		EdgePercentage:       3.6,
		KellyFraction:        0.03,
		Result:               "PENDING",
		PlacedAt:             time.Date(2026, 7, 4, 16, 0, 0, 0, time.UTC),
	}
}

func TestBetPlace(t *testing.T) {
	var (
		gotIdempotencyKey string
		gotBody           map[string]any
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1/emulator/bets" {
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		gotIdempotencyKey = r.Header.Get("X-Idempotency-Key")
		raw, _ := io.ReadAll(r.Body)
		if err := json.Unmarshal(raw, &gotBody); err != nil {
			t.Errorf("request body not JSON: %v", err)
		}
		writeEnvelope(t, w, http.StatusCreated, placedBetFixture())
	}))
	defer srv.Close()

	res := runBB(t, map[string]string{"BOOKIE_EMULATOR_URL": srv.URL},
		"bet", "place",
		"--game", testGameID,
		"--market", "SPREAD",
		"--selection", "PHI -2.5",
		"--side", "home",
		"--stake", "1.5",
		"--prob", "0.56",
		"--edge", "3.6",
		"--book", "draftkings",
		"--kelly", "0.03",
		"--reason", "model edge",
	)
	if res.code != api.ExitOK {
		t.Fatalf("exit code = %d, stderr: %s", res.code, res.stderr)
	}

	// A fresh UUID idempotency key is generated when none is supplied.
	if _, err := uuid.Parse(gotIdempotencyKey); err != nil {
		t.Errorf("X-Idempotency-Key = %q, want a UUID", gotIdempotencyKey)
	}

	wantBody := map[string]any{
		"game_id":               testGameID,
		"market_type":           "SPREAD",
		"selection":             "PHI -2.5",
		"side":                  "HOME",
		"stake":                 1.5,
		"predicted_probability": 0.56,
		"edge_percentage":       3.6,
		"sportsbook_key":        "draftkings",
		"kelly_fraction":        0.03,
		"reasoning":             "model edge",
	}
	for key, want := range wantBody {
		if got := gotBody[key]; got != want {
			t.Errorf("body[%s] = %v (%T), want %v", key, got, got, want)
		}
	}

	for _, want := range []string{
		"Bet placed", "77777777-7777-7777-7777-777777777777", "DAL@PHI-2026-07-04",
		"PHI -2.5", "HOME", "-2.5", "-110", "1.50u", "draftkings", "PENDING",
	} {
		if !strings.Contains(res.stdout, want) {
			t.Errorf("stdout missing %q:\n%s", want, res.stdout)
		}
	}
}

// drawBetFixture is a three-way moneyline DRAW bet (ADR-027).
func drawBetFixture() bookieemulator.BetData {
	b := placedBetFixture()
	b.Id = uuid.MustParse("aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa")
	b.GameExternalId = "CHE@ARS-2026-07-05"
	b.MarketType = "MONEYLINE"
	b.Selection = "Draw"
	b.Side = ptr("DRAW")
	b.LineValue = nil
	b.OddsAmerican = 240
	b.OddsDecimal = 3.4
	b.PredictedProbability = 0.31
	b.EdgePercentage = 1.6
	return b
}

func TestBetPlaceDrawSide(t *testing.T) {
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		if err := json.Unmarshal(raw, &gotBody); err != nil {
			t.Errorf("request body not JSON: %v", err)
		}
		writeEnvelope(t, w, http.StatusCreated, drawBetFixture())
	}))
	defer srv.Close()

	res := runBB(t, map[string]string{"BOOKIE_EMULATOR_URL": srv.URL},
		"bet", "place",
		"--game", testGameID,
		"--market", "moneyline",
		"--selection", "Draw",
		"--side", "draw",
		"--stake", "1.5",
		"--prob", "0.31",
		"--edge", "1.6",
	)
	if res.code != api.ExitOK {
		t.Fatalf("exit code = %d, stderr: %s", res.code, res.stderr)
	}

	// The lowercased flag is upper-cased into the DRAW enum on the wire.
	if got := gotBody["side"]; got != "DRAW" {
		t.Errorf("body[side] = %v, want DRAW", got)
	}
	if got := gotBody["market_type"]; got != "MONEYLINE" {
		t.Errorf("body[market_type] = %v, want MONEYLINE", got)
	}

	for _, want := range []string{"Bet placed", "CHE@ARS-2026-07-05", "Draw", "DRAW", "MONEYLINE", "+240"} {
		if !strings.Contains(res.stdout, want) {
			t.Errorf("stdout missing %q:\n%s", want, res.stdout)
		}
	}
}

func TestBetPlaceExplicitIdempotencyKeyAndJSON(t *testing.T) {
	const key = "99999999-9999-9999-9999-999999999999"
	var gotIdempotencyKey string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotIdempotencyKey = r.Header.Get("X-Idempotency-Key")
		writeEnvelope(t, w, http.StatusCreated, placedBetFixture())
	}))
	defer srv.Close()

	res := runBB(t, map[string]string{"BOOKIE_EMULATOR_URL": srv.URL},
		"bet", "place", "--format", "json",
		"--game", testGameID, "--market", "SPREAD", "--selection", "PHI -2.5",
		"--side", "HOME", "--stake", "1.5", "--prob", "0.56", "--edge", "3.6",
		"--idempotency-key", key,
	)
	if res.code != api.ExitOK {
		t.Fatalf("exit code = %d, stderr: %s", res.code, res.stderr)
	}
	if gotIdempotencyKey != key {
		t.Errorf("X-Idempotency-Key = %q, want %q", gotIdempotencyKey, key)
	}
	mustJSONEqual(t, res.stdout, placedBetFixture())
}

func TestBetPlaceMissingRequiredFlags(t *testing.T) {
	res := runBB(t, nil, "bet", "place", "--game", testGameID)
	if res.code != api.ExitUsage {
		t.Fatalf("exit code = %d, want %d", res.code, api.ExitUsage)
	}
	if !strings.Contains(res.stderr, "required flag(s)") {
		t.Errorf("stderr = %q", res.stderr)
	}
}

func TestBetList(t *testing.T) {
	graded := placedBetFixture()
	graded.Id = uuid.MustParse("88888888-8888-8888-8888-888888888888")
	graded.Result = "WIN"
	graded.ProfitLoss = ptr(float32(1.36))
	graded.Clv = ptr(float32(0.8))
	graded.GradedAt = ptr(time.Date(2026, 7, 5, 4, 0, 0, 0, time.UTC))
	bets := []bookieemulator.BetData{placedBetFixture(), graded, drawBetFixture()}

	var gotQuery map[string][]string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/emulator/bets" {
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		gotQuery = r.URL.Query()
		writePagedEnvelope(t, w, http.StatusOK, bets)
	}))
	defer srv.Close()

	res := runBB(t, map[string]string{"BOOKIE_EMULATOR_URL": srv.URL},
		"bet", "list", "--league", "NFL", "--status", "graded", "--market", "spread",
		"--min-edge", "2", "--from", "2026-07-01", "--limit", "25")
	if res.code != api.ExitOK {
		t.Fatalf("exit code = %d, stderr: %s", res.code, res.stderr)
	}

	if got := gotQuery["league"]; len(got) != 1 || got[0] != "NFL" {
		t.Errorf("league query = %v", got)
	}
	if got := gotQuery["status"]; len(got) != 1 || got[0] != "graded" {
		t.Errorf("status query = %v", got)
	}
	if got := gotQuery["market_type"]; len(got) != 1 || got[0] != "SPREAD" {
		t.Errorf("market_type query = %v", got)
	}
	if got := gotQuery["min_edge"]; len(got) != 1 || got[0] != "2" {
		t.Errorf("min_edge query = %v", got)
	}
	if got := gotQuery["limit"]; len(got) != 1 || got[0] != "25" {
		t.Errorf("limit query = %v", got)
	}
	if got := gotQuery["date_from"]; len(got) != 1 || !strings.HasPrefix(got[0], "2026-07-01") {
		t.Errorf("date_from query = %v", got)
	}

	for _, want := range []string{
		"PLACED", "GAME", "MARKET", "SELECTION", "ODDS", "STAKE", "RESULT", "P&L", "CLV",
		"DAL@PHI-2026-07-04", "SPREAD", "PHI -2.5", "-110", "1.50",
		"PENDING", "WIN", "+1.36", "+0.80", "—",
		"CHE@ARS-2026-07-05", "MONEYLINE", "Draw", "+240",
	} {
		if !strings.Contains(res.stdout, want) {
			t.Errorf("stdout missing %q:\n%s", want, res.stdout)
		}
	}
}

func TestBetListJSON(t *testing.T) {
	bets := []bookieemulator.BetData{placedBetFixture()}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writePagedEnvelope(t, w, http.StatusOK, bets)
	}))
	defer srv.Close()

	res := runBB(t, map[string]string{"BOOKIE_EMULATOR_URL": srv.URL}, "bet", "list", "--format", "json")
	if res.code != api.ExitOK {
		t.Fatalf("exit code = %d, stderr: %s", res.code, res.stderr)
	}
	mustJSONEqual(t, res.stdout, bets)
}

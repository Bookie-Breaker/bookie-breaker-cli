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
	"github.com/Bookie-Breaker/bookie-breaker-cli/internal/client/agentservice"
	"github.com/Bookie-Breaker/bookie-breaker-cli/internal/client/bookieemulator"
)

func TestParseLegs(t *testing.T) {
	tests := []struct {
		name    string
		specs   []string
		want    []agentservice.ParlayLegRequest
		wantErr string
	}{
		{
			name:  "two legs, one with a line",
			specs: []string{"wc-semi-1:MONEYLINE:HOME", "wc-semi-1:TOTAL:OVER:2.5"},
			want: []agentservice.ParlayLegRequest{
				{GameExternalId: "wc-semi-1", MarketType: "MONEYLINE", Side: "HOME"},
				{GameExternalId: "wc-semi-1", MarketType: "TOTAL", Side: "OVER", LineValue: ptr(float32(2.5))},
			},
		},
		{
			name:  "lowercase market and side are normalized",
			specs: []string{"g1:moneyline:away", "g2:spread:home:-3.5"},
			want: []agentservice.ParlayLegRequest{
				{GameExternalId: "g1", MarketType: "MONEYLINE", Side: "AWAY"},
				{GameExternalId: "g2", MarketType: "SPREAD", Side: "HOME", LineValue: ptr(float32(-3.5))},
			},
		},
		{
			name:  "draw side accepted",
			specs: []string{"g1:MONEYLINE:DRAW", "g2:MONEYLINE:HOME"},
			want: []agentservice.ParlayLegRequest{
				{GameExternalId: "g1", MarketType: "MONEYLINE", Side: "DRAW"},
				{GameExternalId: "g2", MarketType: "MONEYLINE", Side: "HOME"},
			},
		},
		{
			name:    "too few legs",
			specs:   []string{"g1:MONEYLINE:HOME"},
			wantErr: "needs 2-6 --leg flags",
		},
		{
			name: "too many legs",
			specs: []string{
				"g1:MONEYLINE:HOME", "g2:MONEYLINE:HOME", "g3:MONEYLINE:HOME",
				"g4:MONEYLINE:HOME", "g5:MONEYLINE:HOME", "g6:MONEYLINE:HOME",
				"g7:MONEYLINE:HOME",
			},
			wantErr: "needs 2-6 --leg flags",
		},
		{
			name:    "missing side",
			specs:   []string{"g1:MONEYLINE", "g2:MONEYLINE:HOME"},
			wantErr: "expected game_ext:MARKET:SIDE[:line]",
		},
		{
			name:    "too many segments",
			specs:   []string{"g1:TOTAL:OVER:2.5:extra", "g2:MONEYLINE:HOME"},
			wantErr: "expected game_ext:MARKET:SIDE[:line]",
		},
		{
			name:    "empty game id",
			specs:   []string{":MONEYLINE:HOME", "g2:MONEYLINE:HOME"},
			wantErr: "empty game id",
		},
		{
			name:    "unsupported market",
			specs:   []string{"g1:PLAYER_PROP:OVER:9.5", "g2:MONEYLINE:HOME"},
			wantErr: "market must be MONEYLINE, SPREAD, or TOTAL",
		},
		{
			name:    "invalid side",
			specs:   []string{"g1:MONEYLINE:UP", "g2:MONEYLINE:HOME"},
			wantErr: "side must be HOME, AWAY, DRAW, OVER, or UNDER",
		},
		{
			name:    "non-numeric line",
			specs:   []string{"g1:TOTAL:OVER:abc", "g2:MONEYLINE:HOME"},
			wantErr: "is not a number",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseLegs(tt.specs)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("err = %v, want containing %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			gotJSON, _ := json.Marshal(got)
			wantJSON, _ := json.Marshal(tt.want)
			if string(gotJSON) != string(wantJSON) {
				t.Errorf("legs = %s, want %s", gotJSON, wantJSON)
			}
		})
	}
}

func parlayEvaluationFixture() agentservice.ParlayEvaluationData {
	return agentservice.ParlayEvaluationData{
		League:                 "WC2026",
		IsSameGame:             true,
		JointProbability:       0.31,
		IndependentProbability: 0.27,
		CorrelationEdge:        4.0,
		Correlations:           map[string]float32{"leg0:leg1": 0.22},
		CombinedOddsAmerican:   275,
		CombinedOddsDecimal:    3.75,
		EvPct:                  6.3,
		ExpectedValue:          0.063,
		ExpiresAt:              "2026-07-19T18:00:00Z",
		KellyFraction:          0.021,
		RecommendedStake:       1.05,
		MeetsThreshold:         true,
		Method:                 "sim_joint",
		ParlayId:               ptr("55555555-5555-5555-5555-555555555555"),
		Legs: []agentservice.ParlayLegData{
			{
				GameExternalId:       "wc-semi-1",
				GameId:               testGameID,
				MarketType:           "MONEYLINE",
				Side:                 "HOME",
				Selection:            "France",
				OddsAmerican:         -120,
				OddsDecimal:          1.83,
				PredictedProbability: 0.58,
				SportsbookKey:        "draftkings",
				SimLegKey:            ptr("home_win"),
			},
			{
				GameExternalId:       "wc-semi-1",
				GameId:               testGameID,
				MarketType:           "TOTAL",
				Side:                 "OVER",
				Selection:            "Over 2.5",
				LineValue:            ptr(float32(2.5)),
				OddsAmerican:         110,
				OddsDecimal:          2.1,
				PredictedProbability: 0.52,
				SportsbookKey:        "draftkings",
				SimLegKey:            ptr("total_over_2.5"),
			},
		},
	}
}

// newEvaluateServer fakes the agent's parlay evaluate endpoint and captures
// the request body.
func newEvaluateServer(t *testing.T, gotBody *map[string]any) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1/agent/parlays/evaluate" {
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		raw, _ := io.ReadAll(r.Body)
		if err := json.Unmarshal(raw, gotBody); err != nil {
			t.Errorf("request body not JSON: %v", err)
		}
		writeEnvelope(t, w, http.StatusOK, parlayEvaluationFixture())
	}))
}

func TestParlayEvaluate(t *testing.T) {
	var gotBody map[string]any
	srv := newEvaluateServer(t, &gotBody)
	defer srv.Close()

	res := runBB(t, map[string]string{"AGENT_URL": srv.URL},
		"parlay", "evaluate",
		"--leg", "wc-semi-1:MONEYLINE:HOME",
		"--leg", "wc-semi-1:TOTAL:OVER:2.5",
		"--odds", "275",
		"--persist",
	)
	if res.code != api.ExitOK {
		t.Fatalf("exit code = %d, stderr: %s", res.code, res.stderr)
	}

	if got := gotBody["parlay_odds_american"]; got != float64(275) {
		t.Errorf("body[parlay_odds_american] = %v, want 275", got)
	}
	if got := gotBody["persist"]; got != true {
		t.Errorf("body[persist] = %v, want true", got)
	}
	legs, _ := gotBody["legs"].([]any)
	if len(legs) != 2 {
		t.Fatalf("body[legs] = %v, want 2 legs", gotBody["legs"])
	}
	leg0, _ := legs[0].(map[string]any)
	if leg0["game_external_id"] != "wc-semi-1" || leg0["market_type"] != "MONEYLINE" || leg0["side"] != "HOME" {
		t.Errorf("leg0 = %v", leg0)
	}
	leg1, _ := legs[1].(map[string]any)
	if leg1["line_value"] != float64(2.5) {
		t.Errorf("leg1[line_value] = %v, want 2.5", leg1["line_value"])
	}

	for _, want := range []string{
		// Legs table.
		"GAME", "MARKET", "SIDE", "LINE", "ODDS", "PROB", "SIM KEY",
		"wc-semi-1", "MONEYLINE", "HOME", "-120", "58.0%", "home_win",
		"TOTAL", "OVER", "+2.5", "+110", "52.0%", "total_over_2.5",
		// Summary block.
		"Parlay evaluation",
		"Joint prob", "31.0%",
		"Independent prob", "27.0%",
		"Correlation edge", "+4.0%",
		"Combined odds", "+275 (3.75)",
		"EV", "+6.3%",
		"Method", "sim_joint",
		"Kelly", "0.0210",
		"Recommended stake", "1.05u",
		"Meets threshold", "YES",
	} {
		if !strings.Contains(res.stdout, want) {
			t.Errorf("stdout missing %q:\n%s", want, res.stdout)
		}
	}
}

func TestParlayEvaluateJSON(t *testing.T) {
	var gotBody map[string]any
	srv := newEvaluateServer(t, &gotBody)
	defer srv.Close()

	res := runBB(t, map[string]string{"AGENT_URL": srv.URL},
		"parlay", "evaluate", "--format", "json",
		"--leg", "wc-semi-1:MONEYLINE:HOME",
		"--leg", "wc-semi-1:TOTAL:OVER:2.5",
	)
	if res.code != api.ExitOK {
		t.Fatalf("exit code = %d, stderr: %s", res.code, res.stderr)
	}
	// Without --odds the offered price is omitted (null on the wire).
	if got, present := gotBody["parlay_odds_american"]; present && got != nil {
		t.Errorf("body[parlay_odds_american] = %v, want null", got)
	}
	mustJSONEqual(t, res.stdout, parlayEvaluationFixture())
}

func TestParlayEvaluateUsageErrors(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{"one leg", []string{"parlay", "evaluate", "--leg", "g1:MONEYLINE:HOME"}},
		{"malformed leg", []string{"parlay", "evaluate", "--leg", "g1", "--leg", "g2:MONEYLINE:HOME"}},
		{"no legs", []string{"parlay", "evaluate"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res := runBB(t, nil, tt.args...)
			if res.code != api.ExitUsage {
				t.Fatalf("exit code = %d, want %d, stderr: %s", res.code, api.ExitUsage, res.stderr)
			}
		})
	}
}

func parlayDetailFixture() bookieemulator.ParlayDetailData {
	parentID := uuid.MustParse("66666666-6666-6666-6666-666666666666")
	gameID := uuid.MustParse(testGameID)
	return bookieemulator.ParlayDetailData{
		Id:                   parentID,
		GameExternalId:       "PARLAY",
		IsParlay:             ptr(true),
		MarketType:           "PARLAY",
		Selection:            "2-leg parlay",
		CombinedOddsAmerican: 275,
		CombinedOddsDecimal:  3.75,
		OddsAmerican:         275,
		OddsDecimal:          3.75,
		Stake:                1.5,
		StakeDollars:         150,
		PredictedProbability: 0.31,
		EdgePercentage:       6.3,
		KellyFraction:        0.021,
		Result:               "PENDING",
		PlacedAt:             time.Date(2026, 7, 19, 15, 0, 0, 0, time.UTC),
		Legs: []bookieemulator.ParlayLegData{
			{
				Id:             uuid.MustParse("aaaaaaaa-1111-1111-1111-111111111111"),
				GameId:         ptr(gameID),
				GameExternalId: "wc-semi-1",
				League:         "WC2026",
				LegIndex:       0,
				LegStatus:      "PENDING",
				MarketType:     "MONEYLINE",
				Selection:      "France",
				Side:           ptr("HOME"),
				OddsAmerican:   -120,
				OddsDecimal:    1.83,
			},
			{
				Id:             uuid.MustParse("aaaaaaaa-2222-2222-2222-222222222222"),
				GameId:         ptr(gameID),
				GameExternalId: "wc-semi-1",
				League:         "WC2026",
				LegIndex:       1,
				LegStatus:      "WIN",
				MarketType:     "TOTAL",
				Selection:      "Over 2.5",
				Side:           ptr("OVER"),
				LineValue:      ptr(float32(2.5)),
				OddsAmerican:   110,
				OddsDecimal:    2.1,
			},
		},
	}
}

func TestParlayPlaceWithYes(t *testing.T) {
	agentSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/agent/parlays/evaluate" {
			t.Errorf("unexpected agent path %s", r.URL.Path)
		}
		writeEnvelope(t, w, http.StatusOK, parlayEvaluationFixture())
	}))
	defer agentSrv.Close()

	var (
		gotIdempotencyKey string
		gotBody           map[string]any
	)
	emulatorSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1/emulator/parlays" {
			t.Errorf("unexpected emulator request %s %s", r.Method, r.URL.Path)
		}
		gotIdempotencyKey = r.Header.Get("X-Idempotency-Key")
		raw, _ := io.ReadAll(r.Body)
		if err := json.Unmarshal(raw, &gotBody); err != nil {
			t.Errorf("request body not JSON: %v", err)
		}
		writeEnvelope(t, w, http.StatusCreated, parlayDetailFixture())
	}))
	defer emulatorSrv.Close()

	res := runBB(t, map[string]string{"AGENT_URL": agentSrv.URL, "BOOKIE_EMULATOR_URL": emulatorSrv.URL},
		"parlay", "place",
		"--leg", "wc-semi-1:MONEYLINE:HOME",
		"--leg", "wc-semi-1:TOTAL:OVER:2.5",
		"--stake", "1.5",
		"--yes",
	)
	if res.code != api.ExitOK {
		t.Fatalf("exit code = %d, stderr: %s", res.code, res.stderr)
	}

	if _, err := uuid.Parse(gotIdempotencyKey); err != nil {
		t.Errorf("X-Idempotency-Key = %q, want a UUID", gotIdempotencyKey)
	}
	if got := gotBody["stake"]; got != float64(1.5) {
		t.Errorf("body[stake] = %v, want 1.5", got)
	}
	if got := gotBody["predicted_probability"]; got != float64(0.31) {
		t.Errorf("body[predicted_probability] = %v, want 0.31", got)
	}
	if got := gotBody["edge_percentage"]; got != float64(6.3) {
		t.Errorf("body[edge_percentage] = %v, want 6.3", got)
	}
	legs, _ := gotBody["legs"].([]any)
	if len(legs) != 2 {
		t.Fatalf("body[legs] = %v, want 2 legs", gotBody["legs"])
	}
	leg0, _ := legs[0].(map[string]any)
	if leg0["game_id"] != testGameID || leg0["selection"] != "France" ||
		leg0["market_type"] != "MONEYLINE" || leg0["side"] != "HOME" ||
		leg0["sportsbook_key"] != "draftkings" {
		t.Errorf("leg0 = %v", leg0)
	}

	for _, want := range []string{
		"Parlay placed", "66666666-6666-6666-6666-666666666666", "+275", "1.50u", "PENDING",
		"#", "GAME", "MARKET", "SELECTION", "SIDE", "LINE", "ODDS", "STATUS",
		"wc-semi-1", "France", "HOME", "Over 2.5", "OVER", "+2.5", "WIN",
	} {
		if !strings.Contains(res.stdout, want) {
			t.Errorf("stdout missing %q:\n%s", want, res.stdout)
		}
	}
	// --yes skips the confirmation prompt.
	if strings.Contains(res.stdout, "Place this parlay") {
		t.Errorf("stdout unexpectedly contains the confirm prompt:\n%s", res.stdout)
	}
}

func TestParlayPlaceDeclinedPrompt(t *testing.T) {
	agentSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeEnvelope(t, w, http.StatusOK, parlayEvaluationFixture())
	}))
	defer agentSrv.Close()

	emulatorCalled := false
	emulatorSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		emulatorCalled = true
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer emulatorSrv.Close()

	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	for _, key := range []string{
		"AGENT_URL", "LINES_SERVICE_URL", "STATISTICS_SERVICE_URL",
		"BOOKIE_EMULATOR_URL", "PREDICTION_ENGINE_URL",
	} {
		t.Setenv(key, "")
	}
	t.Setenv("AGENT_URL", agentSrv.URL)
	t.Setenv("BOOKIE_EMULATOR_URL", emulatorSrv.URL)

	root := NewRootCmd()
	var out, errOut strings.Builder
	root.SetOut(&out)
	root.SetErr(&errOut)
	root.SetIn(strings.NewReader("n\n"))
	root.SetArgs([]string{
		"parlay", "place",
		"--leg", "wc-semi-1:MONEYLINE:HOME",
		"--leg", "wc-semi-1:TOTAL:OVER:2.5",
		"--stake", "1.5",
	})

	code := run(root, &errOut)
	if code != api.ExitOK {
		t.Fatalf("exit code = %d, stderr: %s", code, errOut.String())
	}
	if emulatorCalled {
		t.Error("emulator was called despite a declined prompt")
	}
	stdout := out.String()
	for _, want := range []string{"Parlay evaluation", "Place this parlay for 1.50u? [y/N]", "Aborted."} {
		if !strings.Contains(stdout, want) {
			t.Errorf("stdout missing %q:\n%s", want, stdout)
		}
	}
}

func TestParlayShow(t *testing.T) {
	const parentID = "66666666-6666-6666-6666-666666666666"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/api/v1/emulator/parlays/"+parentID {
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		writeEnvelope(t, w, http.StatusOK, parlayDetailFixture())
	}))
	defer srv.Close()

	res := runBB(t, map[string]string{"BOOKIE_EMULATOR_URL": srv.URL},
		"parlay", "show", parentID)
	if res.code != api.ExitOK {
		t.Fatalf("exit code = %d, stderr: %s", res.code, res.stderr)
	}

	for _, want := range []string{
		"Parlay", parentID,
		"Combined odds", "+275 (3.75)",
		"Stake", "1.50u",
		"Prob", "31.0%",
		"Edge", "+6.3%",
		"Result", "PENDING",
		"#", "GAME", "MARKET", "SELECTION", "SIDE", "LINE", "ODDS", "STATUS",
		"1", "wc-semi-1", "MONEYLINE", "France", "HOME", "-120",
		"2", "TOTAL", "Over 2.5", "OVER", "+2.5", "+110", "WIN",
	} {
		if !strings.Contains(res.stdout, want) {
			t.Errorf("stdout missing %q:\n%s", want, res.stdout)
		}
	}
}

func TestParlayShowJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeEnvelope(t, w, http.StatusOK, parlayDetailFixture())
	}))
	defer srv.Close()

	res := runBB(t, map[string]string{"BOOKIE_EMULATOR_URL": srv.URL},
		"parlay", "show", "--format", "json", "66666666-6666-6666-6666-666666666666")
	if res.code != api.ExitOK {
		t.Fatalf("exit code = %d, stderr: %s", res.code, res.stderr)
	}
	mustJSONEqual(t, res.stdout, parlayDetailFixture())
}

func TestParlayShowInvalidID(t *testing.T) {
	res := runBB(t, nil, "parlay", "show", "not-a-uuid")
	if res.code != api.ExitUsage {
		t.Fatalf("exit code = %d, want %d, stderr: %s", res.code, api.ExitUsage, res.stderr)
	}
	if !strings.Contains(res.stderr, "invalid bet_id") {
		t.Errorf("stderr = %q", res.stderr)
	}
}

func TestParlayEvaluateAPIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		writeErrorEnvelope(t, w, http.StatusUnprocessableEntity, "MIXED_LEAGUES", "legs span multiple leagues")
	}))
	defer srv.Close()

	res := runBB(t, map[string]string{"AGENT_URL": srv.URL},
		"parlay", "evaluate",
		"--leg", "g1:MONEYLINE:HOME",
		"--leg", "g2:MONEYLINE:AWAY",
	)
	if res.code != api.ExitAPIError {
		t.Fatalf("exit code = %d, want %d", res.code, api.ExitAPIError)
	}
	if !strings.Contains(res.stderr, "legs span multiple leagues") {
		t.Errorf("stderr = %q", res.stderr)
	}
}

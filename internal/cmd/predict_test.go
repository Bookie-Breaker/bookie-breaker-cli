package cmd

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Bookie-Breaker/bookie-breaker-cli/internal/api"
	"github.com/Bookie-Breaker/bookie-breaker-cli/internal/client/predictionengine"
)

func predictionsFixture() predictionengine.LatestPredictionsData {
	return predictionengine.LatestPredictionsData{
		GameId: "22222222-2222-2222-2222-222222222222",
		Predictions: []predictionengine.PredictionItem{
			{
				Id:                    "p1",
				MarketType:            "SPREAD",
				Selection:             "PHI -2.5",
				PredictedProbability:  0.56,
				SimulationProbability: ptr(float32(0.54)),
				AdjustmentMagnitude:   0.02,
				ConfidenceLower:       ptr(float32(0.512)),
				ConfidenceUpper:       ptr(float32(0.608)),
				ModelVersionId:        "abcdef12-3456-7890-abcd-ef1234567890",
				FeatureImportance: map[string]float32{
					"rest_days_delta":   0.18,
					"pace_differential": 0.12,
					"injury_impact":     0.09,
				},
				CreatedAt: "2026-07-04T12:00:00Z",
			},
			{
				Id:                    "p2",
				MarketType:            "MONEYLINE",
				Selection:             "PHI",
				PredictedProbability:  0.61,
				SimulationProbability: nil,
				AdjustmentMagnitude:   -0.01,
				ConfidenceLower:       nil,
				ConfidenceUpper:       nil,
				ModelVersionId:        "abcdef12-3456-7890-abcd-ef1234567890",
				FeatureImportance:     map[string]float32{},
				CreatedAt:             "2026-07-04T12:00:00Z",
			},
		},
	}
}

func TestPredictTable(t *testing.T) {
	var gotQuery map[string][]string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		wantPath := "/api/v1/predict/games/22222222-2222-2222-2222-222222222222/latest"
		if r.URL.Path != wantPath {
			t.Errorf("path = %s, want %s", r.URL.Path, wantPath)
		}
		gotQuery = r.URL.Query()
		writeEnvelope(t, w, http.StatusOK, predictionsFixture())
	}))
	defer srv.Close()

	res := runBB(t, map[string]string{"PREDICTION_ENGINE_URL": srv.URL},
		"predict", "22222222-2222-2222-2222-222222222222", "--market", "SPREAD,MONEYLINE")
	if res.code != api.ExitOK {
		t.Fatalf("exit code = %d, stderr: %s", res.code, res.stderr)
	}
	if got := gotQuery["market_type"]; len(got) != 1 || got[0] != "SPREAD,MONEYLINE" {
		t.Errorf("market_type query = %v", got)
	}

	for _, want := range []string{
		"MARKET", "SELECTION", "PROB", "90% CI", "SIM PROB", "ADJ", "MODEL",
		"SPREAD", "PHI -2.5", "56.0%", "51.2%–60.8%", "54.0%", "+0.020", "abcdef12",
		"MONEYLINE", "61.0%", "—", "-0.010",
	} {
		if !strings.Contains(res.stdout, want) {
			t.Errorf("stdout missing %q:\n%s", want, res.stdout)
		}
	}

	// Top feature-importance lines below the table, sorted by value.
	for _, want := range []string{
		"SPREAD feature importance:",
		"rest_days_delta",
		"0.18",
		"pace_differential",
		"injury_impact",
	} {
		if !strings.Contains(res.stdout, want) {
			t.Errorf("stdout missing feature importance %q:\n%s", want, res.stdout)
		}
	}
	if strings.Index(res.stdout, "rest_days_delta") > strings.Index(res.stdout, "injury_impact") {
		t.Errorf("feature importance not sorted desc:\n%s", res.stdout)
	}
}

func TestPredictJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeEnvelope(t, w, http.StatusOK, predictionsFixture())
	}))
	defer srv.Close()

	res := runBB(t, map[string]string{"PREDICTION_ENGINE_URL": srv.URL},
		"predict", "22222222-2222-2222-2222-222222222222", "--format", "json")
	if res.code != api.ExitOK {
		t.Fatalf("exit code = %d, stderr: %s", res.code, res.stderr)
	}
	mustJSONEqual(t, res.stdout, predictionsFixture())
}

func TestPredictRequiresGameID(t *testing.T) {
	res := runBB(t, nil, "predict")
	if res.code != api.ExitUsage {
		t.Fatalf("exit code = %d, want %d", res.code, api.ExitUsage)
	}
}

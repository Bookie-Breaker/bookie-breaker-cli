package cmd

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Bookie-Breaker/bookie-breaker-cli/internal/api"
	"github.com/Bookie-Breaker/bookie-breaker-cli/internal/client/bookieemulator"
)

func performanceFixture() bookieemulator.PerformanceData {
	return bookieemulator.PerformanceData{
		Period: bookieemulator.PeriodData{
			To:     time.Date(2026, 7, 4, 12, 0, 0, 0, time.UTC),
			Window: "all_time",
		},
		TotalBets:           120,
		TotalWins:           68,
		TotalLosses:         49,
		TotalPushes:         3,
		WinRate:             0.581,
		Roi:                 0.062,
		TotalWageredUnits:   140,
		TotalProfitUnits:    8.68,
		TotalWageredDollars: 14000,
		TotalProfitDollars:  868,
		AvgOddsAmerican:     ptr(-105),
		AvgEdgePercentage:   ptr(float32(3.4)),
		AvgClv:              ptr(float32(0.9)),
		LongestWinStreak:    7,
		LongestLossStreak:   4,
		BrierScore:          ptr(float32(0.2412)),
	}
}

func TestPerformanceStatBlock(t *testing.T) {
	var gotQuery map[string][]string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/emulator/performance" {
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		gotQuery = r.URL.Query()
		writeEnvelope(t, w, http.StatusOK, performanceFixture())
	}))
	defer srv.Close()

	res := runBB(t, map[string]string{"BOOKIE_EMULATOR_URL": srv.URL}, "performance", "--league", "NFL")
	if res.code != api.ExitOK {
		t.Fatalf("exit code = %d, stderr: %s", res.code, res.stderr)
	}
	if got := gotQuery["league"]; len(got) != 1 || got[0] != "NFL" {
		t.Errorf("league query = %v", got)
	}

	for _, want := range []string{
		"TOTAL BETS", "120",
		"RECORD", "68-49-3",
		"WIN RATE", "58.1%",
		"ROI", "+6.2%",
		"UNITS", "+8.68",
		"AVG EDGE", "+3.4%",
		"AVG CLV", "+0.90",
		"BRIER", "0.2412",
	} {
		if !strings.Contains(res.stdout, want) {
			t.Errorf("stdout missing %q:\n%s", want, res.stdout)
		}
	}
}

func TestPerformanceJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeEnvelope(t, w, http.StatusOK, performanceFixture())
	}))
	defer srv.Close()

	res := runBB(t, map[string]string{"BOOKIE_EMULATOR_URL": srv.URL}, "performance", "--format", "json")
	if res.code != api.ExitOK {
		t.Fatalf("exit code = %d, stderr: %s", res.code, res.stderr)
	}
	mustJSONEqual(t, res.stdout, performanceFixture())
}

func TestPerformanceBreakdown(t *testing.T) {
	breakdown := bookieemulator.BreakdownData{
		GroupBy: "league",
		Breakdowns: []bookieemulator.BreakdownEntry{
			{
				Group: "NFL", TotalBets: 80, Wins: 47, Losses: 31, Pushes: 2,
				WinRate: 0.603, Roi: 0.081, TotalProfitUnits: 6.5,
				AvgClv: ptr(float32(1.1)), AvgEdgePercentage: ptr(float32(3.8)),
			},
			{
				Group: "NBA", TotalBets: 40, Wins: 21, Losses: 18, Pushes: 1,
				WinRate: 0.538, Roi: -0.021, TotalProfitUnits: -0.9,
				AvgClv: nil, AvgEdgePercentage: nil,
			},
		},
	}

	var gotQuery map[string][]string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/emulator/performance/breakdown" {
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		gotQuery = r.URL.Query()
		writeEnvelope(t, w, http.StatusOK, breakdown)
	}))
	defer srv.Close()

	res := runBB(t, map[string]string{"BOOKIE_EMULATOR_URL": srv.URL}, "performance", "--breakdown", "league")
	if res.code != api.ExitOK {
		t.Fatalf("exit code = %d, stderr: %s", res.code, res.stderr)
	}
	if got := gotQuery["group_by"]; len(got) != 1 || got[0] != "league" {
		t.Errorf("group_by query = %v", got)
	}

	for _, want := range []string{
		"GROUP", "BETS", "RECORD", "WIN RATE", "ROI", "UNITS", "AVG EDGE", "AVG CLV",
		"NFL", "80", "47-31-2", "60.3%", "+8.1%", "+6.50", "+3.8%", "+1.10",
		"NBA", "40", "21-18-1", "53.8%", "-2.1%", "-0.90", "—",
	} {
		if !strings.Contains(res.stdout, want) {
			t.Errorf("stdout missing %q:\n%s", want, res.stdout)
		}
	}
}

func TestPerformanceBreakdownInvalid(t *testing.T) {
	res := runBB(t, nil, "performance", "--breakdown", "vibes")
	if res.code != api.ExitUsage {
		t.Fatalf("exit code = %d, want %d", res.code, api.ExitUsage)
	}
	if !strings.Contains(res.stderr, "invalid --breakdown") {
		t.Errorf("stderr = %q", res.stderr)
	}
}

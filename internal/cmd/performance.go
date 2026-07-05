package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/Bookie-Breaker/bookie-breaker-cli/internal/api"
	"github.com/Bookie-Breaker/bookie-breaker-cli/internal/client/bookieemulator"
	"github.com/Bookie-Breaker/bookie-breaker-cli/internal/ui"
)

// NewPerformanceCmd shows aggregate paper-trading performance, optionally
// broken down by a grouping dimension.
func NewPerformanceCmd(a *app) *cobra.Command {
	var breakdown string

	cmd := &cobra.Command{
		Use:   "performance",
		Short: "Show paper-trading performance",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if breakdown != "" {
				return runBreakdown(cmd, a, breakdown)
			}
			return runPerformance(cmd, a)
		},
	}

	cmd.Flags().StringVar(&breakdown, "breakdown", "", "group performance by: league, market_type, sportsbook, or month")
	return cmd
}

func runPerformance(cmd *cobra.Command, a *app) error {
	params := &bookieemulator.GetPerformanceApiV1EmulatorPerformanceGetParams{}
	if a.cfg.DefaultLeague != "" {
		league := bookieemulator.GetPerformanceApiV1EmulatorPerformanceGetParamsLeague(a.cfg.DefaultLeague)
		params.League = &league
	}

	resp, err := a.clients.Emulator.GetPerformanceApiV1EmulatorPerformanceGetWithResponse(cmd.Context(), params)
	if err != nil {
		return err
	}
	if resp.JSON200 == nil {
		return api.ErrorFromResponse(resp.StatusCode(), resp.Body)
	}

	perf := resp.JSON200.Data
	if a.jsonOutput() {
		return ui.PrintJSON(cmd.OutOrStdout(), perf)
	}

	roi := float64(perf.Roi)
	units := float64(perf.TotalProfitUnits)
	card := ui.KeyValueCard("Performance ("+string(perf.Period.Window)+")", [][2]string{
		{"TOTAL BETS", fmt.Sprintf("%d", perf.TotalBets)},
		{"RECORD", fmt.Sprintf("%d-%d-%d", perf.TotalWins, perf.TotalLosses, perf.TotalPushes)},
		{"WIN RATE", ui.Percent(float64(perf.WinRate))},
		{"ROI", ui.ColorBySign(roi, ui.EdgePercent(roi*100))},
		{"UNITS", ui.ColorBySign(units, ui.Units(units))},
		{"AVG EDGE", edgePtrLabel(perf.AvgEdgePercentage)},
		{"AVG CLV", clvLabel(perf.AvgClv)},
		{"BRIER", brierLabel(perf.BrierScore)},
	})
	ui.Println(cmd.OutOrStdout(), card)
	return nil
}

func runBreakdown(cmd *cobra.Command, a *app, groupBy string) error {
	switch groupBy {
	case "league", "market_type", "sportsbook", "month":
	default:
		return &api.UsageError{Err: fmt.Errorf("invalid --breakdown %q: must be league, market_type, sportsbook, or month", groupBy)}
	}

	g := bookieemulator.GetBreakdownApiV1EmulatorPerformanceBreakdownGetParamsGroupBy(groupBy)
	params := &bookieemulator.GetBreakdownApiV1EmulatorPerformanceBreakdownGetParams{GroupBy: &g}

	resp, err := a.clients.Emulator.GetBreakdownApiV1EmulatorPerformanceBreakdownGetWithResponse(cmd.Context(), params)
	if err != nil {
		return err
	}
	if resp.JSON200 == nil {
		return api.ErrorFromResponse(resp.StatusCode(), resp.Body)
	}

	data := resp.JSON200.Data
	if a.jsonOutput() {
		return ui.PrintJSON(cmd.OutOrStdout(), data)
	}

	rows := make([][]string, 0, len(data.Breakdowns))
	for _, b := range data.Breakdowns {
		roi := float64(b.Roi)
		units := float64(b.TotalProfitUnits)
		rows = append(rows, []string{
			b.Group,
			fmt.Sprintf("%d", b.TotalBets),
			fmt.Sprintf("%d-%d-%d", b.Wins, b.Losses, b.Pushes),
			ui.Percent(float64(b.WinRate)),
			ui.ColorBySign(roi, ui.EdgePercent(roi*100)),
			ui.ColorBySign(units, ui.Units(units)),
			edgePtrLabel(b.AvgEdgePercentage),
			clvLabel(b.AvgClv),
		})
	}
	headers := []string{"GROUP", "BETS", "RECORD", "WIN RATE", "ROI", "UNITS", "AVG EDGE", "AVG CLV"}
	ui.Println(cmd.OutOrStdout(), ui.Table(headers, rows))
	return nil
}

func edgePtrLabel(v *float32) string {
	if v == nil {
		return ui.Dash
	}
	return ui.EdgePercent(float64(*v))
}

func brierLabel(v *float32) string {
	if v == nil {
		return ui.Dash
	}
	return fmt.Sprintf("%.4f", *v)
}

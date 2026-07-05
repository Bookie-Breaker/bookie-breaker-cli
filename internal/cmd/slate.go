package cmd

import (
	"fmt"
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"
	"github.com/spf13/cobra"

	"github.com/Bookie-Breaker/bookie-breaker-cli/internal/api"
	"github.com/Bookie-Breaker/bookie-breaker-cli/internal/client/agentservice"
	"github.com/Bookie-Breaker/bookie-breaker-cli/internal/ui"
)

// NewSlateCmd shows a date's games with prediction summaries and edges.
func NewSlateCmd(a *app) *cobra.Command {
	var date string

	cmd := &cobra.Command{
		Use:   "slate",
		Short: "Show a date's games with predictions and edges",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			params := &agentservice.GetSlateApiV1AgentSlateGetParams{}
			if a.cfg.DefaultLeague != "" {
				params.League = &a.cfg.DefaultLeague
			}
			if date != "" {
				d, err := time.Parse("2006-01-02", date)
				if err != nil {
					return &api.UsageError{Err: fmt.Errorf("invalid --date %q: expected YYYY-MM-DD", date)}
				}
				params.Date = &openapi_types.Date{Time: d}
			}

			resp, err := a.clients.Agent.GetSlateApiV1AgentSlateGetWithResponse(cmd.Context(), params)
			if err != nil {
				return err
			}
			if resp.JSON200 == nil {
				return api.ErrorFromResponse(resp.StatusCode(), resp.Body)
			}

			slate := resp.JSON200.Data
			if a.jsonOutput() {
				return ui.PrintJSON(cmd.OutOrStdout(), slate)
			}

			rows := make([][]string, 0, len(slate.Games))
			for _, g := range slate.Games {
				rows = append(rows, []string{
					ui.Timestamp(g.ScheduledStart),
					fmt.Sprintf("%s @ %s", g.AwayTeam.Abbreviation, g.HomeTeam.Abbreviation),
					g.Status,
					slatePredictionLabel(g.Prediction),
					slateBestEdgeLabel(g.Edges),
				})
			}
			headers := []string{"TIME", "MATCHUP", "STATUS", "PREDICTION", "BEST EDGE"}
			ui.Println(cmd.OutOrStdout(), ui.Table(headers, rows))
			return nil
		},
	}

	cmd.Flags().StringVar(&date, "date", "", "slate date (YYYY-MM-DD, default today)")
	return cmd
}

func slatePredictionLabel(p *agentservice.SlatePrediction) string {
	if p == nil {
		return ui.Dash
	}
	return fmt.Sprintf("%s %s", p.Selection, ui.Percent(float64(p.PredictedProbability)))
}

func slateBestEdgeLabel(edges []agentservice.SlateEdge) string {
	if len(edges) == 0 {
		return ui.Dash
	}
	best := edges[0]
	for _, e := range edges[1:] {
		if e.EdgePercentage > best.EdgePercentage {
			best = e
		}
	}
	return fmt.Sprintf("%s %s", ui.Green.Render(ui.EdgePercent(float64(best.EdgePercentage))), best.MarketType)
}

package cmd

import (
	"fmt"
	"sort"
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"
	"github.com/spf13/cobra"

	"github.com/Bookie-Breaker/bookie-breaker-cli/internal/api"
	"github.com/Bookie-Breaker/bookie-breaker-cli/internal/client/agentservice"
	"github.com/Bookie-Breaker/bookie-breaker-cli/internal/ui"
)

// NewEdgesCmd lists currently detected +EV edges from the agent.
func NewEdgesCmd(a *app) *cobra.Command {
	var (
		market  string
		minEdge float32
		stale   bool
		date    string
		limit   int
	)

	cmd := &cobra.Command{
		Use:   "edges",
		Short: "List detected +EV edges",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			params := &agentservice.ListEdgesApiV1AgentEdgesGetParams{}
			if a.cfg.DefaultLeague != "" {
				params.League = &a.cfg.DefaultLeague
			}
			if market != "" {
				params.MarketType = &market
			}
			if cmd.Flags().Changed("min-edge") {
				params.MinEdge = &minEdge
			}
			if stale {
				params.IsStale = &stale
			}
			if date != "" {
				d, err := time.Parse("2006-01-02", date)
				if err != nil {
					return &api.UsageError{Err: fmt.Errorf("invalid --date %q: expected YYYY-MM-DD", date)}
				}
				params.Date = &openapi_types.Date{Time: d}
			}
			if cmd.Flags().Changed("limit") {
				params.Limit = &limit
			}

			resp, err := a.clients.Agent.ListEdgesApiV1AgentEdgesGetWithResponse(cmd.Context(), params)
			if err != nil {
				return err
			}
			if resp.JSON200 == nil {
				return api.ErrorFromResponse(resp.StatusCode(), resp.Body)
			}

			edges := resp.JSON200.Data
			sort.SliceStable(edges, func(i, j int) bool {
				return edges[i].EdgePercentage > edges[j].EdgePercentage
			})

			if a.jsonOutput() {
				return ui.PrintJSON(cmd.OutOrStdout(), edges)
			}

			rows := make([][]string, 0, len(edges))
			for _, e := range edges {
				rows = append(rows, []string{
					edgeGameLabel(e),
					e.MarketType,
					e.Selection,
					ui.Odds(e.OddsAmerican),
					ui.Percent(float64(e.PredictedProbability)),
					ui.Percent(float64(e.ImpliedProbability)),
					ui.Green.Render(ui.EdgePercent(float64(e.EdgePercentage))),
					e.SportsbookKey,
					ui.Timestamp(e.DetectedAt),
				})
			}
			headers := []string{"GAME", "MARKET", "SELECTION", "ODDS", "MODEL %", "IMPLIED %", "EDGE", "BOOK", "DETECTED"}
			ui.Println(cmd.OutOrStdout(), ui.Table(headers, rows))
			return nil
		},
	}

	cmd.Flags().StringVar(&market, "market", "", "filter by market type (SPREAD, TOTAL, MONEYLINE)")
	cmd.Flags().Float32Var(&minEdge, "min-edge", 0, "minimum edge percentage to include")
	cmd.Flags().BoolVar(&stale, "stale", false, "include stale edges")
	cmd.Flags().StringVar(&date, "date", "", "filter by game date (YYYY-MM-DD)")
	cmd.Flags().IntVar(&limit, "limit", 0, "max results")
	return cmd
}

// edgeGameLabel prefers "AWY @ HOM" abbreviations and falls back to a
// shortened game id.
func edgeGameLabel(e agentservice.EdgeListItem) string {
	if e.AwayTeam != nil && e.HomeTeam != nil {
		return fmt.Sprintf("%s @ %s", *e.AwayTeam, *e.HomeTeam)
	}
	return shortID(e.GameId)
}

// shortID trims a UUID to its first block for display.
func shortID(id string) string {
	if len(id) > 8 {
		return id[:8]
	}
	return id
}

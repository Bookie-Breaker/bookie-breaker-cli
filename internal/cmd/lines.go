package cmd

import (
	"fmt"

	"github.com/google/uuid"
	"github.com/spf13/cobra"

	"github.com/Bookie-Breaker/bookie-breaker-cli/internal/api"
	"github.com/Bookie-Breaker/bookie-breaker-cli/internal/client/linesservice"
	"github.com/Bookie-Breaker/bookie-breaker-cli/internal/ui"
)

// NewLinesCmd shows current lines (or movement history) for a game.
func NewLinesCmd(a *app) *cobra.Command {
	var (
		market    string
		book      string
		side      string
		selection string
		limit     int
		movement  bool
	)

	cmd := &cobra.Command{
		Use:   "lines <game_id>",
		Short: "Show current betting lines for a game",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			gameID, err := uuid.Parse(args[0])
			if err != nil {
				return &api.UsageError{Err: fmt.Errorf("invalid game id %q: expected a UUID", args[0])}
			}

			if movement {
				return runLineMovement(cmd, a, gameID, market, book, selection)
			}
			return runCurrentLines(cmd, a, gameID, market, book, side, limit)
		},
	}

	cmd.Flags().StringVar(&market, "market", "", "filter by market type (SPREAD, TOTAL, MONEYLINE); defaults to SPREAD with --movement")
	cmd.Flags().StringVar(&book, "book", "", "filter by sportsbook key (e.g. draftkings)")
	cmd.Flags().StringVar(&side, "side", "", "filter by selection side (home, away, draw, over, under)")
	cmd.Flags().StringVar(&selection, "selection", "", "restrict movement history to one selection")
	cmd.Flags().IntVar(&limit, "limit", 0, "max results")
	cmd.Flags().BoolVar(&movement, "movement", false, "show line movement history instead of current lines")
	return cmd
}

func runCurrentLines(cmd *cobra.Command, a *app, gameID uuid.UUID, market, book, side string, limit int) error {
	params := &linesservice.GetGameLinesParams{}
	if market != "" {
		params.MarketType = &market
	}
	if book != "" {
		params.Sportsbook = &book
	}
	if side != "" {
		params.Side = &side
	}
	if cmd.Flags().Changed("limit") {
		params.Limit = &limit
	}

	resp, err := a.clients.Lines.GetGameLinesWithResponse(cmd.Context(), gameID, params)
	if err != nil {
		return err
	}
	if resp.JSON200 == nil {
		return api.ErrorFromResponse(resp.StatusCode(), resp.Body)
	}

	lines := resp.JSON200.Data
	if a.jsonOutput() {
		return ui.PrintJSON(cmd.OutOrStdout(), lines)
	}

	rows := make([][]string, 0, len(lines))
	for _, l := range lines {
		rows = append(rows, []string{
			l.SportsbookKey,
			string(l.MarketType),
			l.Selection,
			ui.LineValue(l.LineValue),
			ui.Odds(l.OddsAmerican),
			ui.Percent(float64(l.ImpliedProbability)),
			ui.TimeShort(l.Timestamp),
		})
	}
	headers := []string{"BOOK", "MARKET", "SELECTION", "LINE", "ODDS", "IMPLIED %", "UPDATED"}
	ui.Println(cmd.OutOrStdout(), ui.Table(headers, rows))
	return nil
}

func runLineMovement(cmd *cobra.Command, a *app, gameID uuid.UUID, market, book, selection string) error {
	if market == "" {
		market = "SPREAD"
	}
	params := &linesservice.GetGameLineMovementParams{MarketType: &market}
	if book != "" {
		params.Sportsbook = &book
	}
	if selection != "" {
		params.Selection = &selection
	}

	resp, err := a.clients.Lines.GetGameLineMovementWithResponse(cmd.Context(), gameID, params)
	if err != nil {
		return err
	}
	if resp.JSON200 == nil {
		return api.ErrorFromResponse(resp.StatusCode(), resp.Body)
	}

	movements := resp.JSON200.Data
	if a.jsonOutput() {
		return ui.PrintJSON(cmd.OutOrStdout(), movements)
	}

	var rows [][]string
	for _, m := range movements {
		if m.LineSnapshots == nil {
			continue
		}
		for _, s := range *m.LineSnapshots {
			timeCell := ui.Dash
			if s.Timestamp != nil {
				timeCell = ui.TimeShort(*s.Timestamp)
			}
			if s.IsOpening != nil && *s.IsOpening {
				timeCell += " (open)"
			}
			odds := ui.Dash
			if s.OddsAmerican != nil {
				odds = ui.Odds(*s.OddsAmerican)
			}
			rows = append(rows, []string{
				timeCell,
				derefOr(m.SportsbookKey, ui.Dash),
				derefOr(m.Selection, ui.Dash),
				ui.LineValue(s.LineValue),
				odds,
			})
		}
	}
	headers := []string{"TIME", "BOOK", "SELECTION", "LINE", "ODDS"}
	ui.Println(cmd.OutOrStdout(), ui.Table(headers, rows))
	return nil
}

func derefOr(s *string, fallback string) string {
	if s == nil {
		return fallback
	}
	return *s
}

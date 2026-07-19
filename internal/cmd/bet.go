package cmd

import (
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/spf13/cobra"

	"github.com/Bookie-Breaker/bookie-breaker-cli/internal/api"
	"github.com/Bookie-Breaker/bookie-breaker-cli/internal/client/bookieemulator"
	"github.com/Bookie-Breaker/bookie-breaker-cli/internal/ui"
)

// NewBetCmd groups paper-bet subcommands.
func NewBetCmd(a *app) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "bet",
		Short: "Place and list paper bets",
	}
	cmd.AddCommand(newBetPlaceCmd(a), newBetListCmd(a))
	return cmd
}

func newBetPlaceCmd(a *app) *cobra.Command {
	var (
		game           string
		market         string
		selection      string
		side           string
		stake          float64
		prob           float64
		edge           float64
		book           string
		edgeID         string
		predictionID   string
		kelly          float64
		reason         string
		idempotencyKey string
	)

	cmd := &cobra.Command{
		Use:   "place",
		Short: "Place a paper bet",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			gameID, err := uuid.Parse(game)
			if err != nil {
				return &api.UsageError{Err: fmt.Errorf("invalid --game %q: expected a UUID", game)}
			}

			body := bookieemulator.PlaceBetRequest{
				GameId:               gameID,
				MarketType:           bookieemulator.PlaceBetRequestMarketType(strings.ToUpper(market)),
				Selection:            selection,
				Side:                 bookieemulator.PlaceBetRequestSide(strings.ToUpper(side)),
				Stake:                float32(stake),
				PredictedProbability: float32(prob),
				EdgePercentage:       float32(edge),
			}
			if book != "" {
				body.SportsbookKey = &book
			}
			if edgeID != "" {
				id, err := uuid.Parse(edgeID)
				if err != nil {
					return &api.UsageError{Err: fmt.Errorf("invalid --edge-id %q: expected a UUID", edgeID)}
				}
				body.EdgeId = &id
			}
			if predictionID != "" {
				id, err := uuid.Parse(predictionID)
				if err != nil {
					return &api.UsageError{Err: fmt.Errorf("invalid --prediction-id %q: expected a UUID", predictionID)}
				}
				body.PredictionId = &id
			}
			if cmd.Flags().Changed("kelly") {
				k := float32(kelly)
				body.KellyFraction = &k
			}
			if reason != "" {
				body.Reasoning = &reason
			}
			if idempotencyKey == "" {
				idempotencyKey = uuid.New().String()
			}
			params := &bookieemulator.PlaceBetApiV1EmulatorBetsPostParams{XIdempotencyKey: idempotencyKey}

			resp, err := a.clients.Emulator.PlaceBetApiV1EmulatorBetsPostWithResponse(cmd.Context(), params, body)
			if err != nil {
				return err
			}
			bet := resp.JSON201
			if bet == nil {
				return api.ErrorFromResponse(resp.StatusCode(), resp.Body)
			}

			if a.jsonOutput() {
				return ui.PrintJSON(cmd.OutOrStdout(), bet.Data)
			}

			b := bet.Data
			card := ui.KeyValueCard(ui.Green.Render("Bet placed"), [][2]string{
				{"ID", b.Id.String()},
				{"Game", b.GameExternalId},
				{"Selection", b.Selection},
				{"Side", ui.StringPtr(b.Side)},
				{"Market", b.MarketType},
				{"Line", ui.LineValue(b.LineValue)},
				{"Odds", ui.Odds(b.OddsAmerican)},
				{"Stake", ui.Stake(float64(b.Stake)) + "u"},
				{"Book", b.SportsbookKey},
				{"Result", ui.ColorResult(string(b.Result))},
			})
			ui.Println(cmd.OutOrStdout(), card)
			return nil
		},
	}

	flags := cmd.Flags()
	flags.StringVar(&game, "game", "", "game UUID (required)")
	flags.StringVar(&market, "market", "", "market type: SPREAD, TOTAL, MONEYLINE (required)")
	flags.StringVar(&selection, "selection", "", "selection (required)")
	flags.StringVar(&side, "side", "", "side: HOME, AWAY, DRAW, OVER, UNDER (required; DRAW only on MONEYLINE)")
	flags.Float64Var(&stake, "stake", 0, "stake in units (required)")
	flags.Float64Var(&prob, "prob", 0, "predicted probability, 0-1 (required)")
	flags.Float64Var(&edge, "edge", 0, "edge in percentage points (required)")
	flags.StringVar(&book, "book", "", "sportsbook key")
	flags.StringVar(&edgeID, "edge-id", "", "edge UUID")
	flags.StringVar(&predictionID, "prediction-id", "", "prediction UUID")
	flags.Float64Var(&kelly, "kelly", 0, "kelly fraction, 0-1")
	flags.StringVar(&reason, "reason", "", "reasoning note")
	flags.StringVar(&idempotencyKey, "idempotency-key", "", "idempotency key (default: fresh UUID)")

	for _, f := range []string{"game", "market", "selection", "side", "stake", "prob", "edge"} {
		_ = cmd.MarkFlagRequired(f)
	}
	return cmd
}

func newBetListCmd(a *app) *cobra.Command {
	var (
		status  string
		result  string
		market  string
		minEdge float32
		from    string
		to      string
		limit   int
	)

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List paper bets",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			params := &bookieemulator.ListBetsApiV1EmulatorBetsGetParams{}
			if a.cfg.DefaultLeague != "" {
				league := bookieemulator.ListBetsApiV1EmulatorBetsGetParamsLeague(a.cfg.DefaultLeague)
				params.League = &league
			}
			if cmd.Flags().Changed("status") {
				s := bookieemulator.ListBetsApiV1EmulatorBetsGetParamsStatus(status)
				params.Status = &s
			}
			if result != "" {
				r := bookieemulator.ListBetsApiV1EmulatorBetsGetParamsResult(strings.ToUpper(result))
				params.Result = &r
			}
			if market != "" {
				m := bookieemulator.ListBetsApiV1EmulatorBetsGetParamsMarketType(strings.ToUpper(market))
				params.MarketType = &m
			}
			if cmd.Flags().Changed("min-edge") {
				params.MinEdge = &minEdge
			}
			if from != "" {
				t, err := parseDateTime(from)
				if err != nil {
					return &api.UsageError{Err: fmt.Errorf("invalid --from %q: %w", from, err)}
				}
				params.DateFrom = &t
			}
			if to != "" {
				t, err := parseDateTime(to)
				if err != nil {
					return &api.UsageError{Err: fmt.Errorf("invalid --to %q: %w", to, err)}
				}
				params.DateTo = &t
			}
			if cmd.Flags().Changed("limit") {
				params.Limit = &limit
			}

			resp, err := a.clients.Emulator.ListBetsApiV1EmulatorBetsGetWithResponse(cmd.Context(), params)
			if err != nil {
				return err
			}
			if resp.JSON200 == nil {
				return api.ErrorFromResponse(resp.StatusCode(), resp.Body)
			}

			bets := resp.JSON200.Data
			if a.jsonOutput() {
				return ui.PrintJSON(cmd.OutOrStdout(), bets)
			}

			rows := make([][]string, 0, len(bets))
			for _, b := range bets {
				rows = append(rows, []string{
					ui.TimeShort(b.PlacedAt),
					b.GameExternalId,
					b.MarketType,
					b.Selection,
					ui.Odds(b.OddsAmerican),
					ui.Stake(float64(b.Stake)),
					ui.ColorResult(string(b.Result)),
					ui.UnitsPtr(b.ProfitLoss),
					clvLabel(b.Clv),
				})
			}
			headers := []string{"PLACED", "GAME", "MARKET", "SELECTION", "ODDS", "STAKE", "RESULT", "P&L", "CLV"}
			ui.Println(cmd.OutOrStdout(), ui.Table(headers, rows))
			return nil
		},
	}

	flags := cmd.Flags()
	flags.StringVar(&status, "status", "all", "bet status: open, graded, or all")
	flags.StringVar(&result, "result", "", "filter by result: PENDING, WIN, LOSS, PUSH, VOID")
	flags.StringVar(&market, "market", "", "filter by market type")
	flags.Float32Var(&minEdge, "min-edge", 0, "minimum edge percentage")
	flags.StringVar(&from, "from", "", "start date (YYYY-MM-DD or RFC 3339), inclusive")
	flags.StringVar(&to, "to", "", "end date (YYYY-MM-DD or RFC 3339), inclusive")
	flags.IntVar(&limit, "limit", 0, "max results")
	return cmd
}

func clvLabel(clv *float32) string {
	if clv == nil {
		return ui.Dash
	}
	return fmt.Sprintf("%+.2f", *clv)
}

// parseDateTime accepts a bare date or a full RFC 3339 timestamp.
func parseDateTime(s string) (time.Time, error) {
	if t, err := time.Parse("2006-01-02", s); err == nil {
		return t, nil
	}
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return time.Time{}, fmt.Errorf("expected YYYY-MM-DD or RFC 3339")
	}
	return t, nil
}

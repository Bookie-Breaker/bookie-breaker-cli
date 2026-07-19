package cmd

import (
	"bufio"
	"fmt"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"github.com/spf13/cobra"

	"github.com/Bookie-Breaker/bookie-breaker-cli/internal/api"
	"github.com/Bookie-Breaker/bookie-breaker-cli/internal/client/agentservice"
	"github.com/Bookie-Breaker/bookie-breaker-cli/internal/client/bookieemulator"
	"github.com/Bookie-Breaker/bookie-breaker-cli/internal/ui"
)

// Parlay leg limits pinned by the API contracts (ADR-028).
const (
	minParlayLegs = 2
	maxParlayLegs = 6
)

// Parlay v1 accepts team markets only; props are rejected server-side too.
var (
	parlayMarkets = map[string]bool{"MONEYLINE": true, "SPREAD": true, "TOTAL": true}
	parlaySides   = map[string]bool{"HOME": true, "AWAY": true, "DRAW": true, "OVER": true, "UNDER": true}
)

// NewParlayCmd groups parlay subcommands.
func NewParlayCmd(a *app) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "parlay",
		Short: "Evaluate, place, and inspect parlays",
	}
	cmd.AddCommand(newParlayEvaluateCmd(a), newParlayPlaceCmd(a), newParlayShowCmd(a))
	return cmd
}

// parseLegs converts repeated --leg values of the form
// game_ext:MARKET:SIDE[:line] into agent evaluation legs.
func parseLegs(specs []string) ([]agentservice.ParlayLegRequest, error) {
	if len(specs) < minParlayLegs || len(specs) > maxParlayLegs {
		return nil, fmt.Errorf("a parlay needs %d-%d --leg flags, got %d", minParlayLegs, maxParlayLegs, len(specs))
	}

	legs := make([]agentservice.ParlayLegRequest, 0, len(specs))
	for _, spec := range specs {
		parts := strings.Split(spec, ":")
		if len(parts) < 3 || len(parts) > 4 {
			return nil, fmt.Errorf("invalid --leg %q: expected game_ext:MARKET:SIDE[:line]", spec)
		}
		game := strings.TrimSpace(parts[0])
		if game == "" {
			return nil, fmt.Errorf("invalid --leg %q: empty game id", spec)
		}
		market := strings.ToUpper(strings.TrimSpace(parts[1]))
		if !parlayMarkets[market] {
			return nil, fmt.Errorf("invalid --leg %q: market must be MONEYLINE, SPREAD, or TOTAL", spec)
		}
		side := strings.ToUpper(strings.TrimSpace(parts[2]))
		if !parlaySides[side] {
			return nil, fmt.Errorf("invalid --leg %q: side must be HOME, AWAY, DRAW, OVER, or UNDER", spec)
		}

		leg := agentservice.ParlayLegRequest{
			GameExternalId: game,
			MarketType:     market,
			Side:           side,
		}
		if len(parts) == 4 {
			line, err := strconv.ParseFloat(strings.TrimSpace(parts[3]), 32)
			if err != nil {
				return nil, fmt.Errorf("invalid --leg %q: line %q is not a number", spec, parts[3])
			}
			f := float32(line)
			leg.LineValue = &f
		}
		legs = append(legs, leg)
	}
	return legs, nil
}

// evaluateParlay parses leg specs and calls the agent's evaluate endpoint.
func evaluateParlay(a *app, cmd *cobra.Command, legSpecs []string, odds int, oddsSet, persist bool) (*agentservice.ParlayEvaluationData, error) {
	legs, err := parseLegs(legSpecs)
	if err != nil {
		return nil, &api.UsageError{Err: err}
	}

	body := agentservice.ParlayEvaluateRequest{Legs: legs}
	if oddsSet {
		body.ParlayOddsAmerican = &odds
	}
	if persist {
		body.Persist = &persist
	}

	resp, err := a.clients.Agent.EvaluateParlayApiV1AgentParlaysEvaluatePostWithResponse(cmd.Context(), body)
	if err != nil {
		return nil, err
	}
	if resp.JSON200 == nil {
		return nil, api.ErrorFromResponse(resp.StatusCode(), resp.Body)
	}
	return &resp.JSON200.Data, nil
}

// evaluationLegsTable renders the per-leg breakdown of an evaluation.
func evaluationLegsTable(legs []agentservice.ParlayLegData) string {
	rows := make([][]string, 0, len(legs))
	for _, l := range legs {
		rows = append(rows, []string{
			l.GameExternalId,
			l.MarketType,
			l.Side,
			ui.LineValue(l.LineValue),
			ui.Odds(l.OddsAmerican),
			ui.Percent(float64(l.PredictedProbability)),
			ui.StringPtr(l.SimLegKey),
		})
	}
	headers := []string{"GAME", "MARKET", "SIDE", "LINE", "ODDS", "PROB", "SIM KEY"}
	return ui.Table(headers, rows)
}

// evaluationSummaryCard renders the joint-math summary of an evaluation.
func evaluationSummaryCard(d *agentservice.ParlayEvaluationData) string {
	meets := ui.Red.Render("NO")
	if d.MeetsThreshold {
		meets = ui.Green.Render("YES")
	}
	sameGame := "no"
	if d.IsSameGame {
		sameGame = "yes"
	}
	return ui.KeyValueCard("Parlay evaluation", [][2]string{
		{"League", d.League},
		{"Same game", sameGame},
		{"Joint prob", ui.Percent(float64(d.JointProbability))},
		{"Independent prob", ui.Percent(float64(d.IndependentProbability))},
		{"Correlation edge", ui.ColorBySign(float64(d.CorrelationEdge), ui.EdgePercent(float64(d.CorrelationEdge)))},
		{"Combined odds", fmt.Sprintf("%s (%.2f)", ui.Odds(d.CombinedOddsAmerican), d.CombinedOddsDecimal)},
		{"EV", ui.ColorBySign(float64(d.EvPct), ui.EdgePercent(float64(d.EvPct)))},
		{"Method", d.Method},
		{"Kelly", fmt.Sprintf("%.4f", d.KellyFraction)},
		{"Recommended stake", ui.Stake(float64(d.RecommendedStake)) + "u"},
		{"Meets threshold", meets},
	})
}

func newParlayEvaluateCmd(a *app) *cobra.Command {
	var (
		legSpecs []string
		odds     int
		persist  bool
	)

	cmd := &cobra.Command{
		Use:   "evaluate",
		Short: "Evaluate a parlay with correlation-aware math",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			data, err := evaluateParlay(a, cmd, legSpecs, odds, cmd.Flags().Changed("odds"), persist)
			if err != nil {
				return err
			}

			if a.jsonOutput() {
				return ui.PrintJSON(cmd.OutOrStdout(), data)
			}
			out := cmd.OutOrStdout()
			ui.Println(out, evaluationLegsTable(data.Legs))
			ui.Println(out, evaluationSummaryCard(data))
			return nil
		},
	}

	flags := cmd.Flags()
	flags.StringArrayVar(&legSpecs, "leg", nil,
		"parlay leg as game_ext:MARKET:SIDE[:line], repeat 2-6 times (required)")
	flags.IntVar(&odds, "odds", 0, "offered SGP price in American odds (default: product of leg decimals)")
	flags.BoolVar(&persist, "persist", false, "persist the evaluation even below the EV threshold")
	_ = cmd.MarkFlagRequired("leg")
	return cmd
}

func newParlayPlaceCmd(a *app) *cobra.Command {
	var (
		legSpecs []string
		stake    float64
		odds     int
		yes      bool
	)

	cmd := &cobra.Command{
		Use:   "place",
		Short: "Evaluate and place a paper parlay",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			data, err := evaluateParlay(a, cmd, legSpecs, odds, cmd.Flags().Changed("odds"), false)
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()

			if !yes {
				ui.Println(out, evaluationSummaryCard(data))
				ui.Printf(out, "Place this parlay for %su? [y/N] ", ui.Stake(stake))
				answer, _ := bufio.NewReader(cmd.InOrStdin()).ReadString('\n')
				answer = strings.ToLower(strings.TrimSpace(answer))
				if answer != "y" && answer != "yes" {
					ui.Println(out, "Aborted.")
					return nil
				}
			}

			legs := make([]bookieemulator.ParlayLegRequest, 0, len(data.Legs))
			for _, l := range data.Legs {
				gameID, err := uuid.Parse(l.GameId)
				if err != nil {
					return fmt.Errorf("agent returned invalid game id %q: %w", l.GameId, err)
				}
				leg := bookieemulator.ParlayLegRequest{
					GameId:     gameID,
					MarketType: bookieemulator.ParlayLegRequestMarketType(l.MarketType),
					Selection:  l.Selection,
					Side:       bookieemulator.ParlayLegRequestSide(l.Side),
					LineValue:  l.LineValue,
				}
				ext := l.GameExternalId
				leg.GameExternalId = &ext
				if l.SportsbookKey != "" {
					book := l.SportsbookKey
					leg.SportsbookKey = &book
				}
				legs = append(legs, leg)
			}
			kelly := data.KellyFraction
			body := bookieemulator.PlaceParlayRequest{
				Legs:                 legs,
				Stake:                float32(stake),
				PredictedProbability: data.JointProbability,
				EdgePercentage:       data.EvPct,
				KellyFraction:        &kelly,
			}
			params := &bookieemulator.PlaceParlayApiV1EmulatorParlaysPostParams{
				XIdempotencyKey: uuid.New().String(),
			}

			resp, err := a.clients.Emulator.PlaceParlayApiV1EmulatorParlaysPostWithResponse(cmd.Context(), params, body)
			if err != nil {
				return err
			}
			if resp.JSON201 == nil {
				return api.ErrorFromResponse(resp.StatusCode(), resp.Body)
			}
			parlay := resp.JSON201.Data

			if a.jsonOutput() {
				return ui.PrintJSON(out, parlay)
			}
			ui.Println(out, ui.KeyValueCard(ui.Green.Render("Parlay placed"), [][2]string{
				{"ID", parlay.Id.String()},
				{"Legs", strconv.Itoa(len(parlay.Legs))},
				{"Combined odds", ui.Odds(parlay.CombinedOddsAmerican)},
				{"Stake", ui.Stake(float64(parlay.Stake)) + "u"},
				{"Result", ui.ColorResult(string(parlay.Result))},
			}))
			ui.Println(out, parlayLegsTable(parlay.Legs))
			return nil
		},
	}

	flags := cmd.Flags()
	flags.StringArrayVar(&legSpecs, "leg", nil,
		"parlay leg as game_ext:MARKET:SIDE[:line], repeat 2-6 times (required)")
	flags.Float64Var(&stake, "stake", 0, "stake in units (required)")
	flags.IntVar(&odds, "odds", 0, "offered SGP price in American odds (default: product of leg decimals)")
	flags.BoolVar(&yes, "yes", false, "skip the confirmation prompt")
	_ = cmd.MarkFlagRequired("leg")
	_ = cmd.MarkFlagRequired("stake")
	return cmd
}

func newParlayShowCmd(a *app) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "show <bet_id>",
		Short: "Show a parlay with its legs and statuses",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			betID, err := uuid.Parse(args[0])
			if err != nil {
				return &api.UsageError{Err: fmt.Errorf("invalid bet_id %q: expected a UUID", args[0])}
			}

			resp, err := a.clients.Emulator.GetParlayApiV1EmulatorParlaysBetIdGetWithResponse(cmd.Context(), betID)
			if err != nil {
				return err
			}
			if resp.JSON200 == nil {
				return api.ErrorFromResponse(resp.StatusCode(), resp.Body)
			}
			parlay := resp.JSON200.Data

			if a.jsonOutput() {
				return ui.PrintJSON(cmd.OutOrStdout(), parlay)
			}

			pairs := [][2]string{
				{"ID", parlay.Id.String()},
				{"Legs", strconv.Itoa(len(parlay.Legs))},
				{"Combined odds", fmt.Sprintf("%s (%.2f)", ui.Odds(parlay.CombinedOddsAmerican), parlay.CombinedOddsDecimal)},
				{"Stake", ui.Stake(float64(parlay.Stake)) + "u"},
				{"Prob", ui.Percent(float64(parlay.PredictedProbability))},
				{"Edge", ui.EdgePercent(float64(parlay.EdgePercentage))},
				{"Placed", ui.TimeShort(parlay.PlacedAt)},
				{"Result", ui.ColorResult(string(parlay.Result))},
				{"P&L", ui.UnitsPtr(parlay.ProfitLoss)},
			}
			if parlay.GradedAt != nil {
				pairs = append(pairs, [2]string{"Graded", ui.TimeShort(*parlay.GradedAt)})
			}
			out := cmd.OutOrStdout()
			ui.Println(out, ui.KeyValueCard("Parlay", pairs))
			ui.Println(out, parlayLegsTable(parlay.Legs))
			return nil
		},
	}
	return cmd
}

// parlayLegsTable renders emulator parlay legs with their statuses.
func parlayLegsTable(legs []bookieemulator.ParlayLegData) string {
	rows := make([][]string, 0, len(legs))
	for _, l := range legs {
		rows = append(rows, []string{
			strconv.Itoa(l.LegIndex + 1),
			l.GameExternalId,
			l.MarketType,
			l.Selection,
			ui.StringPtr(l.Side),
			ui.LineValue(l.LineValue),
			ui.Odds(l.OddsAmerican),
			ui.ColorResult(string(l.LegStatus)),
		})
	}
	headers := []string{"#", "GAME", "MARKET", "SELECTION", "SIDE", "LINE", "ODDS", "STATUS"}
	return ui.Table(headers, rows)
}

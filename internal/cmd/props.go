package cmd

import (
	"sort"
	"strconv"
	"strings"
	"unicode"

	"github.com/spf13/cobra"

	"github.com/Bookie-Breaker/bookie-breaker-cli/internal/api"
	"github.com/Bookie-Breaker/bookie-breaker-cli/internal/client/agentservice"
	"github.com/Bookie-Breaker/bookie-breaker-cli/internal/client/linesservice"
	"github.com/Bookie-Breaker/bookie-breaker-cli/internal/ui"
)

// playerPropMarket is the market type both the agent and the lines
// service use for player prop markets (ADR-029).
const playerPropMarket = "PLAYER_PROP"

// statLabels humanizes the canonical prop stat keys (the raw odds-source
// market keys the lines-service normalizer ingests, ADR-029). Unknown
// keys fall back to a title-cased rendering of the raw key.
var statLabels = map[string]string{
	"player_goal_scorer_anytime":     "Anytime goalscorer",
	"player_shots":                   "Shots",
	"player_shots_on_target":         "Shots on target",
	"batter_hits":                    "Hits",
	"batter_total_bases":             "Total bases",
	"batter_home_runs":               "Home runs",
	"pitcher_strikeouts":             "Strikeouts",
	"player_points":                  "Points",
	"player_rebounds":                "Rebounds",
	"player_assists":                 "Assists",
	"player_threes":                  "Threes",
	"player_points_rebounds_assists": "Points + rebounds + assists",
	"player_pass_yds":                "Passing yards",
	"player_rush_yds":                "Rushing yards",
	"player_reception_yds":           "Receiving yards",
	"player_receptions":              "Receptions",
	"player_anytime_td":              "Anytime touchdown",
}

// NewPropsCmd lists current player prop edges from the agent, or raw
// prop lines from the lines service with --lines.
func NewPropsCmd(a *app) *cobra.Command {
	var lines bool

	cmd := &cobra.Command{
		Use:   "props",
		Short: "List player prop edges and lines",
		Long: "List currently detected PLAYER_PROP edges from the agent, highest edge\n" +
			"first. With --lines, show raw player prop lines from the lines service\n" +
			"instead, grouped by game then player.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if lines {
				return runPropLines(cmd, a)
			}
			return runPropEdges(cmd, a)
		},
	}

	cmd.Flags().BoolVar(&lines, "lines", false, "show raw prop lines from the lines service instead of edges")
	return cmd
}

// runPropEdges lists PLAYER_PROP edges, highest edge first. The market
// filter is passed as a query param and re-checked client-side so a
// backend that ignores unknown market_type values cannot leak game
// markets into the prop view.
func runPropEdges(cmd *cobra.Command, a *app) error {
	market := playerPropMarket
	params := &agentservice.ListEdgesApiV1AgentEdgesGetParams{MarketType: &market}
	if a.cfg.DefaultLeague != "" {
		params.League = &a.cfg.DefaultLeague
	}

	resp, err := a.clients.Agent.ListEdgesApiV1AgentEdgesGetWithResponse(cmd.Context(), params)
	if err != nil {
		return err
	}
	if resp.JSON200 == nil {
		return api.ErrorFromResponse(resp.StatusCode(), resp.Body)
	}

	edges := make([]agentservice.EdgeListItem, 0, len(resp.JSON200.Data))
	for _, e := range resp.JSON200.Data {
		if e.MarketType == playerPropMarket {
			edges = append(edges, e)
		}
	}
	sort.SliceStable(edges, func(i, j int) bool {
		return edges[i].EdgePercentage > edges[j].EdgePercentage
	})

	if a.jsonOutput() {
		return ui.PrintJSON(cmd.OutOrStdout(), edges)
	}

	if len(edges) == 0 {
		ui.Println(cmd.OutOrStdout(), ui.Dim("No player prop edges right now."))
		return nil
	}

	rows := make([][]string, 0, len(edges))
	for _, e := range edges {
		side, line := propSelectionSideLine(e.Selection)
		rows = append(rows, []string{
			playerLabel(e.PlayerExternalId),
			statLabel(e.StatType),
			side,
			line,
			ui.Odds(e.OddsAmerican),
			ui.Green.Render(ui.EdgePercent(float64(e.EdgePercentage))),
			e.SportsbookKey,
		})
	}
	headers := []string{"PLAYER", "STAT", "SIDE", "LINE", "ODDS", "EV%", "BOOK"}
	ui.Println(cmd.OutOrStdout(), ui.Table(headers, rows))
	return nil
}

// runPropLines shows raw PLAYER_PROP lines from the lines service,
// grouped by game then player.
func runPropLines(cmd *cobra.Command, a *app) error {
	market := playerPropMarket
	params := &linesservice.GetCurrentLinesParams{MarketType: &market}
	if a.cfg.DefaultLeague != "" {
		params.League = &a.cfg.DefaultLeague
	}

	resp, err := a.clients.Lines.GetCurrentLinesWithResponse(cmd.Context(), params)
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

	if len(lines) == 0 {
		ui.Println(cmd.OutOrStdout(), ui.Dim("No player prop lines right now."))
		return nil
	}

	sorted := make([]linesservice.LineSnapshot, len(lines))
	copy(sorted, lines)
	sort.SliceStable(sorted, func(i, j int) bool {
		if sorted[i].GameId != sorted[j].GameId {
			return sorted[i].GameId < sorted[j].GameId
		}
		if pi, pj := derefOr(sorted[i].PlayerId, ""), derefOr(sorted[j].PlayerId, ""); pi != pj {
			return pi < pj
		}
		if si, sj := derefOr(sorted[i].StatType, ""), derefOr(sorted[j].StatType, ""); si != sj {
			return si < sj
		}
		return sorted[i].Selection < sorted[j].Selection
	})

	rows := make([][]string, 0, len(sorted))
	for _, l := range sorted {
		side := ui.Dash
		if l.Side != nil {
			side = string(*l.Side)
		}
		rows = append(rows, []string{
			shortID(l.GameId),
			playerLabel(l.PlayerId),
			statLabel(l.StatType),
			side,
			ui.LineValue(l.LineValue),
			ui.Odds(l.OddsAmerican),
			l.SportsbookKey,
			ui.TimeShort(l.Timestamp),
		})
	}
	headers := []string{"GAME", "PLAYER", "STAT", "SIDE", "LINE", "ODDS", "BOOK", "UPDATED"}
	ui.Println(cmd.OutOrStdout(), ui.Table(headers, rows))
	return nil
}

// playerLabel renders a player external id slug ("erling-haaland",
// ADR-029) as a display name ("Erling Haaland").
func playerLabel(id *string) string {
	if id == nil || *id == "" {
		return ui.Dash
	}
	return titleWords(*id, "-")
}

// statLabel humanizes a canonical stat key, falling back to a
// title-cased rendering of the raw key for unmapped stats.
func statLabel(key *string) string {
	if key == nil || *key == "" {
		return ui.Dash
	}
	if label, ok := statLabels[*key]; ok {
		return label
	}
	return titleWords(*key, "_")
}

// titleWords splits s on sep and upper-cases the first rune of each part.
func titleWords(s, sep string) string {
	parts := strings.Split(s, sep)
	for i, p := range parts {
		if p == "" {
			continue
		}
		r := []rune(p)
		r[0] = unicode.ToUpper(r[0])
		parts[i] = string(r)
	}
	return strings.Join(parts, " ")
}

// propSelectionSideLine extracts the side (OVER/UNDER/YES/NO) and the
// numeric line from an edge's selection string, e.g. "OVER 2.5" or
// "Erling Haaland YES". Edge list items carry no structured side or
// line value, so this parses the selection the agent renders.
func propSelectionSideLine(selection string) (side, line string) {
	side, line = ui.Dash, ui.Dash
	tokens := strings.Fields(selection)
	for i, tok := range tokens {
		switch up := strings.ToUpper(tok); up {
		case "OVER", "UNDER", "YES", "NO":
			side = up
			if i+1 < len(tokens) {
				if _, err := strconv.ParseFloat(tokens[i+1], 64); err == nil {
					line = tokens[i+1]
				}
			}
			return side, line
		}
	}
	return side, line
}

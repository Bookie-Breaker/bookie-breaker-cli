package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"sort"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/Bookie-Breaker/bookie-breaker-cli/internal/api"
	"github.com/Bookie-Breaker/bookie-breaker-cli/internal/client/agentservice"
	"github.com/Bookie-Breaker/bookie-breaker-cli/internal/client/linesservice"
	"github.com/Bookie-Breaker/bookie-breaker-cli/internal/ui"
)

// minWatchInterval is the smallest allowed --watch poll interval; live
// lines carry short expiries but the services should not be hammered.
const minWatchInterval = 5

// clearScreen is the ANSI home-and-clear sequence used between watch
// re-renders. The CLI polls rather than consuming the live SSE stream
// (ADR-031): a first-cut clear-and-reprint loop keeps the CLI stateless.
const clearScreen = "\033[H\033[2J"

// liveEdge pairs a typed edge with its raw JSON so the is_live marker —
// not yet part of the generated agent client's EdgeListItem — survives
// filtering and JSON output.
type liveEdge struct {
	item agentservice.EdgeListItem
	raw  json.RawMessage
}

// NewLiveCmd shows in-game (live) lines grouped by game plus currently
// live edges from the agent.
func NewLiveCmd(a *app) *cobra.Command {
	var watch int

	cmd := &cobra.Command{
		Use:   "live",
		Short: "Show live in-game lines and live edges",
		Long: "Show current in-game betting lines (is_live only) grouped by game,\n" +
			"plus live edges detected by the agent. With --watch, re-polls and\n" +
			"re-renders until interrupted.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if !cmd.Flags().Changed("watch") {
				return runLiveOnce(cmd, a)
			}
			if watch < minWatchInterval {
				return &api.UsageError{Err: fmt.Errorf("invalid --watch %d: minimum interval is %d seconds", watch, minWatchInterval)}
			}
			return runLiveWatch(cmd, a, watch)
		},
	}

	cmd.Flags().IntVar(&watch, "watch", 0, fmt.Sprintf("re-poll every N seconds (min %d) until Ctrl-C", minWatchInterval))
	return cmd
}

// runLiveWatch clears the screen and re-renders every interval seconds
// until the context is canceled (Ctrl-C / SIGTERM).
func runLiveWatch(cmd *cobra.Command, a *app, interval int) error {
	ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	ticker := time.NewTicker(time.Duration(interval) * time.Second)
	defer ticker.Stop()

	for {
		ui.Printf(cmd.OutOrStdout(), clearScreen)
		if err := runLiveOnce(cmd, a); err != nil {
			if ctx.Err() != nil {
				return nil // interrupted mid-poll: clean exit
			}
			return err
		}
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
	}
}

func runLiveOnce(cmd *cobra.Command, a *app) error {
	lines, err := fetchLiveLines(cmd.Context(), a)
	if err != nil {
		return err
	}
	edges, err := fetchLiveEdges(cmd.Context(), a)
	if err != nil {
		return err
	}

	if a.jsonOutput() {
		rawEdges := make([]json.RawMessage, 0, len(edges))
		for _, e := range edges {
			rawEdges = append(rawEdges, e.raw)
		}
		return ui.PrintJSON(cmd.OutOrStdout(), map[string]any{
			"live_lines": lines,
			"live_edges": rawEdges,
		})
	}

	out := cmd.OutOrStdout()
	if len(lines) == 0 && len(edges) == 0 {
		ui.Println(out, ui.Dim("Nothing is live right now."))
		return nil
	}

	ui.Println(out, "LIVE LINES")
	if len(lines) == 0 {
		ui.Println(out, ui.Dim("No live lines right now."))
	} else {
		ui.Println(out, liveLinesTable(a, lines))
	}

	ui.Println(out, "")
	ui.Println(out, "LIVE EDGES")
	if len(edges) == 0 {
		ui.Println(out, ui.Dim("No live edges right now."))
	} else {
		ui.Println(out, liveEdgesTable(edges))
	}
	return nil
}

// fetchLiveLines returns current lines restricted to in-game (is_live)
// snapshots, honoring the --league filter.
func fetchLiveLines(ctx context.Context, a *app) ([]linesservice.LineSnapshot, error) {
	isLive := true
	params := &linesservice.GetCurrentLinesParams{IsLive: &isLive}
	if a.cfg.DefaultLeague != "" {
		params.League = &a.cfg.DefaultLeague
	}

	resp, err := a.clients.Lines.GetCurrentLinesWithResponse(ctx, params)
	if err != nil {
		return nil, err
	}
	if resp.JSON200 == nil {
		return nil, api.ErrorFromResponse(resp.StatusCode(), resp.Body)
	}
	return resp.JSON200.Data, nil
}

// fetchLiveEdges lists agent edges and keeps only those marked is_live.
// The agent spec is unchanged this wave, so the generated client has no
// is_live query param or struct field; the filter reads the raw JSON.
func fetchLiveEdges(ctx context.Context, a *app) ([]liveEdge, error) {
	params := &agentservice.ListEdgesApiV1AgentEdgesGetParams{}
	if a.cfg.DefaultLeague != "" {
		params.League = &a.cfg.DefaultLeague
	}

	resp, err := a.clients.Agent.ListEdgesApiV1AgentEdgesGetWithResponse(ctx, params)
	if err != nil {
		return nil, err
	}
	if resp.JSON200 == nil {
		return nil, api.ErrorFromResponse(resp.StatusCode(), resp.Body)
	}

	var envelope struct {
		Data []json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(resp.Body, &envelope); err != nil {
		return nil, fmt.Errorf("decoding edges response: %w", err)
	}

	var live []liveEdge
	for i, item := range resp.JSON200.Data {
		if i >= len(envelope.Data) {
			break
		}
		var flags struct {
			IsLive bool `json:"is_live"`
		}
		if err := json.Unmarshal(envelope.Data[i], &flags); err != nil || !flags.IsLive {
			continue
		}
		live = append(live, liveEdge{item: item, raw: envelope.Data[i]})
	}
	return live, nil
}

// liveLinesTable renders live lines grouped by game: rows are sorted by
// game id so each game's markets appear together. LineSnapshot carries
// no league field, so the LEAGUE column echoes the active --league
// filter when set.
func liveLinesTable(a *app, lines []linesservice.LineSnapshot) string {
	sorted := make([]linesservice.LineSnapshot, len(lines))
	copy(sorted, lines)
	sort.SliceStable(sorted, func(i, j int) bool {
		if sorted[i].GameId != sorted[j].GameId {
			return sorted[i].GameId < sorted[j].GameId
		}
		if sorted[i].MarketType != sorted[j].MarketType {
			return sorted[i].MarketType < sorted[j].MarketType
		}
		return sorted[i].Selection < sorted[j].Selection
	})

	league := ui.Dash
	if a.cfg.DefaultLeague != "" {
		league = a.cfg.DefaultLeague
	}

	rows := make([][]string, 0, len(sorted))
	for _, l := range sorted {
		side := ui.Dash
		if l.Side != nil {
			side = string(*l.Side)
		}
		rows = append(rows, []string{
			league,
			shortID(l.GameId),
			string(l.MarketType),
			l.Selection,
			side,
			ui.LineValue(l.LineValue),
			ui.Odds(l.OddsAmerican),
			l.SportsbookKey,
			ui.TimeShort(l.Timestamp),
		})
	}
	headers := []string{"LEAGUE", "GAME", "MARKET", "SELECTION", "SIDE", "LINE", "ODDS", "BOOK", "UPDATED"}
	return ui.Table(headers, rows)
}

// liveEdgesTable renders the live-edges section, highest edge first.
func liveEdgesTable(edges []liveEdge) string {
	sorted := make([]liveEdge, len(edges))
	copy(sorted, edges)
	sort.SliceStable(sorted, func(i, j int) bool {
		return sorted[i].item.EdgePercentage > sorted[j].item.EdgePercentage
	})

	rows := make([][]string, 0, len(sorted))
	for _, e := range sorted {
		rows = append(rows, []string{
			e.item.League,
			edgeGameLabel(e.item),
			e.item.MarketType,
			e.item.Selection,
			ui.Odds(e.item.OddsAmerican),
			ui.Percent(float64(e.item.PredictedProbability)),
			ui.Green.Render(ui.EdgePercent(float64(e.item.EdgePercentage))),
			e.item.SportsbookKey,
			ui.Timestamp(e.item.ExpiresAt),
		})
	}
	headers := []string{"LEAGUE", "GAME", "MARKET", "SELECTION", "ODDS", "MODEL %", "EDGE", "BOOK", "EXPIRES"}
	return ui.Table(headers, rows)
}

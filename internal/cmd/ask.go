package cmd

import (
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/spf13/cobra"

	"github.com/Bookie-Breaker/bookie-breaker-cli/internal/api"
	"github.com/Bookie-Breaker/bookie-breaker-cli/internal/client/agentservice"
	"github.com/Bookie-Breaker/bookie-breaker-cli/internal/ui"
)

// NewAskCmd sends a question to the agent's LLM analyst.
func NewAskCmd(a *app) *cobra.Command {
	var (
		gameID       string
		edgeID       string
		analysisType string
	)

	cmd := &cobra.Command{
		Use:   "ask <question...>",
		Short: "Ask the LLM analyst about an edge, a game, or performance",
		Long: "Ask the BookieBreaker LLM analyst a question. Scope it with --edge for an\n" +
			"edge breakdown or --game for a game preview; unscoped questions get a\n" +
			"performance review. LLM generation can take a minute or two.",
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			question := strings.Join(args, " ")

			resolvedType, err := resolveAnalysisType(analysisType, gameID, edgeID)
			if err != nil {
				return err
			}

			body := agentservice.AnalysisRequest{
				AnalysisType: resolvedType,
				Question:     &question,
			}
			if gameID != "" {
				if _, err := uuid.Parse(gameID); err != nil {
					return &api.UsageError{Err: fmt.Errorf("--game must be a UUID: %w", err)}
				}
				body.GameId = &gameID
			}
			if edgeID != "" {
				if _, err := uuid.Parse(edgeID); err != nil {
					return &api.UsageError{Err: fmt.Errorf("--edge must be a UUID: %w", err)}
				}
				body.EdgeId = &edgeID
			}

			// The dedicated analysis client allows for slow LLM generation.
			resp, err := a.clients.AgentAnalysis.CreateAnalysisApiV1AgentAnalysisPostWithResponse(cmd.Context(), body)
			if err != nil {
				return err
			}
			envelope := resp.JSON201
			if envelope == nil {
				envelope = resp.JSON200 // cached analysis reuse
			}
			if envelope == nil {
				return api.ErrorFromResponse(resp.StatusCode(), resp.Body)
			}

			data := envelope.Data
			if a.jsonOutput() {
				return ui.PrintJSON(cmd.OutOrStdout(), data)
			}

			out := cmd.OutOrStdout()
			ui.Println(out, ui.Markdown("# "+data.Title+"\n\n"+data.Content))
			ui.Printf(out, "%s\n", ui.Dim(fmt.Sprintf("%s — analysis %s", data.ModelUsed, data.Id)))
			return nil
		},
	}

	cmd.Flags().StringVar(&gameID, "game", "", "game UUID to scope the question to a game preview")
	cmd.Flags().StringVar(&edgeID, "edge", "", "edge UUID to scope the question to an edge breakdown")
	cmd.Flags().StringVar(&analysisType, "type", "",
		"analysis type override: GAME_PREVIEW, EDGE_BREAKDOWN, or PERFORMANCE_REVIEW")
	return cmd
}

// resolveAnalysisType applies the explicit --type or infers it from the
// provided scope: edge -> breakdown, game -> preview, none -> performance.
func resolveAnalysisType(override, gameID, edgeID string) (agentservice.AnalysisRequestAnalysisType, error) {
	if override != "" {
		switch strings.ToUpper(override) {
		case string(agentservice.GAMEPREVIEW):
			return agentservice.GAMEPREVIEW, nil
		case string(agentservice.EDGEBREAKDOWN):
			return agentservice.EDGEBREAKDOWN, nil
		case string(agentservice.PERFORMANCEREVIEW):
			return agentservice.PERFORMANCEREVIEW, nil
		default:
			return "", &api.UsageError{Err: fmt.Errorf(
				"invalid --type %q: must be GAME_PREVIEW, EDGE_BREAKDOWN, or PERFORMANCE_REVIEW", override)}
		}
	}
	if edgeID != "" {
		return agentservice.EDGEBREAKDOWN, nil
	}
	if gameID != "" {
		return agentservice.GAMEPREVIEW, nil
	}
	return agentservice.PERFORMANCEREVIEW, nil
}

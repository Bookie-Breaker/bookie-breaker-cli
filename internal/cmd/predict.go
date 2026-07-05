package cmd

import (
	"fmt"
	"sort"

	"github.com/spf13/cobra"

	"github.com/Bookie-Breaker/bookie-breaker-cli/internal/api"
	"github.com/Bookie-Breaker/bookie-breaker-cli/internal/client/predictionengine"
	"github.com/Bookie-Breaker/bookie-breaker-cli/internal/ui"
)

// maxFeatureImportanceRows caps the feature-importance listing.
const maxFeatureImportanceRows = 8

// NewPredictCmd shows the latest calibrated predictions for a game.
func NewPredictCmd(a *app) *cobra.Command {
	var market string

	cmd := &cobra.Command{
		Use:   "predict <game_id>",
		Short: "Show the latest predictions for a game",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			params := &predictionengine.LatestPredictionsApiV1PredictGamesGameIdLatestGetParams{}
			if market != "" {
				params.MarketType = &market
			}

			resp, err := a.clients.Prediction.LatestPredictionsApiV1PredictGamesGameIdLatestGetWithResponse(
				cmd.Context(), args[0], params)
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

			out := cmd.OutOrStdout()
			rows := make([][]string, 0, len(data.Predictions))
			for _, p := range data.Predictions {
				rows = append(rows, []string{
					p.MarketType,
					p.Selection,
					ui.Percent(float64(p.PredictedProbability)),
					confidenceInterval(p),
					ui.PercentPtr(p.SimulationProbability),
					fmt.Sprintf("%+.3f", p.AdjustmentMagnitude),
					shortID(p.ModelVersionId),
				})
			}
			headers := []string{"MARKET", "SELECTION", "PROB", "90% CI", "SIM PROB", "ADJ", "MODEL"}
			ui.Println(out, ui.Table(headers, rows))

			for _, p := range data.Predictions {
				printFeatureImportance(out, p)
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&market, "market", "", "filter by market type (comma-separated)")
	return cmd
}

func confidenceInterval(p predictionengine.PredictionItem) string {
	if p.ConfidenceLower == nil || p.ConfidenceUpper == nil {
		return ui.Dash
	}
	return fmt.Sprintf("%s–%s", ui.Percent(float64(*p.ConfidenceLower)), ui.Percent(float64(*p.ConfidenceUpper)))
}

// printFeatureImportance prints the top features by importance as
// indented "name value" lines.
func printFeatureImportance(out interface{ Write([]byte) (int, error) }, p predictionengine.PredictionItem) {
	if len(p.FeatureImportance) == 0 {
		return
	}

	type feature struct {
		name  string
		value float32
	}
	features := make([]feature, 0, len(p.FeatureImportance))
	for name, value := range p.FeatureImportance {
		features = append(features, feature{name, value})
	}
	sort.Slice(features, func(i, j int) bool {
		if features[i].value != features[j].value {
			return features[i].value > features[j].value
		}
		return features[i].name < features[j].name
	})
	if len(features) > maxFeatureImportanceRows {
		features = features[:maxFeatureImportanceRows]
	}

	width := 0
	for _, f := range features {
		if len(f.name) > width {
			width = len(f.name)
		}
	}
	ui.Printf(out, "\n%s feature importance:\n", p.MarketType)
	for _, f := range features {
		ui.Printf(out, "  %-*s  %.2f\n", width, f.name, f.value)
	}
}

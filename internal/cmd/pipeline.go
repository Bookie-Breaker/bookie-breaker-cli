package cmd

import (
	"fmt"
	"sort"
	"strings"

	"github.com/google/uuid"
	"github.com/spf13/cobra"

	"github.com/Bookie-Breaker/bookie-breaker-cli/internal/api"
	"github.com/Bookie-Breaker/bookie-breaker-cli/internal/client/agentservice"
	"github.com/Bookie-Breaker/bookie-breaker-cli/internal/ui"
)

// NewPipelineCmd groups pipeline subcommands.
func NewPipelineCmd(a *app) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "pipeline",
		Short: "Trigger and inspect prediction pipeline runs",
	}
	cmd.AddCommand(newPipelineRunCmd(a), newPipelineStatusCmd(a), newPipelineScheduleCmd(a))
	return cmd
}

func newPipelineScheduleCmd(a *app) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "schedule",
		Short: "Manage cron schedules for automated pipeline runs",
	}
	cmd.AddCommand(newScheduleListCmd(a), newScheduleSetCmd(a))
	return cmd
}

func newScheduleListCmd(a *app) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List configured pipeline schedules",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			resp, err := a.clients.Agent.ListSchedulesApiV1AgentScheduleGetWithResponse(cmd.Context())
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

			rows := make([][]string, 0, len(data.Schedules))
			for _, s := range data.Schedules {
				rows = append(rows, scheduleRow(s))
			}
			headers := []string{"LEAGUE", "CRON", "TZ", "ENABLED", "AUTO-BET", "MIN EDGE", "LAST RUN", "NEXT RUN"}
			ui.Println(cmd.OutOrStdout(), ui.Table(headers, rows))
			return nil
		},
	}
}

func newScheduleSetCmd(a *app) *cobra.Command {
	var (
		league      string
		cron        string
		timezone    string
		description string
		enabled     bool
		autoBet     bool
		minEdge     float32
	)

	cmd := &cobra.Command{
		Use:   "set",
		Short: "Create or update a league's pipeline schedule",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if league == "" {
				league = a.cfg.DefaultLeague
			}
			if league == "" {
				return &api.UsageError{Err: fmt.Errorf("--league is required (or set a default league)")}
			}
			if cron == "" {
				return &api.UsageError{Err: fmt.Errorf("--cron is required")}
			}

			body := agentservice.ScheduleRequest{
				League:           league,
				CronExpression:   cron,
				Enabled:          &enabled,
				AutoBet:          &autoBet,
				MinEdgeThreshold: &minEdge,
			}
			if timezone != "" {
				body.Timezone = &timezone
			}
			if description != "" {
				body.Description = &description
			}

			resp, err := a.clients.Agent.UpsertScheduleApiV1AgentSchedulePostWithResponse(cmd.Context(), body)
			if err != nil {
				return err
			}
			envelope := resp.JSON201
			created := envelope != nil
			if envelope == nil {
				envelope = resp.JSON200
			}
			if envelope == nil {
				return api.ErrorFromResponse(resp.StatusCode(), resp.Body)
			}

			data := envelope.Data
			if a.jsonOutput() {
				return ui.PrintJSON(cmd.OutOrStdout(), data)
			}

			out := cmd.OutOrStdout()
			verb := "updated"
			if created {
				verb = "created"
			}
			ui.Printf(out, "Schedule %s for %s\n", ui.Green.Render(verb), data.League)
			headers := []string{"LEAGUE", "CRON", "TZ", "ENABLED", "AUTO-BET", "MIN EDGE", "LAST RUN", "NEXT RUN"}
			ui.Println(out, ui.Table(headers, [][]string{scheduleRow(data)}))
			return nil
		},
	}

	cmd.Flags().StringVar(&league, "league", "", "league for this schedule (defaults to the configured league)")
	cmd.Flags().StringVar(&cron, "cron", "", "cron expression, e.g. \"0 10,14,18 * * *\"")
	cmd.Flags().StringVar(&timezone, "timezone", "", "IANA timezone for the cron expression (default UTC)")
	cmd.Flags().StringVar(&description, "description", "", "human-readable schedule description")
	cmd.Flags().BoolVar(&enabled, "enabled", true, "whether the schedule is active")
	cmd.Flags().BoolVar(&autoBet, "auto-bet", true, "auto-place paper bets on scheduled runs")
	cmd.Flags().Float32Var(&minEdge, "min-edge", 3.0, "minimum edge percentage for auto-betting")
	return cmd
}

func scheduleRow(s agentservice.ScheduleData) []string {
	return []string{
		s.League,
		s.CronExpression,
		s.Timezone,
		fmt.Sprintf("%t", s.Enabled),
		fmt.Sprintf("%t", s.AutoBet),
		fmt.Sprintf("%.1f%%", s.MinEdgeThreshold),
		derefOr(s.LastRunAt, ui.Dash),
		derefOr(s.NextRunAt, ui.Dash),
	}
}

func newPipelineRunCmd(a *app) *cobra.Command {
	var (
		games        string
		forceRefresh bool
		autoBet      bool
	)

	cmd := &cobra.Command{
		Use:   "run",
		Short: "Trigger a pipeline run",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			body := agentservice.PipelineRunRequest{
				AutoBet:      &autoBet,
				ForceRefresh: &forceRefresh,
			}
			if a.cfg.DefaultLeague != "" {
				body.League = &a.cfg.DefaultLeague
			}
			if games != "" {
				ids := strings.Split(games, ",")
				for i := range ids {
					ids[i] = strings.TrimSpace(ids[i])
				}
				body.GameIds = &ids
			}

			resp, err := a.clients.Agent.RunPipelineApiV1AgentPipelineRunPostWithResponse(cmd.Context(), body)
			if err != nil {
				return err
			}
			if resp.JSON202 == nil {
				return api.ErrorFromResponse(resp.StatusCode(), resp.Body)
			}

			data := resp.JSON202.Data
			if a.jsonOutput() {
				return ui.PrintJSON(cmd.OutOrStdout(), data)
			}

			out := cmd.OutOrStdout()
			ui.Printf(out, "Pipeline run %s\n", ui.Green.Render(data.PipelineRunId))
			ui.Printf(out, "Status: %s  League: %s  Games queued: %d\n",
				data.Status, derefOr(data.League, "all"), data.GamesQueued)

			rows := make([][]string, 0, len(data.Steps))
			for _, step := range sortedKeys(data.Steps) {
				rows = append(rows, []string{step, data.Steps[step]})
			}
			ui.Println(out, ui.Table([]string{"STEP", "STATE"}, rows))
			return nil
		},
	}

	cmd.Flags().StringVar(&games, "games", "", "comma-separated game UUIDs to process")
	cmd.Flags().BoolVar(&forceRefresh, "force-refresh", false, "bypass caches and refetch inputs")
	cmd.Flags().BoolVar(&autoBet, "auto-bet", true, "place paper bets on detected edges")
	return cmd
}

func newPipelineStatusCmd(a *app) *cobra.Command {
	return &cobra.Command{
		Use:   "status <run_id>",
		Short: "Show the status of a pipeline run",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			runID, err := uuid.Parse(args[0])
			if err != nil {
				return &api.UsageError{Err: fmt.Errorf("invalid run id %q: expected a UUID", args[0])}
			}

			resp, err := a.clients.Agent.GetPipelineRunApiV1AgentPipelineRunsPipelineRunIdGetWithResponse(cmd.Context(), runID)
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
			ui.Printf(out, "Pipeline run %s\n", data.PipelineRunId)
			ui.Printf(out, "Status: %s  Trigger: %s  League: %s\n",
				data.Status, data.Trigger, derefOr(data.League, "all"))
			ui.Printf(out, "Games processed: %d  Edges found: %d  Bets placed: %d\n",
				data.GamesProcessed, data.EdgesFound, data.BetsPlaced)
			if data.Error != nil && *data.Error != "" {
				ui.Println(out, ui.Red.Render("Error: "+*data.Error))
			}

			rows := make([][]string, 0, len(data.Steps))
			for _, step := range sortedKeys(data.Steps) {
				rows = append(rows, []string{step, stepState(data.Steps[step])})
			}
			ui.Println(out, ui.Table([]string{"STEP", "STATE"}, rows))
			return nil
		},
	}
}

// sortedKeys returns the map's keys in sorted order for stable output.
func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// stepState renders a pipeline step value, which may be a plain string or
// an object carrying a "status" field.
func stepState(v any) string {
	switch s := v.(type) {
	case string:
		return s
	case map[string]any:
		if status, ok := s["status"].(string); ok {
			return status
		}
	}
	return fmt.Sprint(v)
}

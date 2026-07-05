// Package cmd defines the bb command tree. Tables and JSON go to stdout;
// errors and diagnostics go to stderr. Exit codes: 0 success, 1 API error,
// 2 usage error, 3 connection/timeout failure.
package cmd

import (
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"

	"github.com/Bookie-Breaker/bookie-breaker-cli/internal/api"
	"github.com/Bookie-Breaker/bookie-breaker-cli/internal/config"
)

// app carries the resolved configuration and API clients from the root
// command's PersistentPreRunE to the subcommands.
type app struct {
	cfg        *config.Config
	clients    *api.Clients
	configPath string
}

// jsonOutput reports whether --format json is in effect.
func (a *app) jsonOutput() bool {
	return a.cfg.Format == config.FormatJSON
}

// NewRootCmd builds the bb command tree.
func NewRootCmd() *cobra.Command {
	a := &app{}

	root := &cobra.Command{
		Use:           "bb",
		Short:         "BookieBreaker terminal interface",
		Long:          "bb is the BookieBreaker terminal interface for viewing edges, predictions,\nlines, paper bets, and performance.",
		SilenceUsage:  true,
		SilenceErrors: true,
		PersistentPreRunE: func(cmd *cobra.Command, _ []string) error {
			cfg, err := config.Load(a.configPath)
			if err != nil {
				return err
			}

			flags := cmd.Flags()
			if flags.Changed("format") {
				cfg.Format, _ = flags.GetString("format")
			}
			if flags.Changed("league") {
				cfg.DefaultLeague, _ = flags.GetString("league")
			}
			if cfg.Format != config.FormatTable && cfg.Format != config.FormatJSON {
				return &api.UsageError{Err: fmt.Errorf("invalid format %q: must be table or json", cfg.Format)}
			}

			clients, err := api.NewClients(cfg)
			if err != nil {
				return err
			}
			a.cfg = cfg
			a.clients = clients
			return nil
		},
	}

	pf := root.PersistentFlags()
	pf.String("format", "", "output format: table or json")
	pf.String("league", "", "filter by league (e.g. NFL, NBA)")
	pf.StringVar(&a.configPath, "config", "", "path to config file (default: os user config dir /bookiebreaker/config.yaml)")

	root.SetFlagErrorFunc(func(_ *cobra.Command, err error) error {
		return &api.UsageError{Err: err}
	})

	root.AddCommand(
		NewEdgesCmd(a),
		NewSlateCmd(a),
		NewPredictCmd(a),
		NewLinesCmd(a),
		NewBetCmd(a),
		NewPerformanceCmd(a),
		NewPipelineCmd(a),
		NewHealthCmd(a),
	)
	return root
}

// Execute runs the CLI and returns the process exit code.
func Execute() int {
	return run(NewRootCmd(), os.Stderr)
}

// run executes root, rendering any error to errW and mapping it to an
// exit code. Split out from Execute for testability.
func run(root *cobra.Command, errW io.Writer) int {
	if err := root.Execute(); err != nil {
		api.RenderError(errW, err)
		return api.ExitCode(err)
	}
	return api.ExitOK
}

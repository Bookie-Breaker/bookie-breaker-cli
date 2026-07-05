# bookie-breaker-cli

`bb` is the BookieBreaker terminal interface for viewing edges, predictions, lines, paper bets,
and performance, built with Cobra and Lip Gloss.

## Install

```bash
task bootstrap   # install tools and git hooks
task build       # builds bin/bb
./bin/bb --help
```

Or without go-task:

```bash
go build -o bin/bb ./cmd/cli
```

## Configuration

Settings resolve with the precedence **defaults < config file < environment variables < flags**.

The config file lives at `os.UserConfigDir()/bookiebreaker/config.yaml`
(`~/.config/bookiebreaker/config.yaml` on Linux) or wherever `--config` points. A missing file is
fine; a malformed one is an error.

```yaml
# ~/.config/bookiebreaker/config.yaml
agent_url: http://localhost:8006
lines_service_url: http://localhost:8001
statistics_service_url: http://localhost:8002
bookie_emulator_url: http://localhost:8005
prediction_engine_url: http://localhost:8004
default_league: NFL
format: table # or json
timeout: 10s
```

### Environment variables

`AGENT_URL`, `LINES_SERVICE_URL`, `STATISTICS_SERVICE_URL`, `BOOKIE_EMULATOR_URL`, and
`PREDICTION_ENGINE_URL` override the config file. See `.env.example`.

### Global flags

| Flag       | Description                           |
| ---------- | ------------------------------------- |
| `--format` | `table` (default) or `json`           |
| `--league` | league filter applied where supported |
| `--config` | explicit config file path             |

## Commands

### bb edges

List detected +EV edges, sorted by edge percentage descending.

```bash
bb edges --league NFL --market SPREAD --min-edge 3 --limit 20
bb edges --date 2026-07-04 --stale
```

### bb slate

Show a date's games with prediction summaries and best edges.

```bash
bb slate --league NFL --date 2026-07-04
```

### bb predict

Show the latest calibrated predictions for a game, with top feature importance.

```bash
bb predict 22222222-2222-2222-2222-222222222222 --market SPREAD,MONEYLINE
```

### bb lines

Show current lines for a game, or its movement history with `--movement`.

```bash
bb lines <game_id> --market SPREAD --book draftkings --side home
bb lines <game_id> --movement --market SPREAD --selection "PHI -2.5"
```

### bb bet

Place and list paper bets against the bookie-emulator.

```bash
bb bet place --game <game_id> --market SPREAD --selection "PHI -2.5" \
  --side HOME --stake 1.5 --prob 0.56 --edge 3.6 --book draftkings
bb bet list --status graded --result WIN --min-edge 2 --from 2026-07-01
```

`bet place` sends an `X-Idempotency-Key` header (a fresh UUID unless `--idempotency-key` is
given), so retries never double-place a bet.

### bb performance

Show aggregate paper-trading performance, or a grouped table with `--breakdown`.

```bash
bb performance --league NFL
bb performance --breakdown market_type
```

### bb pipeline

Trigger a pipeline run, poll its status, and manage cron schedules for automated runs.

```bash
bb pipeline run --league NFL --force-refresh --auto-bet=false
bb pipeline status <run_id>
bb pipeline schedule list
bb pipeline schedule set --league NBA --cron "0 10,14,18 * * *" --timezone America/New_York --min-edge 4
```

### bb ask

Ask the agent's LLM analyst a question. Scope with `--edge` (edge breakdown) or `--game`
(game preview); unscoped questions get a performance review. Rendered as markdown; LLM
generation can take a minute or two (`analysis_timeout` in config, default 120s).

```bash
bb ask "Why do you like the over in Lakers vs Celtics?" --game <game_id>
bb ask --edge <edge_id> "What would make this edge wrong?"
bb ask "What should we change about our betting?"
```

### bb health

Probe every configured service's health endpoint concurrently. Exits 1 when any service is
unreachable or unhealthy.

```bash
bb health
```

## Scripting with --format json

`--format json` prints the unwrapped response `data` (no envelope) pretty-printed to stdout;
tables and diagnostics never mix into it, so output pipes cleanly:

```bash
bb edges --format json | jq '.[0].edge_percentage'
bb bet list --format json --status open | jq 'length'
```

Exit codes: `0` success, `1` API error, `2` usage error, `3` connection/timeout.

## Shell completion

Cobra's built-in completion command is enabled:

```bash
bb completion bash > /etc/bash_completion.d/bb   # bash
bb completion zsh > "${fpath[1]}/_bb"            # zsh
bb completion fish > ~/.config/fish/completions/bb.fish
```

## Regenerating API clients

Clients under `internal/client/` are generated from the OpenAPI specs in
`bookie-breaker-docs/api-contracts/openapi/`:

```bash
task generate:clients
```

## Architecture Decisions

- [Tech Stack Selection (ADR-010)](https://github.com/Bookie-Breaker/bookie-breaker-docs/blob/main/decisions/010-tech-stack-selection.md)

# bookie-breaker-cli

## Service Purpose

Go terminal interface for viewing edges, predictions, paper bets, and performance. Built with Cobra for commands and Charm (Lip Gloss, Glamour) for styled terminal output.

## Language & Conventions

- **Language:** Go 1.22
- **Framework:** Cobra (CLI), Charm ecosystem (TUI)
- **Project layout:** `cmd/cli/main.go` entry point, `internal/` for commands and API clients
- **Naming:** `snake_case.go` files, `camelCase` variables, `PascalCase` exports
- **Testing:** `*_test.go` co-located

## Key Files

- `cmd/cli/main.go` — CLI entry point
- `internal/cmd/` — Cobra command definitions
- `internal/client/` — HTTP clients for backend services
- `.config/mise.toml` — Tool versions
- `.config/lefthook.yml` — Git hooks

## Service-Specific Commands

```bash
task dev          # go run ./cmd/cli
task lint         # golangci-lint
task test         # go test -race ./...
task build        # Build to bin/bb
```

## Dependencies

- **agent** (port 8006) — Primary backend for edges, pipeline, analysis
- **lines-service** (port 8001) — Direct line lookups
- **statistics-service** (port 8002) — Direct stat lookups
- **bookie-emulator** (port 8005) — Bet placement and performance

## Environment Variables

See `.env.example`. Key: `AGENT_URL`, `LINES_SERVICE_URL`, `STATISTICS_SERVICE_URL`, `BOOKIE_EMULATOR_URL`.

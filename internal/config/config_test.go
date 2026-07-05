package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// clearEnv blanks every config-relevant environment variable so tests are
// hermetic regardless of the developer's shell.
func clearEnv(t *testing.T) {
	t.Helper()
	for _, key := range []string{
		"AGENT_URL", "LINES_SERVICE_URL", "STATISTICS_SERVICE_URL",
		"BOOKIE_EMULATOR_URL", "PREDICTION_ENGINE_URL",
	} {
		t.Setenv(key, "")
	}
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
}

func writeConfig(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLoadDefaults(t *testing.T) {
	clearEnv(t)

	cfg, err := Load("")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	want := Config{
		AgentURL:             "http://localhost:8006",
		LinesServiceURL:      "http://localhost:8001",
		StatisticsServiceURL: "http://localhost:8002",
		BookieEmulatorURL:    "http://localhost:8005",
		PredictionEngineURL:  "http://localhost:8004",
		Format:               FormatTable,
		Timeout:              10 * time.Second,
		AnalysisTimeout:      120 * time.Second,
	}
	if *cfg != want {
		t.Errorf("Load() = %+v, want %+v", *cfg, want)
	}
}

func TestLoadFileOverridesDefaults(t *testing.T) {
	clearEnv(t)
	path := writeConfig(t, `
agent_url: http://agent.internal:9000
default_league: NBA
format: json
timeout: 30s
`)

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.AgentURL != "http://agent.internal:9000" {
		t.Errorf("AgentURL = %q", cfg.AgentURL)
	}
	if cfg.DefaultLeague != "NBA" {
		t.Errorf("DefaultLeague = %q", cfg.DefaultLeague)
	}
	if cfg.Format != FormatJSON {
		t.Errorf("Format = %q", cfg.Format)
	}
	if cfg.Timeout != 30*time.Second {
		t.Errorf("Timeout = %v", cfg.Timeout)
	}
	// Untouched keys keep their defaults.
	if cfg.LinesServiceURL != DefaultLinesServiceURL {
		t.Errorf("LinesServiceURL = %q, want default", cfg.LinesServiceURL)
	}
}

func TestLoadDefaultLocation(t *testing.T) {
	clearEnv(t)
	configHome := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", configHome)

	dir := filepath.Join(configHome, "bookiebreaker")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	content := []byte("agent_url: http://from-default-location:1\n")
	if err := os.WriteFile(filepath.Join(dir, "config.yaml"), content, 0o600); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load("")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.AgentURL != "http://from-default-location:1" {
		t.Errorf("AgentURL = %q, want value from default config location", cfg.AgentURL)
	}
}

func TestLoadEnvOverridesFile(t *testing.T) {
	clearEnv(t)
	path := writeConfig(t, "agent_url: http://from-file:1\nlines_service_url: http://lines-from-file:1\n")
	t.Setenv("AGENT_URL", "http://from-env:2")
	t.Setenv("PREDICTION_ENGINE_URL", "http://prediction-from-env:2")

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.AgentURL != "http://from-env:2" {
		t.Errorf("AgentURL = %q, want env to beat file", cfg.AgentURL)
	}
	if cfg.LinesServiceURL != "http://lines-from-file:1" {
		t.Errorf("LinesServiceURL = %q, want file value", cfg.LinesServiceURL)
	}
	if cfg.PredictionEngineURL != "http://prediction-from-env:2" {
		t.Errorf("PredictionEngineURL = %q, want env to beat default", cfg.PredictionEngineURL)
	}
}

func TestLoadMissingFileOK(t *testing.T) {
	clearEnv(t)

	cfg, err := Load(filepath.Join(t.TempDir(), "nope.yaml"))
	if err != nil {
		t.Fatalf("Load with missing file: %v", err)
	}
	if cfg.AgentURL != DefaultAgentURL {
		t.Errorf("AgentURL = %q, want default", cfg.AgentURL)
	}
}

func TestLoadMalformedFile(t *testing.T) {
	clearEnv(t)
	path := writeConfig(t, "agent_url: [unclosed\n")

	if _, err := Load(path); err == nil {
		t.Fatal("Load with malformed YAML: expected error, got nil")
	}
}

func TestLoadBadTimeout(t *testing.T) {
	clearEnv(t)
	path := writeConfig(t, "timeout: not-a-duration\n")

	if _, err := Load(path); err == nil {
		t.Fatal("Load with bad timeout: expected error, got nil")
	}
}

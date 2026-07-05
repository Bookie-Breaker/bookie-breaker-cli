// Package config loads bb CLI configuration with the precedence
// defaults < YAML config file < environment variables < command-line flags.
// Flags are applied by the root command after Load returns.
package config

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"gopkg.in/yaml.v3"
)

// Output formats supported by the CLI.
const (
	FormatTable = "table"
	FormatJSON  = "json"
)

// Default service URLs and settings.
const (
	DefaultAgentURL             = "http://localhost:8006"
	DefaultLinesServiceURL      = "http://localhost:8001"
	DefaultStatisticsServiceURL = "http://localhost:8002"
	DefaultBookieEmulatorURL    = "http://localhost:8005"
	DefaultPredictionEngineURL  = "http://localhost:8004"
	DefaultTimeout              = 10 * time.Second
	// DefaultAnalysisTimeout bounds bb ask: LLM generation is slow.
	DefaultAnalysisTimeout = 120 * time.Second
)

// Config holds every setting the CLI needs to talk to the backend services.
type Config struct {
	AgentURL             string
	LinesServiceURL      string
	StatisticsServiceURL string
	BookieEmulatorURL    string
	PredictionEngineURL  string
	DefaultLeague        string
	Format               string
	Timeout              time.Duration
	AnalysisTimeout      time.Duration
}

// fileConfig mirrors the YAML config file. Pointer fields distinguish
// "absent" from "explicitly empty" so file values only override defaults
// when present.
type fileConfig struct {
	AgentURL             *string `yaml:"agent_url"`
	LinesServiceURL      *string `yaml:"lines_service_url"`
	StatisticsServiceURL *string `yaml:"statistics_service_url"`
	BookieEmulatorURL    *string `yaml:"bookie_emulator_url"`
	PredictionEngineURL  *string `yaml:"prediction_engine_url"`
	DefaultLeague        *string `yaml:"default_league"`
	Format               *string `yaml:"format"`
	Timeout              *string `yaml:"timeout"`
	AnalysisTimeout      *string `yaml:"analysis_timeout"`
}

// Default returns the built-in configuration.
func Default() *Config {
	return &Config{
		AgentURL:             DefaultAgentURL,
		LinesServiceURL:      DefaultLinesServiceURL,
		StatisticsServiceURL: DefaultStatisticsServiceURL,
		BookieEmulatorURL:    DefaultBookieEmulatorURL,
		PredictionEngineURL:  DefaultPredictionEngineURL,
		Format:               FormatTable,
		Timeout:              DefaultTimeout,
		AnalysisTimeout:      DefaultAnalysisTimeout,
	}
}

// Load builds the configuration from defaults, the YAML config file, and
// environment variables, in that order. path overrides the default config
// file location (os.UserConfigDir()/bookiebreaker/config.yaml). A missing
// file is fine; a malformed one is an error.
func Load(path string) (*Config, error) {
	cfg := Default()

	if path == "" {
		if dir, err := os.UserConfigDir(); err == nil {
			path = filepath.Join(dir, "bookiebreaker", "config.yaml")
		}
	}
	if path != "" {
		if err := applyFile(cfg, path); err != nil {
			return nil, err
		}
	}
	applyEnv(cfg)
	return cfg, nil
}

func applyFile(cfg *Config, path string) error {
	raw, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("reading config file %s: %w", path, err)
	}

	var fc fileConfig
	if err := yaml.Unmarshal(raw, &fc); err != nil {
		return fmt.Errorf("parsing config file %s: %w", path, err)
	}

	setString(&cfg.AgentURL, fc.AgentURL)
	setString(&cfg.LinesServiceURL, fc.LinesServiceURL)
	setString(&cfg.StatisticsServiceURL, fc.StatisticsServiceURL)
	setString(&cfg.BookieEmulatorURL, fc.BookieEmulatorURL)
	setString(&cfg.PredictionEngineURL, fc.PredictionEngineURL)
	setString(&cfg.DefaultLeague, fc.DefaultLeague)
	setString(&cfg.Format, fc.Format)

	if fc.Timeout != nil {
		d, err := time.ParseDuration(*fc.Timeout)
		if err != nil {
			return fmt.Errorf("parsing timeout in config file %s: %w", path, err)
		}
		cfg.Timeout = d
	}
	if fc.AnalysisTimeout != nil {
		d, err := time.ParseDuration(*fc.AnalysisTimeout)
		if err != nil {
			return fmt.Errorf("parsing analysis_timeout in config file %s: %w", path, err)
		}
		cfg.AnalysisTimeout = d
	}
	return nil
}

func applyEnv(cfg *Config) {
	setEnv(&cfg.AgentURL, "AGENT_URL")
	setEnv(&cfg.LinesServiceURL, "LINES_SERVICE_URL")
	setEnv(&cfg.StatisticsServiceURL, "STATISTICS_SERVICE_URL")
	setEnv(&cfg.BookieEmulatorURL, "BOOKIE_EMULATOR_URL")
	setEnv(&cfg.PredictionEngineURL, "PREDICTION_ENGINE_URL")
}

func setString(dst *string, src *string) {
	if src != nil {
		*dst = *src
	}
}

func setEnv(dst *string, key string) {
	if v := os.Getenv(key); v != "" {
		*dst = v
	}
}

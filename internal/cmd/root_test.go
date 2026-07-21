package cmd

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Bookie-Breaker/bookie-breaker-cli/internal/api"
)

func TestRootHelp(t *testing.T) {
	res := runBB(t, nil, "--help")
	if res.code != api.ExitOK {
		t.Fatalf("exit code = %d, stderr: %s", res.code, res.stderr)
	}
	for _, want := range []string{"edges", "slate", "predict", "lines", "bet", "performance", "pipeline", "health", "completion"} {
		if !strings.Contains(res.stdout, want) {
			t.Errorf("help missing %q", want)
		}
	}
}

// TestExecuteMapsErrorsToExitCode drives the real Execute entry point
// with an unknown command, which fails before any config or network work.
// os.Args and os.Stderr are swapped so the run is hermetic.
func TestExecuteMapsErrorsToExitCode(t *testing.T) {
	origArgs, origStderr := os.Args, os.Stderr
	t.Cleanup(func() {
		os.Args, os.Stderr = origArgs, origStderr
	})

	stderrFile, err := os.CreateTemp(t.TempDir(), "stderr")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = stderrFile.Close() }()

	os.Args = []string{"bb", "definitely-not-a-command"}
	os.Stderr = stderrFile

	if code := Execute(); code != api.ExitUsage {
		t.Errorf("Execute() = %d, want %d", code, api.ExitUsage)
	}

	rendered, err := os.ReadFile(stderrFile.Name())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(rendered), "unknown command") {
		t.Errorf("stderr = %q, want unknown command error", rendered)
	}
}

func TestUnknownCommandExitsUsage(t *testing.T) {
	res := runBB(t, nil, "bogus")
	if res.code != api.ExitUsage {
		t.Fatalf("exit code = %d, want %d", res.code, api.ExitUsage)
	}
	if !strings.Contains(res.stderr, "unknown command") {
		t.Errorf("stderr = %q", res.stderr)
	}
}

func TestUnknownFlagExitsUsage(t *testing.T) {
	res := runBB(t, nil, "edges", "--nope")
	if res.code != api.ExitUsage {
		t.Fatalf("exit code = %d, want %d", res.code, api.ExitUsage)
	}
}

func TestInvalidFormatExitsUsage(t *testing.T) {
	res := runBB(t, nil, "edges", "--format", "xml")
	if res.code != api.ExitUsage {
		t.Fatalf("exit code = %d, want %d", res.code, api.ExitUsage)
	}
	if !strings.Contains(res.stderr, "invalid format") {
		t.Errorf("stderr = %q", res.stderr)
	}
}

func TestConnectionErrorExitsThree(t *testing.T) {
	res := runBB(t, map[string]string{"AGENT_URL": "http://127.0.0.1:1"}, "edges")
	if res.code != api.ExitConnection {
		t.Fatalf("exit code = %d, want %d; stderr: %s", res.code, api.ExitConnection, res.stderr)
	}
	if res.stderr == "" {
		t.Error("expected connection error on stderr")
	}
}

// TestFlagOverridesFileConfig exercises the full precedence chain: the
// config file sets format=table and a league, the flag flips the format,
// and the env var points the agent URL at the fixture server.
func TestFlagOverridesFileConfig(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("league"); got != "NBA" {
			t.Errorf("league query = %q, want flag value NBA", got)
		}
		writePagedEnvelope(t, w, http.StatusOK, edgesFixture())
	}))
	defer srv.Close()

	cfgPath := filepath.Join(t.TempDir(), "config.yaml")
	content := "format: table\ndefault_league: NFL\nagent_url: http://127.0.0.1:1\n"
	if err := os.WriteFile(cfgPath, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	res := runBB(t, map[string]string{"AGENT_URL": srv.URL},
		"edges", "--config", cfgPath, "--format", "json", "--league", "NBA")
	if res.code != api.ExitOK {
		t.Fatalf("exit code = %d, stderr: %s", res.code, res.stderr)
	}
	if !strings.HasPrefix(strings.TrimSpace(res.stdout), "[") {
		t.Errorf("expected JSON output when --format json overrides file, got:\n%s", res.stdout)
	}
}

func TestFileConfigUsedWithoutFlags(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("league"); got != "NFL" {
			t.Errorf("league query = %q, want file value NFL", got)
		}
		writePagedEnvelope(t, w, http.StatusOK, edgesFixture())
	}))
	defer srv.Close()

	cfgPath := filepath.Join(t.TempDir(), "config.yaml")
	content := "format: json\ndefault_league: NFL\nagent_url: " + srv.URL + "\n"
	if err := os.WriteFile(cfgPath, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	res := runBB(t, nil, "edges", "--config", cfgPath)
	if res.code != api.ExitOK {
		t.Fatalf("exit code = %d, stderr: %s", res.code, res.stderr)
	}
	if !strings.HasPrefix(strings.TrimSpace(res.stdout), "[") {
		t.Errorf("expected JSON output from file format=json, got:\n%s", res.stdout)
	}
}

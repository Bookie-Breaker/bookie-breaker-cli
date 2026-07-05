package ui

import (
	"os"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

func TestMain(m *testing.M) {
	// Force plain output so assertions are byte-exact regardless of the
	// terminal running the tests.
	lipgloss.SetColorProfile(termenv.Ascii)
	os.Exit(m.Run())
}

func TestOdds(t *testing.T) {
	cases := map[int]string{150: "+150", -110: "-110", 100: "+100"}
	for in, want := range cases {
		if got := Odds(in); got != want {
			t.Errorf("Odds(%d) = %q, want %q", in, got, want)
		}
	}
}

func TestPercent(t *testing.T) {
	if got := Percent(0.524); got != "52.4%" {
		t.Errorf("Percent(0.524) = %q, want 52.4%%", got)
	}
	if got := Percent(1); got != "100.0%" {
		t.Errorf("Percent(1) = %q, want 100.0%%", got)
	}
	if got := PercentPtr(nil); got != Dash {
		t.Errorf("PercentPtr(nil) = %q, want dash", got)
	}
	v := float32(0.5)
	if got := PercentPtr(&v); got != "50.0%" {
		t.Errorf("PercentPtr(0.5) = %q, want 50.0%%", got)
	}
}

func TestEdgePercent(t *testing.T) {
	if got := EdgePercent(4.2); got != "+4.2%" {
		t.Errorf("EdgePercent(4.2) = %q, want +4.2%%", got)
	}
	if got := EdgePercent(-2.5); got != "-2.5%" {
		t.Errorf("EdgePercent(-2.5) = %q, want -2.5%%", got)
	}
}

func TestUnits(t *testing.T) {
	if got := Units(1.5); got != "+1.50" {
		t.Errorf("Units(1.5) = %q, want +1.50", got)
	}
	if got := Units(-0.5); got != "-0.50" {
		t.Errorf("Units(-0.5) = %q, want -0.50", got)
	}
	if got := UnitsPtr(nil); got != Dash {
		t.Errorf("UnitsPtr(nil) = %q, want dash", got)
	}
	if got := Stake(2); got != "2.00" {
		t.Errorf("Stake(2) = %q, want 2.00", got)
	}
}

func TestLineValue(t *testing.T) {
	if got := LineValue(nil); got != Dash {
		t.Errorf("LineValue(nil) = %q, want dash", got)
	}
	v := float32(-3.5)
	if got := LineValue(&v); got != "-3.5" {
		t.Errorf("LineValue(-3.5) = %q, want -3.5", got)
	}
}

func TestTimestamp(t *testing.T) {
	shortFormat := regexp.MustCompile(`^[A-Z][a-z]{2} \d{2} \d{2}:\d{2}$`)

	for _, in := range []string{
		"2026-07-04T18:00:00Z",
		"2026-07-04T18:00:00.123456Z",
		"2026-07-04T18:00:00",
	} {
		if got := Timestamp(in); !shortFormat.MatchString(got) {
			t.Errorf("Timestamp(%q) = %q, want short local format", in, got)
		}
	}
	if got := Timestamp("not-a-time"); got != "not-a-time" {
		t.Errorf("Timestamp(garbage) = %q, want passthrough", got)
	}
	if got := TimeShort(time.Date(2026, 7, 4, 18, 0, 0, 0, time.UTC)); !shortFormat.MatchString(got) {
		t.Errorf("TimeShort = %q, want short local format", got)
	}
}

func TestStylesPlainUnderAscii(t *testing.T) {
	if got := Green.Render("edge"); got != "edge" {
		t.Errorf("Green.Render under ascii = %q, want plain text", got)
	}
	if got := ColorResult("WIN"); got != "WIN" {
		t.Errorf("ColorResult(WIN) under ascii = %q", got)
	}
	if got := ColorBySign(-1, "-1.00"); got != "-1.00" {
		t.Errorf("ColorBySign under ascii = %q", got)
	}
}

func TestTable(t *testing.T) {
	out := Table(
		[]string{"BOOK", "ODDS"},
		[][]string{{"draftkings", "-110"}, {"pinnacle", "+102"}},
	)

	for _, want := range []string{"BOOK", "ODDS", "draftkings", "-110", "pinnacle", "+102"} {
		if !strings.Contains(out, want) {
			t.Errorf("Table output missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "\x1b[") {
		t.Errorf("Table output contains ANSI escapes under ascii profile:\n%s", out)
	}
}

func TestKeyValueCard(t *testing.T) {
	out := KeyValueCard("Bet placed", [][2]string{{"ID", "abc"}, {"Stake", "1.00u"}})
	for _, want := range []string{"Bet placed", "ID", "abc", "Stake", "1.00u"} {
		if !strings.Contains(out, want) {
			t.Errorf("KeyValueCard output missing %q:\n%s", want, out)
		}
	}
}

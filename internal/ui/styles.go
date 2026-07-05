// Package ui renders tables, styled blocks, and formatted values for the
// bb CLI. Styling degrades to plain text on non-TTY output and when
// NO_COLOR is set (handled by lipgloss/termenv).
package ui

import "github.com/charmbracelet/lipgloss"

// Shared color styles. ANSI palette colors keep output readable on both
// light and dark terminals.
var (
	Green  = lipgloss.NewStyle().Foreground(lipgloss.Color("2"))
	Red    = lipgloss.NewStyle().Foreground(lipgloss.Color("1"))
	Yellow = lipgloss.NewStyle().Foreground(lipgloss.Color("3"))

	// CardBorder frames confirmation cards and stat blocks.
	CardBorder = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			Padding(0, 1)
)

// ColorResult colors a bet result: WIN green, LOSS red, PENDING yellow.
func ColorResult(result string) string {
	switch result {
	case "WIN":
		return Green.Render(result)
	case "LOSS":
		return Red.Render(result)
	case "PENDING":
		return Yellow.Render(result)
	default:
		return result
	}
}

// ColorBySign colors s green when v is positive and red when negative.
func ColorBySign(v float64, s string) string {
	switch {
	case v > 0:
		return Green.Render(s)
	case v < 0:
		return Red.Render(s)
	default:
		return s
	}
}

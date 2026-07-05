package ui

import (
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/lipgloss/table"
)

// Table renders headers and rows as a bordered terminal table.
func Table(headers []string, rows [][]string) string {
	t := table.New().
		Border(lipgloss.NormalBorder()).
		StyleFunc(func(int, int) lipgloss.Style {
			return lipgloss.NewStyle().Padding(0, 1)
		}).
		Headers(headers...).
		Rows(rows...)
	return t.String()
}

// KeyValueCard renders label/value pairs inside a rounded border, with
// labels padded to a common width.
func KeyValueCard(title string, pairs [][2]string) string {
	width := 0
	for _, p := range pairs {
		if len(p[0]) > width {
			width = len(p[0])
		}
	}

	content := title
	for _, p := range pairs {
		content += "\n" + pad(p[0], width+2) + p[1]
	}
	return CardBorder.Render(content)
}

func pad(s string, width int) string {
	for len(s) < width {
		s += " "
	}
	return s
}

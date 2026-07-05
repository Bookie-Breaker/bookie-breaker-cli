package ui

import "github.com/charmbracelet/glamour"

// markdownWrap is the word-wrap width for rendered markdown.
const markdownWrap = 100

// Markdown renders GitHub-flavored markdown for the terminal with Glamour,
// falling back to the raw text when rendering fails (e.g. no TTY quirks).
func Markdown(content string) string {
	renderer, err := glamour.NewTermRenderer(
		glamour.WithAutoStyle(),
		glamour.WithWordWrap(markdownWrap),
	)
	if err != nil {
		return content
	}
	rendered, err := renderer.Render(content)
	if err != nil {
		return content
	}
	return rendered
}

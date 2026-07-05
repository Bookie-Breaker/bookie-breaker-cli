package ui

import (
	"encoding/json"
	"io"
)

// PrintJSON pretty-prints v to w with two-space indentation.
func PrintJSON(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

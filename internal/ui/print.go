package ui

import (
	"fmt"
	"io"
)

// Println writes a line to w, best-effort: terminal writes have no useful
// error recovery in a CLI.
func Println(w io.Writer, a ...any) {
	_, _ = fmt.Fprintln(w, a...)
}

// Printf writes formatted output to w, best-effort.
func Printf(w io.Writer, format string, a ...any) {
	_, _ = fmt.Fprintf(w, format, a...)
}

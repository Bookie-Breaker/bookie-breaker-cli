// Package main is the bb CLI entry point.
package main

import (
	"os"

	"github.com/Bookie-Breaker/bookie-breaker-cli/internal/cmd"
)

func main() {
	os.Exit(cmd.Execute())
}

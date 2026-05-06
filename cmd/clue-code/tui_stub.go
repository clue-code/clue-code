//go:build !tui

package main

import (
	"fmt"
	"os"
)

func runTUI(args []string) int {
	fmt.Fprintln(os.Stderr, "clue-code was built without TUI support; rebuild with -tags=tui")
	return 2
}

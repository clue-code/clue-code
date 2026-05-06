//go:build tui

package main

import (
	"github.com/clue-code/clue-code/internal/tui"
)

func runTUI(args []string) int {
	return tui.RunReadOnlyState(args)
}

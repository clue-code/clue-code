//go:build windows

package aider

import (
	"os"
)

var (
	sigTermSignal os.Signal = os.Interrupt
	sigKillSignal os.Signal = os.Kill
)

func sendSignal(p *os.Process, sig os.Signal) error {
	if p == nil {
		return nil
	}
	return p.Signal(sig)
}

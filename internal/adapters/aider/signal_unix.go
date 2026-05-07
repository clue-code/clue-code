//go:build !windows

package aider

import (
	"os"
	"syscall"
)

var (
	sigTermSignal = syscall.SIGTERM
	sigKillSignal = syscall.SIGKILL
)

func sendSignal(p *os.Process, sig os.Signal) error {
	if p == nil {
		return nil
	}
	return p.Signal(sig)
}

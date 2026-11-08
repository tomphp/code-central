//go:build e2e && unix

// How the e2e tests name and stop the binaries on Unix.

package e2e

import (
	"os"
	"os/exec"
)

// exeSuffix ends the name of a program.
const exeSuffix = ""

// startInOwnGroup readies cmd for interrupt. On Unix a signal goes to
// the process alone, so cmd stays in the test's group, and a Ctrl-C to
// the test reaches it too.
func startInOwnGroup(*exec.Cmd) {}

// interrupt sends p the stop launchd, systemd and a terminal's Ctrl-C
// send: os.Interrupt.
func interrupt(p *os.Process) error { return p.Signal(os.Interrupt) }

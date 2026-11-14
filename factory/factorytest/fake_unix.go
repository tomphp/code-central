//go:build unix

// How the fakes are named on PATH and take stop signals on Unix.

package factorytest

import (
	"os"
	"syscall"
)

// exeSuffix ends the name of a program on PATH.
const exeSuffix = ""

// waitSignals are the signals a Wait step notes.
var waitSignals = []os.Signal{syscall.SIGINT, syscall.SIGTERM}

// signalName is sig as Signals lists it, and whether it ends the run:
// SIGINT is noted and ignored, SIGTERM ends it.
func signalName(sig os.Signal) (name string, ends bool) {
	if sig == syscall.SIGTERM {
		return "TERM", true
	}
	return "INT", false
}

// inUse says err is a file held open by another process, which only
// Windows refuses to read.
func inUse(error) bool { return false }

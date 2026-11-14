// How the fakes are named on PATH and take stop signals on Windows.

package factorytest

import (
	"errors"
	"os"

	"golang.org/x/sys/windows"
)

// exeSuffix ends the name of a program on PATH.
const exeSuffix = ".exe"

// waitSignals are the signals a Wait step notes. Go delivers the
// Ctrl-Break the Factory sends a turn's process group as os.Interrupt.
var waitSignals = []os.Signal{os.Interrupt}

// signalName is sig as Signals lists it, and whether it ends the run:
// Ctrl-Break is noted and ignored, so the Factory has to kill the run.
func signalName(os.Signal) (name string, ends bool) { return "BREAK", false }

// inUse says err is a file held open by another process: Defender or
// the Search indexer opens a new file to scan it, and while it does
// Windows refuses to read it.
func inUse(err error) bool { return errors.Is(err, windows.ERROR_SHARING_VIOLATION) }

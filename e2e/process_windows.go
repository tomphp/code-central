//go:build e2e

// How the e2e tests name and stop the binaries on Windows.

package e2e

import (
	"os"
	"os/exec"
	"syscall"

	"golang.org/x/sys/windows"
)

// exeSuffix ends the name of a program.
const exeSuffix = ".exe"

// startInOwnGroup has cmd start in its own process group, so interrupt
// reaches it alone, and never the test or another test's binary.
func startInOwnGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: windows.CREATE_NEW_PROCESS_GROUP}
}

// interrupt sends Ctrl-Break to p's process group, which startInOwnGroup
// made. Go delivers it to the binary as os.Interrupt, so it takes the
// same shutdown path as Ctrl-C.
func interrupt(p *os.Process) error {
	return windows.GenerateConsoleCtrlEvent(windows.CTRL_BREAK_EVENT, uint32(p.Pid))
}

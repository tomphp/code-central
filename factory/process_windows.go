// How the Factory runs, stops and finds processes on Windows.

package factory

import (
	"os"
	"os/exec"
	"syscall"

	"golang.org/x/sys/windows"
)

// startInOwnGroup has cmd start a turn in its own process group, so the
// console's Ctrl-C reaches only the Factory.
func startInOwnGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: windows.CREATE_NEW_PROCESS_GROUP}
}

// stopSteps is how a turn is stopped, one step per grace period:
// Ctrl-Break to the turn's process group, then kill.
func stopSteps(p *os.Process) []func() error {
	return []func() error{
		func() error { return windows.GenerateConsoleCtrlEvent(windows.CTRL_BREAK_EVENT, uint32(p.Pid)) },
		p.Kill,
	}
}

// listProcesses is every process as a line "<pid> <command line>", read
// with PowerShell's Get-CimInstance.
func (e Env) listProcesses() (string, error) {
	ps, ok := e.lookPath("powershell")
	if !ok {
		ps = "powershell.exe"
	}
	cmd := exec.Command(ps, "-NoProfile", "-NonInteractive", "-Command",
		`Get-CimInstance Win32_Process | ForEach-Object { "$($_.ProcessId) $($_.CommandLine)" }`)
	cmd.Env = e.Environ
	out, err := cmd.Output()
	return string(out), err
}

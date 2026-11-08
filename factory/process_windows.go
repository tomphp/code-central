// How the Factory runs, stops and finds processes on Windows.

package factory

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"

	"golang.org/x/sys/windows"
)

// pathExts are the endings lookPath tries on a name, in order, as
// Windows does: PATHEXT's, or its default.
func (e Env) pathExts() []string {
	exts := e.getenv("PATHEXT")
	if exts == "" {
		exts = ".COM;.EXE;.BAT;.CMD"
	}
	var out []string
	for _, ext := range strings.Split(exts, ";") {
		if ext != "" {
			out = append(out, strings.ToLower(ext))
		}
	}
	return out
}

// executable reports whether the file is a program. Windows has no
// execute bits; the PATHEXT ending says it.
func executable(os.FileInfo) bool { return true }

// refuseClaudeShim refuses a claude that isn't the native claude.exe,
// such as the npm shim claude.cmd: Go runs a .cmd or .bat through
// cmd.exe, which cuts arguments at a newline and expands %...%, so a
// multi-line prompt wouldn't arrive intact.
func refuseClaudeShim(path string) error {
	if strings.EqualFold(filepath.Ext(path), ".exe") {
		return nil
	}
	return fmt.Errorf("claude on PATH is %s, not the native claude.exe: %s, like the npm shim claude.cmd, can't carry a multi-line prompt, because Windows runs it through cmd.exe, which cuts arguments at a newline. "+
		"Install the native claude.exe with `irm https://claude.ai/install.ps1 | iex` in PowerShell, put it before the npm folder on PATH, and start the Factory again", path, filepath.Base(path))
}

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
// with PowerShell's Get-CimInstance. A turn's command line holds its
// multi-line prompt, so its line breaks become spaces.
func (e Env) listProcesses() (string, error) {
	ps, ok := e.lookPath("powershell")
	if !ok {
		ps = "powershell.exe"
	}
	cmd := exec.Command(ps, "-NoProfile", "-NonInteractive", "-Command",
		`Get-CimInstance Win32_Process | ForEach-Object { "$($_.ProcessId) $($_.CommandLine -replace '[\r\n]+', ' ')" }`)
	cmd.Env = e.Environ
	out, err := cmd.Output()
	return string(out), err
}

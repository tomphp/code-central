// How the Factory runs, stops and finds processes on Windows.

package factory

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"unsafe"

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
// natively: a snapshot of the processes, and each one's command line
// from NtQueryInformationProcess. Starting PowerShell for the same list
// took seconds, more under load. A process the Factory can't open, such
// as another user's, is left out. A turn's command line holds its
// multi-line prompt, so its line breaks become spaces.
func (Env) listProcesses() (string, error) {
	snap, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return "", fmt.Errorf("snapshot the processes: %w", err)
	}
	defer windows.CloseHandle(snap)
	var b strings.Builder
	entry := windows.ProcessEntry32{Size: uint32(unsafe.Sizeof(windows.ProcessEntry32{}))}
	for err = windows.Process32First(snap, &entry); err == nil; err = windows.Process32Next(snap, &entry) {
		args, ok := commandLine(entry.ProcessID)
		if !ok {
			continue
		}
		fmt.Fprintf(&b, "%d %s\n", entry.ProcessID, lineBreaks.Replace(args))
	}
	if !errors.Is(err, windows.ERROR_NO_MORE_FILES) {
		return "", fmt.Errorf("read the processes: %w", err)
	}
	return b.String(), nil
}

var lineBreaks = strings.NewReplacer("\r\n", " ", "\r", " ", "\n", " ")

// commandLine is the command line of the process pid, if the Factory
// can open it.
func commandLine(pid uint32) (string, bool) {
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
	if err != nil {
		return "", false
	}
	defer windows.CloseHandle(h)
	// The answer is a UNICODE_STRING followed by the text it points to.
	buf := make([]byte, 4096)
	for {
		var n uint32
		err := windows.NtQueryInformationProcess(h, windows.ProcessCommandLineInformation, unsafe.Pointer(&buf[0]), uint32(len(buf)), &n)
		switch {
		case err == nil:
			return (*windows.NTUnicodeString)(unsafe.Pointer(&buf[0])).String(), true
		case (errors.Is(err, windows.STATUS_INFO_LENGTH_MISMATCH) || errors.Is(err, windows.STATUS_BUFFER_TOO_SMALL) || errors.Is(err, windows.STATUS_BUFFER_OVERFLOW)) && int(n) > len(buf):
			buf = make([]byte, n)
		default:
			return "", false
		}
	}
}

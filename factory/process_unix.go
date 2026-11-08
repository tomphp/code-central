//go:build unix

// How the Factory runs, stops and finds processes on Unix.

package factory

import (
	"os"
	"os/exec"
	"syscall"
)

// startInOwnGroup has cmd start a turn in its own process group, so the
// terminal's Ctrl-C reaches only the Factory.
func startInOwnGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

// stopSteps is how a turn is stopped, one step per grace period: SIGINT,
// then SIGTERM, then kill.
func stopSteps(p *os.Process) []func() error {
	var steps []func() error
	for _, sig := range []os.Signal{syscall.SIGINT, syscall.SIGTERM, os.Kill} {
		steps = append(steps, func() error { return p.Signal(sig) })
	}
	return steps
}

// listProcesses is every process as a line "<pid> <command line>", read
// with ps.
func (e Env) listProcesses() (string, error) {
	ps, ok := e.lookPath("ps")
	if !ok {
		ps = "/bin/ps"
	}
	cmd := exec.Command(ps, "-A", "-o", "pid=,args=")
	cmd.Env = e.Environ
	out, err := cmd.Output()
	return string(out), err
}

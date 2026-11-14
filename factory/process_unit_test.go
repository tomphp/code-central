package factory

import (
	"bufio"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"testing"
	"time"
)

// listedEnv has the test binary, run by startWaiting, wait to be listed
// or stopped.
const listedEnv = "FACTORY_TEST_LISTED"

func TestListedProcessWaits(t *testing.T) {
	if os.Getenv(listedEnv) == "" {
		t.Skip("run only as startWaiting's child")
	}
	os.Stdout.WriteString("ready\n")
	time.Sleep(time.Minute)
}

// startWaiting starts the test binary as a child that waits a minute,
// with args after its flags, and returns once the child says it is
// ready, with the rest of its stdout. The child is killed at the end of
// the test.
func startWaiting(t *testing.T, args ...string) (*exec.Cmd, *bufio.Reader) {
	t.Helper()
	cmd := exec.Command(os.Args[0], append([]string{"-test.run=^TestListedProcessWaits$", "--"}, args...)...)
	cmd.Env = append(os.Environ(), listedEnv+"=1")
	pipe, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	})
	stdout := bufio.NewReader(pipe)
	if line, err := stdout.ReadString('\n'); line != "ready\n" {
		_ = cmd.Wait()
		t.Fatalf("the child printed %q (%v) and ended with %v; want it to say it is ready", line, err, cmd.ProcessState)
	}
	return cmd, stdout
}

func TestTheProcessListHasEachProcessesCommandLineOnOneLine(t *testing.T) {
	t.Parallel()
	cmd, _ := startWaiting(t, "first line\nsecond line")

	out, err := Env{Environ: os.Environ()}.listProcesses()
	if err != nil {
		t.Fatal(err)
	}
	prefix := strconv.Itoa(cmd.Process.Pid) + " "
	for _, line := range strings.Split(out, "\n") {
		if args, ok := strings.CutPrefix(strings.TrimSpace(line), prefix); ok {
			if !strings.Contains(args, "first line") || !strings.Contains(args, "second line") {
				t.Fatalf("pid %d is listed as %q; want its whole command line on its one line", cmd.Process.Pid, args)
			}
			return
		}
	}
	t.Fatalf("pid %d isn't in the process list:\n%s", cmd.Process.Pid, out)
}

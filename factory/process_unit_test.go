package factory

import (
	"os"
	"os/exec"
	"strconv"
	"strings"
	"testing"
	"time"
)

// listedEnv has the test binary, run by
// TestTheProcessListHasEachProcessesCommandLineOnOneLine, wait to be
// listed.
const listedEnv = "FACTORY_TEST_LISTED"

func TestListedProcessWaits(t *testing.T) {
	if os.Getenv(listedEnv) == "" {
		t.Skip("run only as TestTheProcessListHasEachProcessesCommandLineOnOneLine's child")
	}
	time.Sleep(time.Minute)
}

func TestTheProcessListHasEachProcessesCommandLineOnOneLine(t *testing.T) {
	t.Parallel()
	cmd := exec.Command(os.Args[0], "-test.run=^TestListedProcessWaits$", "--", "first line\nsecond line")
	cmd.Env = append(os.Environ(), listedEnv+"=1")
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	})

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

package factory

import (
	"runtime"
	"testing"
	"time"

	"github.com/tomoram/code-central/core"
)

func TestATurnKilledBySignalSaysWhichSignal(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows has no signals; a killed process exits 1")
	}
	t.Parallel()
	cmd, stdout := startWaiting(t)
	if err := cmd.Process.Kill(); err != nil {
		t.Fatal(err)
	}

	end := readTurn(cmd, stdout, t.TempDir(), newUsageReader("implementation", time.Now), func(core.SessionEvent) {}, func(core.UsageResponse) {})
	if got, want := end.how(), "signal: killed, with no result message"; got != want {
		t.Errorf("the turn ended %q, want %q", got, want)
	}
}

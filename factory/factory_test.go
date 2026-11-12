package factory_test

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/tomoram/code-central/factory/factorytest"
	"github.com/tomoram/code-central/store"
)

func TestMain(m *testing.M) { factorytest.Main(m) }

// refuses runs the Factory from dir and checks it refused to start,
// saying why, without claiming anything.
func refuses(t *testing.T, h *factorytest.Harness, dir, why string, args ...string) {
	t.Helper()
	code, _, stderr := h.RunIn(dir, args...)
	if code == 0 || !strings.Contains(stderr, why) {
		t.Fatalf("exit %d, stderr %q; want a refusal saying %q", code, stderr, why)
	}
	if calls := h.Calls(); len(calls) != 0 {
		t.Fatalf("a refused Factory ran claude: %+v", calls)
	}
}

func TestTheFactoryRefusesToStartOutsideAGitRepo(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t)
	refuses(t, h, t.TempDir(), "not in a git repository")
}

func TestTheFactoryRefusesToStartWithNoProject(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t)
	h.Git("rm", "-q", "--cached", ".mcp.json")
	h.Git("commit", "-q", "-m", "No Project")
	if err := os.Remove(filepath.Join(h.Repo, ".mcp.json")); err != nil {
		t.Fatal(err)
	}
	refuses(t, h, h.Repo, "no Project")
}

func TestTheFactoryRefusesToStartWhenMCPJSONIsNotCommitted(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t)
	h.Git("rm", "-q", "--cached", ".mcp.json")
	h.Git("commit", "-q", "-m", "Keep .mcp.json local")
	refuses(t, h, h.Repo, ".mcp.json isn't committed")
}

func TestTheFactoryRefusesToStartWithoutClaudeOnPath(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t)
	h.RemoveClaude()
	refuses(t, h, h.Repo, "claude isn't on PATH")
}

func TestTheFactoryRefusesToStartWhenClaudeIsOnlyTheNpmCmdShim(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t)
	h.ShimClaude()
	if runtime.GOOS != "windows" {
		// Only Windows runs a .cmd for the bare name claude.
		refuses(t, h, h.Repo, "claude isn't on PATH")
		return
	}
	code, _, stderr := h.Run()
	for _, why := range []string{"claude.cmd, like the npm shim claude.cmd, can't carry a multi-line prompt", "irm https://claude.ai/install.ps1 | iex"} {
		if code == 0 || !strings.Contains(stderr, why) {
			t.Errorf("exit %d, stderr %q; want a refusal saying %q", code, stderr, why)
		}
	}
}

func TestTheFactoryRefusesToStartWithGitOlderThan2_38(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t)
	h.FakeGit("2.37.9")
	refuses(t, h, h.Repo, "git 2.37.9 is older than 2.38")
}

func TestTheFactoryStartsOnADetachedHead(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t)
	id := h.ReadyTicket("From anywhere")
	h.Git("checkout", "-q", "--detach")
	h.Start().WaitForOutput("base: main")
	h.WaitForState(id, store.InReview)
}

func TestTheFactoryRefusesTheBaseFlagAndPointsToProjectOptions(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t)
	refuses(t, h, h.Repo, "Project Options' Base branch field", "--base", "release")
}

func TestTheFactoryRefusesToStartWhenTheWorktreeFolderCannotBeMade(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t)
	if err := os.WriteFile(h.Repo+".worktrees", []byte("in the way"), 0o644); err != nil {
		t.Fatal(err)
	}
	refuses(t, h, h.Repo, h.Repo+".worktrees")
}

func TestTheFactoryRefusesBadFlags(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t)
	refuses(t, h, h.Repo, "--parallel must be at least 1", "--parallel", "0")
	refuses(t, h, h.Repo, "--poll must be at least 5s", "--poll", "4s")
}

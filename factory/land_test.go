package factory_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tomoram/code-central/core"
	"github.com/tomoram/code-central/factory/factorytest"
	"github.com/tomoram/code-central/store"
)

// accepted makes an Accepted Ticket whose branch adds file.
func accepted(h *factorytest.Harness, title, file, content string) (id, commit string) {
	id = h.ReadyTicket(title)
	commit = h.CommitOn(id, file, content)
	h.Finished(id, id, commit)
	h.Accept(id)
	return id, commit
}

func TestAnAcceptedTicketIsMergedIntoTheBaseAndLanded(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t)
	id, commit := accepted(h, "Add a feature", "feature.txt", "feature\n")
	oldMain := h.Git("rev-parse", "main")

	h.Start()
	tk := h.WaitForState(id, store.Done)

	merge := h.Git("rev-parse", "main")
	if got := h.Git("log", "-1", "--format=%P", merge); got != oldMain+" "+commit {
		t.Errorf("main's tip has parents %q, want %q then %q", got, oldMain, commit)
	}
	if got := h.Git("log", "-1", "--format=%s", merge); got != "Merge "+id+": Add a feature" {
		t.Errorf("merge commit subject %q", got)
	}
	if m := tk.Moves[len(tk.Moves)-1]; m.Commit != merge || m.From != store.Accepted {
		t.Errorf("land move %+v, want from Accepted with commit %s", m, merge)
	}
	// main is checked out in the repo, so it was fast-forwarded there.
	if b, err := os.ReadFile(filepath.Join(h.Repo, "feature.txt")); err != nil || string(b) != "feature\n" {
		t.Errorf("feature.txt in the checkout: %q, %v", b, err)
	}
	if st := h.Git("status", "--porcelain"); st != "" {
		t.Errorf("the base checkout isn't clean after landing: %q", st)
	}
}

func TestAConflictIsSentBackNamingTheBaseCommitAndTheConflictingFiles(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t)
	id, _ := accepted(h, "Change the readme", "README", "theirs\n")
	h.CommitOn("main", "README", "ours\n")
	base := h.Git("rev-parse", "main")

	// Hold the next Claim's turn, so the Ticket stays where send_back put it.
	h.Script(id, factorytest.Wait())
	h.Start()
	h.WaitFor(id+" to be sent back", func() bool { return movedTo(h.Ticket(id), store.Accepted, store.Ready) })

	if h.Git("rev-parse", "main") != base {
		t.Error("main moved on a conflict")
	}
	if !commented(h.Ticket(id), base[:12], "README") {
		t.Errorf("no send_back note naming the base commit %s and README: %+v", base[:12], h.Ticket(id).Comments)
	}
}

func TestUncommittedChangesInTheBaseCheckoutHoldACleanMergeAndAreLeftAlone(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t)
	id, _ := accepted(h, "Add a feature", "feature.txt", "feature\n")
	mine := filepath.Join(h.Repo, "feature.txt")
	if err := os.WriteFile(mine, []byte("my work in progress\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	base := h.Git("rev-parse", "main")

	h.Start()
	tk := h.Ticket(id)
	if tk.State != store.Accepted || h.Git("rev-parse", "main") != base {
		t.Fatalf("%s is %s and main at %s; want it left Accepted with main unmoved", id, tk.State, h.Git("rev-parse", "main"))
	}
	if !commented(tk, "uncommitted changes in "+h.Repo, "feature.txt") {
		t.Errorf("want a comment naming feature.txt, got %+v", tk.Comments)
	}
	if b, _ := os.ReadFile(mine); string(b) != "my work in progress\n" {
		t.Errorf("the human's file was touched: %q", b)
	}
}

// movedTo reports whether tk ever moved from one State to another.
func movedTo(tk core.TicketView, from, to store.State) bool {
	for _, m := range tk.Moves {
		if m.From == from && m.To == to {
			return true
		}
	}
	return false
}

// commented reports whether one of tk's comments contains every part.
func commented(tk core.TicketView, parts ...string) bool {
	for _, c := range tk.Comments {
		all := true
		for _, p := range parts {
			all = all && strings.Contains(c.Body, p)
		}
		if all {
			return true
		}
	}
	return false
}

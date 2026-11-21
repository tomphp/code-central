package factory_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tomoram/code-central/api"
	"github.com/tomoram/code-central/domain"

	"github.com/tomoram/code-central/factory/factorytest"
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
	h := factorytest.New(t, factorytest.RealGit())
	id, commit := accepted(h, "Add a feature", "feature.txt", "feature\n")
	oldMain := h.Tip("main")

	h.Start()
	tk := h.WantState(id, domain.Done)

	merge := h.Tip("main")
	if got := strings.Join(h.Parents(merge), " "); got != oldMain+" "+commit {
		t.Errorf("main's tip has parents %q, want %q then %q", got, oldMain, commit)
	}
	if got := h.Subject(merge); got != "Merge "+id+": Add a feature" {
		t.Errorf("merge commit subject %q", got)
	}
	if m := tk.Moves[len(tk.Moves)-1]; m.Commit != merge || m.From != domain.Accepted {
		t.Errorf("land move %+v, want from Accepted with commit %s", m, merge)
	}
	// main is checked out in the repo, so it was fast-forwarded there.
	if b, err := os.ReadFile(filepath.Join(h.Repo, "feature.txt")); err != nil || string(b) != "feature\n" {
		t.Errorf("feature.txt in the checkout: %q, %v", b, err)
	}
	if st := h.Status(); st != "" {
		t.Errorf("the base checkout isn't clean after landing: %q", st)
	}
}

func TestAConflictIsSentBackNamingTheBaseCommitAndTheConflictingFiles(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t, factorytest.RealGit())
	id, _ := accepted(h, "Change the readme", "README", "theirs\n")
	h.CommitOn("main", "README", "ours\n")
	base := h.Tip("main")

	// Hold the next Claim's turn, so the Ticket stays where send_back put it.
	h.Script(id, factorytest.Wait())
	h.Start()
	if !movedTo(h.Ticket(id), domain.Accepted, domain.Ready) {
		t.Fatalf("%s wasn't sent back: %+v", id, h.Ticket(id).Moves)
	}

	if h.Tip("main") != base {
		t.Error("main moved on a conflict")
	}
	if !commented(h.Ticket(id), base[:12], "README") {
		t.Errorf("no send_back note naming the base commit %s and README: %+v", base[:12], h.Ticket(id).Comments)
	}
}

func TestUncommittedChangesInTheBaseCheckoutHoldACleanMergeAndAreLeftAlone(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t, factorytest.FakeGit())
	id, _ := accepted(h, "Add a feature", "feature.txt", "feature\n")
	mine := filepath.Join(h.Repo, "feature.txt")
	if err := os.WriteFile(mine, []byte("my work in progress\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	base := h.Tip("main")

	h.Start()
	tk := h.Ticket(id)
	if tk.State != domain.Accepted || h.Tip("main") != base {
		t.Fatalf("%s is %s and main at %s; want it left Accepted with main unmoved", id, tk.State, h.Tip("main"))
	}
	if !commented(tk, "uncommitted changes in "+h.Repo, "feature.txt") {
		t.Errorf("want a comment naming feature.txt, got %+v", tk.Comments)
	}
	if b, _ := os.ReadFile(mine); string(b) != "my work in progress\n" {
		t.Errorf("the human's file was touched: %q", b)
	}
}

// movedTo reports whether tk ever moved from one State to another.
func movedTo(tk api.TicketView, from, to domain.State) bool {
	for _, m := range tk.Moves {
		if m.From == from && m.To == to {
			return true
		}
	}
	return false
}

// commented reports whether one of tk's comments contains every part.
func commented(tk api.TicketView, parts ...string) bool {
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

func TestAnAcceptedTicketIsMergedIntoTheBaseTheProjectNames(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t, factorytest.FakeGit())
	h.NewBranch("release", "main")
	base := "release"
	h.SetLandingMethod(api.SetLandingMethodInput{Method: domain.LandingMerge, Base: &base})
	id, commit := accepted(h, "Add a feature", "feature.txt", "feature\n")
	oldMain := h.Tip("main")
	oldRelease := h.Tip("release")

	r := h.Start()
	h.WantState(id, domain.Done)

	if h.Tip("main") != oldMain {
		t.Error("main moved, want the merge on release")
	}
	merge := h.Tip("release")
	if got := strings.Join(h.Parents(merge), " "); got != oldRelease+" "+commit {
		t.Errorf("release's tip has parents %q, want %q then %q", got, oldRelease, commit)
	}

	// Changing the base takes hold on the next poll.
	h.NewBranch("next", "main")
	next := "next"
	h.SetLandingMethod(api.SetLandingMethodInput{Method: domain.LandingMerge, Base: &next})
	id2, commit2 := accepted(h, "Another feature", "other.txt", "other\n")
	r.Poll()
	h.WantState(id2, domain.Done)
	if h.Tip("release") != merge {
		t.Error("release moved after the base changed")
	}
	if got := strings.Join(h.Parents(h.Tip("next")), " "); !strings.HasSuffix(got, " "+commit2) {
		t.Errorf("next's tip has parents %q, want it to end at %s", got, commit2)
	}
}

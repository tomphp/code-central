package factory_test

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/tomoram/code-central/core"
	"github.com/tomoram/code-central/factory/factorytest"
	"github.com/tomoram/code-central/store"
)

// researched scripts research Ticket id to commit its findings on its
// branch and finish with no branch, as the map-research guide says.
func researched(h *factorytest.Harness, id string) {
	h.Script(id,
		factorytest.WriteFile("findings.md", "# Colour libraries\n"),
		factorytest.Git("add", "findings.md"),
		factorytest.Git("commit", "-q", "-m", "Colour libraries"),
		factorytest.Finish("Two libraries fit."),
		factorytest.Result("success"))
}

func TestAMapResearchTicketStartsTheResearchSkillInItsOwnWorktreeOnALocalResearchBranch(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t)
	id := h.LabelledTicket("wayfinder:research", "Which colour libraries exist?")
	researched(h, id)
	mainTip := h.Git("rev-parse", "main")

	h.Start()
	h.WaitForState(id, store.InReview)

	c := h.Calls()[0]
	branch := "research/" + id
	if want := "/research " + id + "\n\nTicket: " + id + "\n"; !strings.HasPrefix(c.Prompt, want) {
		t.Errorf("prompt %q doesn't start %q", c.Prompt, want)
	}
	for _, want := range []string{
		guide(t, h, "map-research"),
		`get_stage_guide, stage "map-research"`,
		"The Factory has already claimed this Ticket for you: Claim " + c.Claim + " is yours",
		"in this Ticket's own worktree, " + h.Worktree(id) + ", on the local branch " + branch + ".",
		"Commit on this branch only, with no push and no PR.",
		"Record the research Tag as `" + branch + "@<commit>`",
		"finish with no branch",
	} {
		if !strings.Contains(c.Prompt, want) {
			t.Errorf("prompt %q doesn't say %q", c.Prompt, want)
		}
	}
	if strings.Contains(c.Prompt, "/implement") || strings.Contains(c.Prompt, guide(t, h, core.ImplementationStage)) {
		t.Errorf("prompt %q runs it as implementation", c.Prompt)
	}
	if c.Dir != h.Worktree(id) {
		t.Errorf("claude ran in %s, want the worktree %s", c.Dir, h.Worktree(id))
	}
	if head := h.GitIn(h.Worktree(id), "rev-parse", "--abbrev-ref", "HEAD"); head != branch {
		t.Errorf("the worktree is on %q, want branch %s", head, branch)
	}
	if got := h.Git("rev-parse", branch+"^"); got != mainTip {
		t.Errorf("%s was made from %s, want main's tip %s", branch, got, mainTip)
	}
	if b := h.Git("branch", "--list", id); b != "" {
		t.Errorf("a research Ticket made the implementation branch %q", b)
	}
}

func TestAnAcceptedMapResearchTicketIsDoneWithNothingPushedOrLandedUnderEveryLandingMethod(t *testing.T) {
	t.Parallel()
	for name, landing := range map[string]core.SetLandingMethodInput{
		"Merge":           {Method: store.LandingMerge},
		"Merge and Push":  {Method: store.LandingMerge, Push: true, Remote: "origin"},
		"Branch":          {Method: store.LandingBranch},
		"Branch and Push": {Method: store.LandingBranch, Push: true, Remote: "origin"},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			h := factorytest.New(t)
			remote := h.AddRemote("origin")
			h.SetLandingMethod(landing)
			id := h.LabelledTicket("wayfinder:research", "Which colour libraries exist?")
			researched(h, id)
			mainTip := h.Git("rev-parse", "main")

			r := h.Start()
			h.WaitForState(id, store.InReview)
			commit := h.Git("rev-parse", "research/"+id)
			h.Accept(id)
			if tk := h.Ticket(id); tk.State != store.Done {
				t.Fatalf("%s is %s once accepted, want Done straight away", id, tk.State)
			}
			r.Poll()
			r.Poll()

			if got := h.Git("rev-parse", "main"); got != mainTip {
				t.Errorf("main moved to %s, want it left at %s", got, mainTip)
			}
			if got := remote.Git("for-each-ref", "--format=%(refname)"); got != "" {
				t.Errorf("the remote got %q, want nothing pushed", got)
			}
			if got := h.Git("rev-parse", "research/"+id); got != commit {
				t.Errorf("research/%s is at %q, want it kept at %s", id, got, commit)
			}
			if out := r.Stdout(); strings.Contains(out, "landed") || strings.Contains(out, "pushed") {
				t.Errorf("the Factory landed or pushed: %q", out)
			}
		})
	}
}

func TestAMapResearchTicketsWorktreeIsKeptInReviewAndSweptOnceDone(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t)
	id := h.LabelledTicket("wayfinder:research", "Which colour libraries exist?")
	researched(h, id)

	r := h.Start()
	h.WaitForState(id, store.InReview)
	r.Poll()
	r.Poll()
	if _, err := os.Stat(h.Worktree(id)); err != nil {
		t.Fatalf("the worktree went in In Review: %v", err)
	}

	commit := h.Git("rev-parse", "research/"+id)
	h.Accept(id)
	r.Poll()
	r.WaitForOutput(id + ": Done: removed its worktree " + h.Worktree(id))
	if _, err := os.Stat(h.Worktree(id)); !os.IsNotExist(err) {
		t.Errorf("the worktree %s is still there: %v", h.Worktree(id), err)
	}
	if got := h.Git("rev-parse", "research/"+id); got != commit {
		t.Errorf("research/%s is at %q, want it kept at %s", id, got, commit)
	}
}

func TestACancelledMapResearchTicketsWorktreeIsSweptAndItsBranchKept(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t)
	id := h.LabelledTicket("wayfinder:research", "Which colour libraries exist?")
	researched(h, id)

	r := h.Start()
	h.WaitForState(id, store.InReview)
	commit := h.Git("rev-parse", "research/"+id)
	if _, err := h.Human().Cancel(context.Background(), h.Project, core.CancelInput{Ticket: core.Ref(id)}); err != nil {
		t.Fatal(err)
	}
	r.Poll()
	r.WaitForOutput(id + ": Cancelled: removed its worktree " + h.Worktree(id))
	if got := h.Git("rev-parse", "research/"+id); got != commit {
		t.Errorf("research/%s is at %q, want it kept at %s", id, got, commit)
	}
}

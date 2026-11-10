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

// builtPrototype scripts Prototype id to commit a file on its branch and
// finish with no branch, as the prototype guide says.
func builtPrototype(h *factorytest.Harness, id string) {
	h.Script(id,
		factorytest.WriteFile("picker.html", "<select></select>\n"),
		factorytest.Git("add", "picker.html"),
		factorytest.Git("commit", "-q", "-m", "A theme picker"),
		factorytest.Finish("Learned that a picker beats a toggle."),
		factorytest.Result("success"))
}

func TestAPrototypeStartsThePrototypeSkillInItsOwnWorktreeOnALocalPrototypeBranch(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t)
	id := h.LabelledTicket("prototype", "Prototype: the theme picker")
	builtPrototype(h, id)
	mainTip := h.Git("rev-parse", "main")

	h.Start()
	h.WaitForState(id, store.InReview)

	c := h.Calls()[0]
	branch := "prototype/" + id
	if want := "/prototype " + id; c.Prompt != want {
		t.Errorf("prompt %q, want %q", c.Prompt, want)
	}
	for _, want := range []string{
		guide(t, h, "prototype"),
		"Ticket: " + id + "\nClaim: " + c.Claim + "\n",
		"The Stage Guide for prototype follows",
		"The Factory has already claimed this Ticket for you: Claim " + c.Claim + " is yours",
		"in this Ticket's own worktree, " + h.Worktree(id) + ", on the local branch " + branch + ".",
		"Commit on this branch only, with no push and no PR.",
		"Record the prototype Tag as `" + branch + "@<commit> " + h.Worktree(id) + "`",
		`open the first Round with a "Try it" Question`,
		"finish with no branch",
	} {
		if !strings.Contains(c.SystemPrompt, want) {
			t.Errorf("system prompt %q doesn't say %q", c.SystemPrompt, want)
		}
	}
	if strings.Contains(c.SystemPrompt, guide(t, h, core.ImplementationStage)) {
		t.Errorf("system prompt %q runs it as implementation", c.SystemPrompt)
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
		t.Errorf("a Prototype made the implementation branch %q", b)
	}
}

func TestAPrototypeIsReusedOnItsBranchAtARedo(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t)
	id := h.LabelledTicket("prototype", "Prototype: the theme picker")
	branch := "prototype/" + id
	commit := h.CommitOn(branch, "picker.html", "<select></select>\n")
	h.Script(id, factorytest.NeedFile("picker.html"), factorytest.Finish("Again."), factorytest.Result("success"))

	h.Start()
	h.WaitForState(id, store.InReview)

	if got := h.GitIn(h.Worktree(id), "rev-parse", "HEAD"); got != commit {
		t.Errorf("the worktree is at %s, want %s's earlier commit %s", got, branch, commit)
	}
}

func TestAnAcceptedPrototypeIsDoneWithNothingPushedOrLandedUnderEveryLandingMethod(t *testing.T) {
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
			id := h.LabelledTicket("prototype", "Prototype: the theme picker")
			builtPrototype(h, id)
			mainTip := h.Git("rev-parse", "main")

			r := h.Start()
			h.WaitForState(id, store.InReview)
			commit := h.Git("rev-parse", "prototype/"+id)
			h.Accept(id)
			tk := h.Ticket(id)
			if tk.State != store.Done {
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
			if got := h.Git("rev-parse", "prototype/"+id); got != commit {
				t.Errorf("prototype/%s is at %q, want it kept at %s", id, got, commit)
			}
			if out := r.Stdout(); strings.Contains(out, "landed") || strings.Contains(out, "pushed") {
				t.Errorf("the Factory landed or pushed: %q", out)
			}
			for _, c := range h.Ticket(id).Comments {
				if strings.Contains(c.Body, "The Factory can't") {
					t.Errorf("the Factory commented %q", c.Body)
				}
			}
		})
	}
}

func TestAPrototypesWorktreeIsKeptWhileTheHumanTriesItAndSweptOnceDone(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t)
	id := h.LabelledTicket("prototype", "Prototype: the theme picker")
	h.Script(id,
		factorytest.WriteFile("picker.html", "<select></select>\n"),
		factorytest.Git("add", "picker.html"),
		factorytest.Git("commit", "-q", "-m", "A theme picker"),
		factorytest.PostRound("Try it"),
		factorytest.Result("success"))

	r := h.Start()
	h.WaitForState(id, store.NeedsAnswer)
	r.Poll()
	r.Poll()
	if _, err := os.Stat(h.Worktree(id)); err != nil {
		t.Fatalf("the worktree went in Needs Answer: %v", err)
	}

	h.Script(id, factorytest.Finish("Learned that a picker beats a toggle."), factorytest.Result("success"))
	h.Answer(id, "Tried it: the picker is better.")
	r.Poll()
	h.WaitForState(id, store.InReview)
	r.Poll()
	r.Poll()
	if _, err := os.Stat(h.Worktree(id)); err != nil {
		t.Fatalf("the worktree went in In Review: %v", err)
	}

	commit := h.Git("rev-parse", "prototype/"+id)
	h.Accept(id)
	r.Poll()
	r.WaitForOutput(id + ": Done: removed its worktree " + h.Worktree(id))
	if _, err := os.Stat(h.Worktree(id)); !os.IsNotExist(err) {
		t.Errorf("the worktree %s is still there: %v", h.Worktree(id), err)
	}
	if got := h.Git("rev-parse", "prototype/"+id); got != commit {
		t.Errorf("prototype/%s is at %q, want it kept at %s", id, got, commit)
	}
}

func TestACancelledPrototypesWorktreeIsSweptAndItsBranchKept(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t)
	id := h.LabelledTicket("prototype", "Prototype: the theme picker")
	builtPrototype(h, id)

	r := h.Start()
	h.WaitForState(id, store.InReview)
	commit := h.Git("rev-parse", "prototype/"+id)
	if _, err := h.Human().Cancel(context.Background(), h.Project, core.CancelInput{Ticket: core.Ref(id)}); err != nil {
		t.Fatal(err)
	}
	r.Poll()
	r.WaitForOutput(id + ": Cancelled: removed its worktree " + h.Worktree(id))
	if got := h.Git("rev-parse", "prototype/"+id); got != commit {
		t.Errorf("prototype/%s is at %q, want it kept at %s", id, got, commit)
	}
}

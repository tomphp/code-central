package factory_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tomoram/code-central/core"
	"github.com/tomoram/code-central/factory/factorytest"
	"github.com/tomoram/code-central/store"
)

// guide is the Stage Guide Code Central serves for stage.
func guide(t *testing.T, h *factorytest.Harness, stage string) string {
	t.Helper()
	g, err := h.Server.Client.GetStageGuide(context.Background(), core.StageGuideInput{Stage: stage})
	if err != nil {
		t.Fatal(err)
	}
	return g.Guide
}

func TestEachSessionStartsItsStagesSkillWithItsStageGuide(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t)
	ids := map[string]string{ // guide stage -> Ticket
		"spec":                   h.StageTicket("spec", "Spec: Add a button"),
		"tickets":                h.StageTicket("tickets", "Tickets: Add a button"),
		core.ImplementationStage: h.ReadyTicket("Add a button"),
	}
	skills := map[string]string{"spec": "/to-spec ", "tickets": "/to-tickets ", core.ImplementationStage: "/implement "}

	h.Start()
	for _, id := range ids {
		h.WaitForState(id, store.InReview)
	}

	calls := map[string]factorytest.Call{}
	for _, c := range h.Calls() {
		calls[c.Ticket] = c
	}
	for stage, id := range ids {
		c := calls[id]
		if want := skills[stage] + id; c.Prompt != want {
			t.Errorf("%s: prompt %q, want only the skill and the Ticket, %q", stage, c.Prompt, want)
		}
		if want := "Ticket: " + id + "\nClaim: " + c.Claim + "\n"; !strings.HasPrefix(c.SystemPrompt, want) {
			t.Errorf("%s: system prompt %q doesn't start %q", stage, c.SystemPrompt, want)
		}
		if want := "The Stage Guide for " + stage + " follows: what this Ticket must produce and how to finish it.\n\n" + guide(t, h, stage); !strings.HasSuffix(c.SystemPrompt, want) {
			t.Errorf("%s: system prompt %q doesn't end with its Stage Guide", stage, c.SystemPrompt)
		}
	}
}

func TestEachSystemPromptSaysTheTicketIsAlreadyClaimedAndNotToCallClaim(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t)
	ids := map[string]string{ // stage -> Ticket
		"spec":                   h.StageTicket("spec", "Spec: Add a button"),
		core.ImplementationStage: h.ReadyTicket("Add a button"),
	}

	h.Start()
	for _, id := range ids {
		h.WaitForState(id, store.InReview)
	}

	calls := map[string]factorytest.Call{}
	for _, c := range h.Calls() {
		calls[c.Ticket] = c
	}
	for stage, id := range ids {
		c := calls[id]
		want := "The Factory has already claimed this Ticket for you: Claim " + c.Claim + " is yours, so don't call claim, and pass " + c.Claim + " as claim_id on every write that needs it."
		if !strings.Contains(c.SystemPrompt, want) {
			t.Errorf("%s: system prompt %q doesn't say %q", stage, c.SystemPrompt, want)
		}
	}
	impl := calls[ids[core.ImplementationStage]]
	if want := "in this Ticket's own worktree on branch " + impl.Ticket + ". The Factory has already claimed"; !strings.Contains(impl.SystemPrompt, want) {
		t.Errorf("implementation system prompt %q doesn't say %q", impl.SystemPrompt, want)
	}
	if want := "as claim_id on every write that needs it. Commit on this branch only, with no push and no PR, and finish the Ticket when it is done."; !strings.Contains(impl.SystemPrompt, want) {
		t.Errorf("implementation system prompt %q doesn't say %q", impl.SystemPrompt, want)
	}
	spec := calls[ids["spec"]].SystemPrompt
	if strings.Contains(spec, "Stage under Claim") {
		t.Errorf("Stage prompt %q still says to work the Stage under Claim", spec)
	}
	if want := "Finish the Ticket when it is done. A Stage writes nothing to the repository: no files, no commits and no branches."; !strings.Contains(spec, want) {
		t.Errorf("Stage prompt %q doesn't say %q", spec, want)
	}
}

func TestAStageWorksInADetachedWorktreeAtTheBasesTip(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t)
	id := h.StageTicket("spec", "Spec: Add a button")
	mainTip := h.Git("rev-parse", "main")

	h.Start()
	h.WaitForState(id, store.InReview)

	if c := h.Calls()[0]; c.Dir != h.Worktree(id) {
		t.Errorf("claude ran in %s, want the worktree %s", c.Dir, h.Worktree(id))
	}
	if head := h.GitIn(h.Worktree(id), "rev-parse", "--abbrev-ref", "HEAD"); head != "HEAD" {
		t.Errorf("the Stage's worktree is on branch %q, want it detached", head)
	}
	if got := h.GitIn(h.Worktree(id), "rev-parse", "HEAD"); got != mainTip {
		t.Errorf("the Stage's worktree is at %s, want main's tip %s", got, mainTip)
	}
	if b := h.Git("branch", "--list", id); b != "" {
		t.Errorf("a Stage made branch %q", b)
	}
}

func TestACleanStageWorktreeMovesToTheBasesCurrentTipAtANewClaim(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t)
	id := h.StageTicket("tickets", "Tickets: Add a button")
	h.Git("worktree", "add", "-q", "--detach", h.Worktree(id), "main")
	h.Git("commit", "-q", "--allow-empty", "-m", "main moves on")
	mainTip := h.Git("rev-parse", "main")

	h.Start()
	h.WaitForState(id, store.InReview)

	if got := h.GitIn(h.Worktree(id), "rev-parse", "HEAD"); got != mainTip {
		t.Errorf("the Stage's worktree is at %s, want main's current tip %s", got, mainTip)
	}
}

func TestADirtyStageWorktreeIsAFailureReport(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t)
	id := h.StageTicket("spec", "Spec: Add a button")
	h.Git("worktree", "add", "-q", "--detach", h.Worktree(id), "main")
	if err := os.WriteFile(filepath.Join(h.Worktree(id), "notes.md"), []byte("written by a Stage"), 0o644); err != nil {
		t.Fatal(err)
	}

	h.Start()
	tk := h.WaitForState(id, store.Failed)

	if n := len(h.Calls()); n != 0 {
		t.Errorf("claude ran %d times in a dirty Stage worktree, want none", n)
	}
	r := tk.FailureReports[len(tk.FailureReports)-1]
	if !strings.Contains(r.WhatBroke+r.Why, "notes.md") || !strings.Contains(r.WhereWorkStands, h.Worktree(id)) {
		t.Errorf("failure report %+v doesn't name the file the Stage wrote and its worktree", r)
	}
	if got, _ := os.ReadFile(filepath.Join(h.Worktree(id), "notes.md")); string(got) != "written by a Stage" {
		t.Errorf("the dirty worktree was changed: notes.md = %q", got)
	}
}

func TestTheGrillingStageStartsGrillingWithDomainModelingAndItsStageGuide(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t)
	id := h.StageTicket("grilling", "Grilling: Add a button")

	h.Start()
	h.WaitForState(id, store.InReview)

	c := h.Calls()[0]
	if c.Prompt != "/grilling "+id {
		t.Errorf("prompt %q, want /grilling %s", c.Prompt, id)
	}
	if !strings.Contains(c.SystemPrompt, "Run /domain-modeling alongside /grilling") {
		t.Errorf("system prompt %q doesn't start domain-modeling too", c.SystemPrompt)
	}
	if !strings.Contains(c.SystemPrompt, guide(t, h, "grilling")) {
		t.Errorf("system prompt %q doesn't paste in the grilling Stage Guide", c.SystemPrompt)
	}
}

func TestEachMapTicketStartsItsSkillWithItsGuideInADetachedWorktree(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t)
	ids := map[string]string{ // guide -> Ticket
		"map-chart":    h.LabelledTicket("wayfinder:chart", "Charting: Dark mode"),
		"map-grilling": h.LabelledTicket("wayfinder:grilling", "Which themes?"),
		"map-task":     h.LabelledTicket("wayfinder:task", "Set up the design account"),
	}
	mainTip := h.Git("rev-parse", "main")

	h.Start()
	for _, id := range ids {
		h.WaitForState(id, store.InReview)
	}

	calls := map[string]factorytest.Call{}
	for _, c := range h.Calls() {
		calls[c.Ticket] = c
	}
	prompts := map[string]string{
		"map-chart":    "/wayfinder " + ids["map-chart"],
		"map-grilling": "/grilling " + ids["map-grilling"],
		"map-task":     "Work Ticket " + ids["map-task"] + ", as the Factory's instructions and its Stage Guide in your system prompt say.", // no skill
	}
	for name, id := range ids {
		c := calls[id]
		if c.Prompt != prompts[name] {
			t.Errorf("%s: prompt %q, want %q", name, c.Prompt, prompts[name])
		}
		if !strings.Contains(c.SystemPrompt, guide(t, h, name)) {
			t.Errorf("%s: system prompt %q doesn't paste in its guide", name, c.SystemPrompt)
		}
		if want := "in a detached, read-only worktree at the base's tip, for this " + name + " Ticket."; !strings.Contains(c.SystemPrompt, want) {
			t.Errorf("%s: system prompt %q doesn't say %q", name, c.SystemPrompt, want)
		}
		if want := "It writes nothing to the repository: no files, no commits and no branches."; !strings.Contains(c.SystemPrompt, want) {
			t.Errorf("%s: system prompt %q doesn't say %q", name, c.SystemPrompt, want)
		}
		if want := "The Stage Guide for " + name + " follows"; !strings.Contains(c.SystemPrompt, want) {
			t.Errorf("%s: system prompt %q doesn't say %q", name, c.SystemPrompt, want)
		}
		if strings.Contains(c.SystemPrompt, guide(t, h, core.ImplementationStage)) {
			t.Errorf("%s: system prompt %q runs it as implementation", name, c.SystemPrompt)
		}
		if c.Dir != h.Worktree(id) {
			t.Errorf("%s: claude ran in %s, want the worktree %s", name, c.Dir, h.Worktree(id))
		}
		if head := h.GitIn(h.Worktree(id), "rev-parse", "--abbrev-ref", "HEAD"); head != "HEAD" {
			t.Errorf("%s: the worktree is on branch %q, want it detached", name, head)
		}
		if got := h.GitIn(h.Worktree(id), "rev-parse", "HEAD"); got != mainTip {
			t.Errorf("%s: the worktree is at %s, want main's tip %s", name, got, mainTip)
		}
		if b := h.Git("branch", "--list", id); b != "" {
			t.Errorf("%s: made branch %q", name, b)
		}
	}
	if p := calls[ids["map-grilling"]].SystemPrompt; !strings.Contains(p, "Run /domain-modeling alongside /grilling") {
		t.Errorf("map-grilling system prompt %q doesn't start domain-modeling too", p)
	}
	for _, name := range []string{"map-chart", "map-task"} {
		if p := calls[ids[name]].SystemPrompt; strings.Contains(p, "/domain-modeling") {
			t.Errorf("%s system prompt %q starts domain-modeling", name, p)
		}
	}
}

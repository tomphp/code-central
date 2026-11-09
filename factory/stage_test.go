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

	prompts := map[string]string{}
	for _, c := range h.Calls() {
		prompts[c.Ticket] = c.Prompt
	}
	for stage, id := range ids {
		p := prompts[id]
		if !strings.HasPrefix(p, skills[stage]+id) {
			t.Errorf("%s: prompt %q doesn't start %s%s", stage, p, skills[stage], id)
		}
		if !strings.Contains(p, guide(t, h, stage)) {
			t.Errorf("%s: prompt %q doesn't paste in its Stage Guide", stage, p)
		}
		if !strings.Contains(p, "get_stage_guide") {
			t.Errorf("%s: prompt %q doesn't name get_stage_guide", stage, p)
		}
	}
}

func TestEachPromptSaysTheTicketIsAlreadyClaimedAndNotToCallClaim(t *testing.T) {
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
		if !strings.Contains(c.Prompt, want) {
			t.Errorf("%s: prompt %q doesn't say %q", stage, c.Prompt, want)
		}
	}
	impl := calls[ids[core.ImplementationStage]]
	if want := "in this Ticket's own worktree on branch " + impl.Ticket + ". The Factory has already claimed"; !strings.Contains(impl.Prompt, want) {
		t.Errorf("implementation prompt %q doesn't say %q", impl.Prompt, want)
	}
	if want := "as claim_id on every write that needs it. Commit on this branch only, with no push and no PR, and finish the Ticket when it is done."; !strings.Contains(impl.Prompt, want) {
		t.Errorf("implementation prompt %q doesn't say %q", impl.Prompt, want)
	}
	spec := calls[ids["spec"]].Prompt
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
	if !strings.HasPrefix(c.Prompt, "/grilling "+id) {
		t.Errorf("prompt %q doesn't start /grilling %s", c.Prompt, id)
	}
	if !strings.Contains(c.Prompt, "/domain-modeling") {
		t.Errorf("prompt %q doesn't start domain-modeling too", c.Prompt)
	}
	if !strings.Contains(c.Prompt, guide(t, h, "grilling")) {
		t.Errorf("prompt %q doesn't paste in the grilling Stage Guide", c.Prompt)
	}
	if !strings.Contains(c.Prompt, "get_stage_guide") {
		t.Errorf("prompt %q doesn't name get_stage_guide", c.Prompt)
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
	starts := map[string]string{
		"map-chart":    "/wayfinder " + ids["map-chart"] + "\n\nTicket: " + ids["map-chart"] + "\n",
		"map-grilling": "/grilling " + ids["map-grilling"] + "\n\nTicket: " + ids["map-grilling"] + "\n",
		"map-task":     "Ticket: " + ids["map-task"] + "\n", // no skill: the guide alone
	}
	for name, id := range ids {
		c := calls[id]
		if !strings.HasPrefix(c.Prompt, starts[name]) {
			t.Errorf("%s: prompt %q doesn't start %q", name, c.Prompt, starts[name])
		}
		if !strings.Contains(c.Prompt, guide(t, h, name)) {
			t.Errorf("%s: prompt %q doesn't paste in its guide", name, c.Prompt)
		}
		if want := "in a detached, read-only worktree at the base's tip, for this " + name + " Ticket."; !strings.Contains(c.Prompt, want) {
			t.Errorf("%s: prompt %q doesn't say %q", name, c.Prompt, want)
		}
		if want := "It writes nothing to the repository: no files, no commits and no branches."; !strings.Contains(c.Prompt, want) {
			t.Errorf("%s: prompt %q doesn't say %q", name, c.Prompt, want)
		}
		if want := `get_stage_guide, stage "` + name + `"`; !strings.Contains(c.Prompt, want) {
			t.Errorf("%s: prompt %q doesn't say %q", name, c.Prompt, want)
		}
		if strings.Contains(c.Prompt, "/implement") || strings.Contains(c.Prompt, guide(t, h, core.ImplementationStage)) {
			t.Errorf("%s: prompt %q runs it as implementation", name, c.Prompt)
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
	if p := calls[ids["map-grilling"]].Prompt; !strings.Contains(p, "Run /domain-modeling alongside /grilling") {
		t.Errorf("map-grilling prompt %q doesn't start domain-modeling too", p)
	}
	for _, name := range []string{"map-chart", "map-task"} {
		if p := calls[ids[name]].Prompt; strings.Contains(p, "/domain-modeling") {
			t.Errorf("%s prompt %q starts domain-modeling", name, p)
		}
	}
}

func TestAMapResearchTicketIsLeftUnclaimed(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t)
	research := h.LabelledTicket("wayfinder:research", "Which colour libraries exist?")
	later := h.ReadyTicket("Add a button") // claimed after it in claim order

	h.Start()
	h.WaitForState(later, store.InReview)

	for _, c := range h.Calls() {
		if c.Ticket != later {
			t.Errorf("claude ran for %s: %q", c.Ticket, c.Prompt)
		}
	}
	if tk := h.Ticket(research); tk.State != store.Ready || len(tk.Claims) != 0 {
		t.Errorf("%s is %s with %d Claims, want Ready and never claimed", research, tk.State, len(tk.Claims))
	}
	if _, err := os.Stat(h.Worktree(research)); !os.IsNotExist(err) {
		t.Errorf("%s got a worktree: %v", research, err)
	}
}

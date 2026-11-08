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

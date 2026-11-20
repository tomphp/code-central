package factory_test

import (
	"github.com/tomoram/code-central/api"
	"github.com/tomoram/code-central/domain"
	"strings"
	"testing"

	"github.com/tomoram/code-central/factory/factorytest"
)

// sessionLogsRule opens what a Retro's system prompt says about where
// Claude Code keeps session logs.
const sessionLogsRule = "Claude Code keeps each session's log in its projects folder"

func TestARetroStartsTheRetroSkillReadOnlyInADetachedWorktreeAtTheBasesTip(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t)
	id := h.LabelledTicket(domain.RetroLabel, "Retro: Add a button")
	h.Script(id, factorytest.Finish("No candidates: see the resolution comment."), factorytest.Result("success"))
	mainTip := h.Git("rev-parse", "main")

	h.Start()
	h.WantState(id, domain.InReview)

	c := h.Calls()[0]
	if want := "/retro " + id; c.Prompt != want {
		t.Errorf("prompt %q, want %q", c.Prompt, want)
	}
	for _, want := range []string{
		"Ticket: " + id + "\nClaim: " + c.Claim + "\n",
		"The Factory has already claimed this Ticket for you: Claim " + c.Claim + " is yours",
		"in a detached, read-only worktree at the base's tip, for this retro Ticket.",
		"It writes nothing to the repository: no files, no commits and no branches.",
		sessionLogsRule,
		"`<projects folder>/<working_dir with every non-alphanumeric character replaced by '-'>/<session_id>.jsonl`",
		"that session's subagents' logs are under `<session_id>/subagents/` beside it",
		"The Stage Guide for retro follows: what this Ticket must produce and how to finish it.\n\n" + guide(t, h, domain.RetroLabel),
	} {
		if !strings.Contains(c.SystemPrompt, want) {
			t.Errorf("system prompt %q doesn't say %q", c.SystemPrompt, want)
		}
	}
	if strings.Contains(c.SystemPrompt, guide(t, h, api.ImplementationStage)) {
		t.Errorf("system prompt %q runs it as implementation", c.SystemPrompt)
	}
	if c.Dir != h.Worktree(id) {
		t.Errorf("claude ran in %s, want the worktree %s", c.Dir, h.Worktree(id))
	}
	if head := h.GitIn(h.Worktree(id), "rev-parse", "--abbrev-ref", "HEAD"); head != "HEAD" {
		t.Errorf("the Retro's worktree is on branch %q, want it detached", head)
	}
	if got := h.GitIn(h.Worktree(id), "rev-parse", "HEAD"); got != mainTip {
		t.Errorf("the Retro's worktree is at %s, want main's tip %s", got, mainTip)
	}
	if b := h.Git("branch", "--list", "*"+id); b != "" {
		t.Errorf("a Retro made branch %q", b)
	}
}

func TestOnlyARetrosSystemPromptSaysWhereSessionLogsAre(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t)
	ids := []string{
		h.StageTicket("spec", "Spec: Add a button"),
		h.LabelledTicket("wayfinder:task", "Set up the design account"),
		h.ReadyTicket("Add a button"),
	}

	h.Start()
	for _, id := range ids {
		h.WantState(id, domain.InReview)
	}

	for _, c := range h.Calls() {
		if strings.Contains(c.SystemPrompt, sessionLogsRule) {
			t.Errorf("%s: system prompt %q says where session logs are", c.Ticket, c.SystemPrompt)
		}
	}
}

package factory

import (
	"strings"
	"testing"

	"github.com/tomoram/code-central/api"
	"github.com/tomoram/code-central/factory/harness/claude"
)

var claudePrompts = claude.New(claude.Options{})

func TestAConflictFirstPromptNamesTheFilesAndNoSkill(t *testing.T) {
	c := claim{Ticket: "acme-7", ClaimID: "C7"}
	p := firstPrompt(claudePrompts, c, api.ImplementationStage, "main", []string{"README", "a/b.go"}, false)
	for _, want := range []string{"acme-7", "main", "Merge main into the branch and resolve the conflicts", "- README\n", "- a/b.go\n", "make typecheck", "no push and no PR", "finish the Ticket"} {
		if !strings.Contains(p, want) {
			t.Errorf("prompt %q lacks %q", p, want)
		}
	}
	for _, sk := range stageSkills {
		if sk.skill != "" && strings.Contains(p, sk.skill) {
			t.Errorf("prompt %q names the skill %q", p, sk.skill)
		}
	}
	if strings.HasPrefix(p, "/") {
		t.Errorf("prompt %q starts a slash command", p)
	}
	if got := firstPrompt(claudePrompts, c, api.ImplementationStage, "main", nil, false); got != "/implement acme-7" {
		t.Errorf("ordinary prompt %q", got)
	}
}

func TestUnderRebaseAConflictFirstPromptAsksForARebase(t *testing.T) {
	c := claim{Ticket: "acme-7", ClaimID: "C7"}
	p := firstPrompt(claudePrompts, c, api.ImplementationStage, "main", []string{"README"}, true)
	if want := "Rebase the branch onto main and resolve the conflicts in these files:\n\n- README\n"; !strings.Contains(p, want) {
		t.Errorf("prompt %q lacks %q", p, want)
	}
	if strings.Contains(p, "Merge main") {
		t.Errorf("prompt %q asks for a merge", p)
	}
	if got := firstPrompt(claudePrompts, c, api.ImplementationStage, "main", nil, true); got != "/implement acme-7" {
		t.Errorf("ordinary prompt %q", got)
	}
}

func TestUnderRebaseAConflictFirstPromptInSeveralRepositoriesAsksForARebaseInEach(t *testing.T) {
	c := claim{Ticket: "acme-7", ClaimID: "C7"}
	apiRepo := repositoryWorktree{r: &repository{name: "api"}, path: "/wt/api", base: "main", branch: "acme-7"}
	ws := worktreeSet{apiRepo}
	conflicts := []conflict{{w: apiRepo, files: []string{"endpoint.go"}}}
	p := severalFirstPrompt(claudePrompts, c, api.ImplementationStage, ws, conflicts, true)
	if want := "In each one's worktree, rebase the branch onto its base and resolve the conflicts in these files:\n\napi, base main:\n- endpoint.go\n"; !strings.Contains(p, want) {
		t.Errorf("prompt %q lacks %q", p, want)
	}
	if m := severalFirstPrompt(claudePrompts, c, api.ImplementationStage, ws, conflicts, false); !strings.Contains(m, "In each one's worktree, merge its base into the branch") {
		t.Errorf("merge prompt %q", m)
	}
}

func TestAConflictSystemPromptKeepsTheClaimAndSwapsTheGuideForAFinishInstruction(t *testing.T) {
	c := claim{Ticket: "acme-7", ClaimID: "C7"}
	g := api.StageGuideView{Stage: api.ImplementationStage, Guide: "THE GUIDE"}
	p := systemPrompt(claudePrompts, c, api.ImplementationStage, "/wt", g, []string{"README"})
	for _, want := range []string{"Ticket: acme-7\nClaim: C7\n", "Claim C7 is yours", "own worktree on branch acme-7", "`branch`", "`commit`"} {
		if !strings.Contains(p, want) {
			t.Errorf("system prompt %q lacks %q", p, want)
		}
	}
	if strings.Contains(p, "THE GUIDE") || strings.Contains(p, "The Stage Guide for") {
		t.Errorf("system prompt %q holds the Stage Guide", p)
	}
	if o := systemPrompt(claudePrompts, c, api.ImplementationStage, "/wt", g, nil); !strings.HasSuffix(o, "THE GUIDE") {
		t.Errorf("ordinary system prompt %q lost its guide", o)
	}
}

package factory

import (
	"strings"
	"testing"

	"github.com/tomoram/code-central/core"
)

func TestAConflictFirstPromptNamesTheFilesAndNoSkill(t *testing.T) {
	c := claim{Ticket: "acme-7", ClaimID: "C7"}
	p := firstPrompt(c, core.ImplementationStage, "main", []string{"README", "a/b.go"})
	for _, want := range []string{"acme-7", "main", "- README\n", "- a/b.go\n", "make typecheck", "no push and no PR", "finish the Ticket"} {
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
	if got := firstPrompt(c, core.ImplementationStage, "main", nil); got != "/implement acme-7" {
		t.Errorf("ordinary prompt %q", got)
	}
}

func TestAConflictSystemPromptKeepsTheClaimAndSwapsTheGuideForAFinishInstruction(t *testing.T) {
	c := claim{Ticket: "acme-7", ClaimID: "C7"}
	g := core.StageGuideView{Stage: core.ImplementationStage, Guide: "THE GUIDE"}
	p := systemPrompt(c, core.ImplementationStage, "/wt", g, []string{"README"})
	for _, want := range []string{"Ticket: acme-7\nClaim: C7\n", "Claim C7 is yours", "own worktree on branch acme-7", "`branch`", "`commit`"} {
		if !strings.Contains(p, want) {
			t.Errorf("system prompt %q lacks %q", p, want)
		}
	}
	if strings.Contains(p, "THE GUIDE") || strings.Contains(p, "The Stage Guide for") {
		t.Errorf("system prompt %q holds the Stage Guide", p)
	}
	if o := systemPrompt(c, core.ImplementationStage, "/wt", g, nil); !strings.HasSuffix(o, "THE GUIDE") {
		t.Errorf("ordinary system prompt %q lost its guide", o)
	}
}

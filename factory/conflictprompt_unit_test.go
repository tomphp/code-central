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
	p := firstPrompt(claudePrompts, c, api.ImplementationStage, "main", []string{"README", "a/b.go"}, false, "Run `make check` and the tests the resolution touches.")
	for _, want := range []string{"acme-7", "main", "Merge main into the branch and resolve the conflicts", "- README\n", "- a/b.go\n", "Run `make check` and the tests the resolution touches.", "no push and no PR", "finish the Ticket"} {
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
	if got := firstPrompt(claudePrompts, c, api.ImplementationStage, "main", nil, false, ""); got != "/implement acme-7" {
		t.Errorf("ordinary prompt %q", got)
	}
}

func TestUnderRebaseAConflictFirstPromptAsksForARebase(t *testing.T) {
	c := claim{Ticket: "acme-7", ClaimID: "C7"}
	p := firstPrompt(claudePrompts, c, api.ImplementationStage, "main", []string{"README"}, true, "")
	if want := "Rebase the branch onto main and resolve the conflicts in these files:\n\n- README\n"; !strings.Contains(p, want) {
		t.Errorf("prompt %q lacks %q", p, want)
	}
	if strings.Contains(p, "Merge main") {
		t.Errorf("prompt %q asks for a merge", p)
	}
	if got := firstPrompt(claudePrompts, c, api.ImplementationStage, "main", nil, true, ""); got != "/implement acme-7" {
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

func TestTheCheckInstructionIsTheCommandOrTheRepositorysOwnDocs(t *testing.T) {
	set, none := &repository{}, &repository{}
	set.listedAs(api.RepositoryView{CheckCommand: "npm run check"})
	none.listedAs(api.RepositoryView{})
	if got, want := set.checkInstruction(), "Run `npm run check` and the tests the resolution touches."; got != want {
		t.Errorf("instruction %q, want %q", got, want)
	}
	if got, want := none.checkInstruction(), "Run the Repository's checks and the tests the resolution touches, as its own docs (such as CLAUDE.md or README) describe."; got != want {
		t.Errorf("instruction %q, want %q", got, want)
	}
}

func TestASeveralRepositoryConflictPromptEndsEachBlockWithItsOwnCheckInstruction(t *testing.T) {
	c := claim{Ticket: "acme-7", ClaimID: "C7"}
	web, apiRepo := &repository{name: "web"}, &repository{name: "api"}
	web.listedAs(api.RepositoryView{CheckCommand: "npm run check"})
	apiRepo.listedAs(api.RepositoryView{})
	webWt := repositoryWorktree{r: web, path: "/wt/web", base: "main", branch: "acme-7"}
	apiWt := repositoryWorktree{r: apiRepo, path: "/wt/api", base: "main", branch: "acme-7"}
	conflicts := []conflict{{w: webWt, files: []string{"a.ts"}}, {w: apiWt, files: []string{"b.go"}}}
	p := severalFirstPrompt(claudePrompts, c, api.ImplementationStage, worktreeSet{webWt, apiWt}, conflicts, false)
	for _, want := range []string{
		"web, base main:\n- a.ts\nRun `npm run check` and the tests the resolution touches.\n",
		"api, base main:\n- b.go\nRun the Repository's checks and the tests the resolution touches, as its own docs (such as CLAUDE.md or README) describe.\n",
		"\nResolve them keeping the intent of both sides. Commit on the branch in each worktree",
	} {
		if !strings.Contains(p, want) {
			t.Errorf("prompt %q lacks %q", p, want)
		}
	}
	if strings.Contains(p, "make typecheck") {
		t.Errorf("prompt %q names make typecheck", p)
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

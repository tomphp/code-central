package factory_test

import (
	"context"
	"strings"
	"testing"

	"github.com/tomoram/code-central/api"
	"github.com/tomoram/code-central/domain"
	"github.com/tomoram/code-central/factory/factorytest"
)

// callFor is the one claude call the Factory made for the Ticket. A test
// settles the Factory first.
func callFor(t *testing.T, h *factorytest.Harness, id string) factorytest.Call {
	t.Helper()
	var found []factorytest.Call
	for _, c := range h.Calls() {
		if c.Ticket == id {
			found = append(found, c)
		}
	}
	if len(found) != 1 {
		t.Fatalf("%s: %d claude calls, want 1", id, len(found))
	}
	return found[0]
}

func requireConflictClaim(t *testing.T, c factorytest.Call, files ...string) {
	t.Helper()
	if strings.HasPrefix(c.Prompt, "/") || strings.Contains(c.Prompt, "implement") {
		t.Errorf("first prompt %q starts a skill", c.Prompt)
	}
	for _, f := range files {
		if !strings.Contains(c.Prompt, f) {
			t.Errorf("first prompt %q doesn't name %s", c.Prompt, f)
		}
	}
	if strings.Contains(c.SystemPrompt, "The Stage Guide for") || strings.Contains(c.SystemPrompt, "Stage Guide: implementation") {
		t.Errorf("system prompt %q holds the implementation Stage Guide", c.SystemPrompt)
	}
	if !strings.Contains(c.SystemPrompt, "Claim: "+c.Claim) || !strings.Contains(c.SystemPrompt, "Conflict Claim:") || !strings.Contains(c.SystemPrompt, "`commit`") {
		t.Errorf("system prompt %q lacks the Claim paragraph or the finish instruction", c.SystemPrompt)
	}
}

func requireOrdinaryClaim(t *testing.T, h *factorytest.Harness, c factorytest.Call) {
	t.Helper()
	if want := "/implement " + c.Ticket; c.Prompt != want {
		t.Errorf("first prompt %q, want %q", c.Prompt, want)
	}
	if !strings.Contains(c.SystemPrompt, "The Stage Guide for implementation follows") {
		t.Errorf("system prompt %q lacks the Stage Guide", c.SystemPrompt)
	}
}

func TestATicketSentBackForAConflictIsClaimedWithAnInlinePrompt(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t, factorytest.FakeGit())
	id, _ := accepted(h, "Change the readme", "README", "theirs\n")
	h.CommitOn("main", "README", "ours\n")

	h.Script(id, factorytest.Wait())
	h.Start()

	requireConflictClaim(t, callFor(t, h, id), "README")
}

func conflictPromptWith(t *testing.T, setup func(h *factorytest.Harness)) string {
	t.Helper()
	h := factorytest.New(t, factorytest.FakeGit())
	id, _ := accepted(h, "Change the readme", "README", "theirs\n")
	h.CommitOn("main", "README", "ours\n")
	setup(h)
	h.Script(id, factorytest.Wait())
	h.Start()
	return callFor(t, h, id).Prompt
}

func TestAConflictPromptNamesTheRepositorysCheckCommandAndNotMakeTypecheck(t *testing.T) {
	t.Parallel()
	p := conflictPromptWith(t, func(h *factorytest.Harness) {
		cmd := "npm run check"
		if _, err := h.Human().EditRepository(context.Background(), h.Project, api.EditRepositoryInput{Name: h.Project, CheckCommand: &cmd}); err != nil {
			t.Fatal(err)
		}
	})
	if !strings.Contains(p, "Run `npm run check` and the tests the resolution touches.") || strings.Contains(p, "make typecheck") {
		t.Errorf("prompt %q", p)
	}
}

func TestAConflictPromptWithNoCheckCommandPointsToTheRepositorysDocs(t *testing.T) {
	t.Parallel()
	p := conflictPromptWith(t, func(*factorytest.Harness) {})
	if !strings.Contains(p, "as its own docs (such as CLAUDE.md or README) describe.") || strings.Contains(p, "make typecheck") || strings.Contains(p, "Run `") {
		t.Errorf("prompt %q", p)
	}
}

func TestATicketSentBackForAnotherReasonStartsAsAnOrdinaryClaim(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t, factorytest.FakeGit())
	id, _ := accepted(h, "Change the readme", "README", "theirs\n")
	if _, err := h.Human().SendBack(context.Background(), h.Project, api.SendBackInput{Ticket: api.Ref(id), Note: "Rename it."}); err != nil {
		t.Fatal(err)
	}

	h.Script(id, factorytest.Wait())
	h.Start()

	requireOrdinaryClaim(t, h, callFor(t, h, id))
}

func TestATicketWhoseBranchNoLongerConflictsStartsAsAnOrdinaryClaim(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t, factorytest.FakeGit())
	id, _ := accepted(h, "Change the readme", "README", "theirs\n")
	h.CommitOn("main", "OTHER", "ours\n")
	if _, err := h.Human().SendBack(context.Background(), h.Project, api.SendBackInput{Ticket: api.Ref(id), Note: "Merge the base."}); err != nil {
		t.Fatal(err)
	}

	h.Script(id, factorytest.Wait())
	h.Start()

	requireOrdinaryClaim(t, h, callFor(t, h, id))
}

func TestAFirstClaimIsAnOrdinaryClaim(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t, factorytest.FakeGit())
	id := h.ReadyTicket("Add a button")
	h.CommitOn("main", "README", "ours\n")

	h.Script(id, factorytest.Wait())
	h.Start()

	requireOrdinaryClaim(t, h, callFor(t, h, id))
}

func TestATicketOnASpecBranchIsAConflictClaimOnlyWhenItsBranchConflicts(t *testing.T) {
	t.Parallel()
	for _, conflicts := range []bool{true, false} {
		h := factorytest.New(t, factorytest.FakeGit())
		h.MakeRequirement()
		id := h.ReadyTicket("Merge the base into the Spec Branch")
		h.CommitOn(id, "README", "theirs\n")
		file := "OTHER"
		if conflicts {
			file = "README"
		}
		h.CommitOn("main", file, "ours\n")

		h.Script(id, factorytest.Wait())
		h.Start()

		c := callFor(t, h, id)
		if conflicts {
			requireConflictClaim(t, c, "README")
		} else {
			requireOrdinaryClaim(t, h, c)
		}
	}
}

func TestTheConflictTicketTheFactoryOpensForASpecBranchStartsAsAConflictClaim(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t, factorytest.FakeGit())
	h.MakeRequirement()
	a, _ := accepted(h, "First", "README", "spec side\n")
	release := holdTicketsStage(t, h)
	fix := "acme-5"
	h.Script(fix, factorytest.Wait())
	r := h.Start()
	r.WantOutput(a + ": merged into spec/" + h.SpecID())

	h.CommitOn("main", "README", "base side\n")
	release()
	r.Poll()
	r.WantOutput(h.SpecID() + ": spec/" + h.SpecID() + " conflicts with main in README: opened " + fix)

	requireConflictClaim(t, callFor(t, h, fix), "README")
}

func conflictClaimFinishing(t *testing.T, h *factorytest.Harness) api.TicketView {
	t.Helper()
	id, _ := accepted(h, "Change the readme", "README", "theirs\n")
	h.CommitOn("main", "README", "ours\n")
	h.Script(id, append(commitFile("main.go"), finishWithBranch("Resolved."))...)
	h.Start()
	return h.WantState(id, domain.InReview)
}

func TestAConflictClaimFinishingWithCodeChangesAndNoCodeReviewGetsNoComment(t *testing.T) {
	t.Parallel()
	tk := conflictClaimFinishing(t, factorytest.New(t, factorytest.FakeGit()))
	if reviewSkippedComment(tk) {
		t.Errorf("a Conflict Claim got a review comment: %+v", tk.Comments)
	}
}

func TestAConflictClaimUnderRebaseFinishingWithNoCodeReviewGetsNoComment(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t, factorytest.FakeGit())
	setRebase(h, true)
	tk := conflictClaimFinishing(t, h)
	if reviewSkippedComment(tk) {
		t.Errorf("a Conflict Claim under Rebase got a review comment: %+v", tk.Comments)
	}
}

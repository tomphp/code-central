package factory_test

import (
	"context"
	"strings"
	"testing"

	"github.com/tomoram/code-central/core"
	"github.com/tomoram/code-central/factory/factorytest"
)

// callFor is the one claude call the Factory made for the Ticket.
func callFor(t *testing.T, h *factorytest.Harness, id string) factorytest.Call {
	t.Helper()
	var found []factorytest.Call
	h.WaitFor("claude to start on "+id, func() bool {
		found = nil
		for _, c := range h.Calls() {
			if c.Ticket == id {
				found = append(found, c)
			}
		}
		return len(found) > 0
	})
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
	h := factorytest.New(t)
	id, _ := accepted(h, "Change the readme", "README", "theirs\n")
	h.CommitOn("main", "README", "ours\n")

	h.Script(id, factorytest.Wait())
	h.Start()
	h.WaitForWorktree(id)

	requireConflictClaim(t, callFor(t, h, id), "README")
}

func TestATicketSentBackForAnotherReasonStartsAsAnOrdinaryClaim(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t)
	id, _ := accepted(h, "Change the readme", "README", "theirs\n")
	if _, err := h.Human().SendBack(context.Background(), h.Project, core.SendBackInput{Ticket: core.Ref(id), Note: "Rename it."}); err != nil {
		t.Fatal(err)
	}

	h.Script(id, factorytest.Wait())
	h.Start()
	h.WaitForWorktree(id)

	requireOrdinaryClaim(t, h, callFor(t, h, id))
}

func TestATicketWhoseBranchNoLongerConflictsStartsAsAnOrdinaryClaim(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t)
	id, _ := accepted(h, "Change the readme", "README", "theirs\n")
	h.CommitOn("main", "OTHER", "ours\n")
	if _, err := h.Human().SendBack(context.Background(), h.Project, core.SendBackInput{Ticket: core.Ref(id), Note: "Merge the base."}); err != nil {
		t.Fatal(err)
	}

	h.Script(id, factorytest.Wait())
	h.Start()
	h.WaitForWorktree(id)

	requireOrdinaryClaim(t, h, callFor(t, h, id))
}

func TestAFirstClaimIsAnOrdinaryClaim(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t)
	id := h.ReadyTicket("Add a button")
	h.CommitOn("main", "README", "ours\n")

	h.Script(id, factorytest.Wait())
	h.Start()
	h.WaitForWorktree(id)

	requireOrdinaryClaim(t, h, callFor(t, h, id))
}

func TestATicketOnASpecBranchIsAConflictClaimOnlyWhenItsBranchConflicts(t *testing.T) {
	t.Parallel()
	for _, conflicts := range []bool{true, false} {
		h := factorytest.New(t)
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
		h.WaitForWorktree(id)

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
	h := factorytest.New(t)
	h.MakeRequirement()
	a, _ := accepted(h, "First", "README", "spec side\n")
	release := holdTicketsStage(t, h)
	fix := "acme-5"
	h.Script(fix, factorytest.Wait())
	r := h.Start()
	r.WaitForOutput(a + ": merged into spec/" + h.SpecID())

	h.CommitOn("main", "README", "base side\n")
	release()
	r.Poll()
	r.WaitForOutput(h.SpecID() + ": spec/" + h.SpecID() + " conflicts with main in README: opened " + fix)

	requireConflictClaim(t, callFor(t, h, fix), "README")
}

package workitemwatcher_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/tomoram/code-central/api"
	"github.com/tomoram/code-central/domain"
)

// Azure Boards' Branch format (Spec code-central-1089): To Requirement
// names the Requirement's Spec Branch from the work item's ID.

// setFormat is the human saving Azure Boards, on for queryID, with the
// Branch format format.
func (w *watching) setFormat(format string) error {
	_, err := w.srv.Core.SetAzureBoards(human, "acme", api.SetAzureBoardsInput{On: true, QueryURL: w.queryURL(queryID), BranchFormat: format})
	return err
}

// branchTag is Spec spec's branch Tag, and whether it has one.
func (w *watching) branchTag(spec string) (string, bool) {
	w.t.Helper()
	sp, err := w.srv.Core.GetSpec(context.Background(), "acme", api.GetSpecInput{Spec: api.Ref(spec)})
	if err != nil {
		w.t.Fatal(err)
	}
	name, ok := sp.Tags[domain.BranchTag]
	return name, ok
}

func TestTheBranchFormatIsEmptyByDefaultAndSavedWithAzureBoards(t *testing.T) {
	w := newWatching(t)
	if got := w.settings().BranchFormat; got != "" {
		t.Fatalf("Branch format = %q, want empty by default", got)
	}
	if err := w.setFormat("feat/{id}"); err != nil {
		t.Fatal(err)
	}
	if got := w.settings().BranchFormat; got != "feat/{id}" {
		t.Fatalf("Branch format = %q, want feat/{id}", got)
	}
	if err := w.setFormat(""); err != nil {
		t.Fatal(err)
	}
	if got := w.settings().BranchFormat; got != "" {
		t.Fatalf("Branch format = %q, want it cleared", got)
	}
}

func TestABranchFormatThatCantNameASpecBranchIsRefused(t *testing.T) {
	w := newWatching(t)
	for format, says := range map[string]string{
		"feat/4711":         "{id}",
		"feat/{id}-{title}": "{title}",
		"feat/{ID}":         "must contain {id}",
		"{id}/{ID}":         "{ID}",
		"feat/{id}}":        "only use { and } around {id}",
		"feat/{{id}":        "only use { and } around {id}",
		"feat {id}":         "valid branch name",
		"spec/{id}":         "starts with spec/",
		"acme-{id}":         "shape of a Ticket ID",
		"{id}/fix":          "",
		"users/me/{id}":     "",
	} {
		err := w.setFormat(format)
		if says == "" {
			if err != nil {
				t.Errorf("%q: %v, want it saved", format, err)
			}
			continue
		}
		var e *api.Error
		if !errors.As(err, &e) || e.Code != api.Invalid || e.Field != "branch_format" || !strings.Contains(e.Message, says) {
			t.Errorf("%q: err = %v, want branch_format refused saying %q", format, err, says)
		}
	}
}

func TestChangingTheBranchFormatLeavesTheWorkItemWatcherAsItIs(t *testing.T) {
	w := newWatching(t)
	w.arrived(1)
	if err := w.setFormat("feat/{id}"); err != nil {
		t.Fatal(err)
	}
	if got := w.settings().Watcher.State; got != api.WatcherWatching {
		t.Fatalf("Work Item Watcher %s after the Branch format changed, want still watching", got)
	}
}

func TestToRequirementSetsTheBranchTagFromTheBranchFormat(t *testing.T) {
	w := newWatching(t)
	w.arrived(1, 2)
	spec := w.toRequirement(1, "")
	if name, ok := w.branchTag(spec); ok {
		t.Fatalf("with no Branch format, branch Tag %q, want none", name)
	}
	if err := w.setFormat("feat/{id}"); err != nil {
		t.Fatal(err)
	}
	spec = w.toRequirement(2, "")
	if name, _ := w.branchTag(spec); name != "feat/2" {
		t.Fatalf("branch Tag %q, want feat/2", name)
	}
	tk, err := w.srv.Core.GetTicket(context.Background(), "acme", api.GetTicketInput{Ticket: api.Ref(w.ticketOf(2).Replacements[0])})
	if err != nil {
		t.Fatal(err)
	}
	if tk.Spec.Branch != "feat/2" {
		t.Fatalf("the grilling Stage's Spec Branch %q, want feat/2", tk.Spec.Branch)
	}
}

func TestToRequirementWhoseBranchIsTakenIsRefusedWithNothingWritten(t *testing.T) {
	w := newWatching(t)
	w.arrived(1)
	other, err := w.srv.Core.CreateSpec(human, "acme", api.CreateSpecInput{Title: "Dark mode", Full: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.srv.Core.SetTags(human, "acme", api.SetTagsInput{Target: api.Ref(other.Record.ID), Tags: map[string]string{domain.BranchTag: "feat/1"}}); err != nil {
		t.Fatal(err)
	}
	if err := w.setFormat("feat/{id}"); err != nil {
		t.Fatal(err)
	}
	before := len(w.linked())
	_, err = w.srv.Core.ToRequirement(human, "acme", api.ToRequirementInput{Ticket: api.Ref(w.ticketOf(1).ID), Context: "Seen on every login."})
	wantSaid := []string{"feat/1", other.Record.ID, "Dark mode", "branch Tag", "Branch format"}
	for _, s := range wantSaid {
		if err == nil || !strings.Contains(err.Error(), s) {
			t.Errorf("err = %v, want it to say %q", err, s)
		}
	}
	if got := len(w.linked()); got != before {
		t.Fatalf("%d linked items, want %d: no Requirement made", got, before)
	}
	if got := w.ticketOf(1); got.State != domain.Draft || got.Link.Detached || len(got.Comments) != 0 {
		t.Fatalf("Incoming Issue %+v, want it Draft and linked as it was", got)
	}
}

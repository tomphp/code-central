package core_test

import (
	"slices"
	"testing"

	"github.com/tomoram/code-central/core"
	"github.com/tomoram/code-central/events"
	"github.com/tomoram/code-central/store"
)

func TestTheStoresNeedsMeReasonsAreCoresInGroupOrder(t *testing.T) {
	w, err := newFixture(t).core.GetWorkflow(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var reasons []string
	for _, r := range w.NeedsMe {
		reasons = append(reasons, string(r.Reason))
	}
	if !slices.Equal(store.NeedsMeReasons, reasons) {
		t.Fatalf("store.NeedsMeReasons = %v, want core's %v", store.NeedsMeReasons, reasons)
	}
}

func TestSetFocusKindsSendsTheProjectsNoticeOnlyOnAChange(t *testing.T) {
	f := newFixture(t)
	f.project(t, "acme")
	notices, stop := f.core.Events().Subscribe()
	defer stop()

	w, err := f.core.SetFocusKinds(human, "acme", core.SetFocusKindsInput{Off: []core.NeedsMeReason{core.ReasonAnswered}})
	if err != nil {
		t.Fatal(err)
	}
	if w.Record != nil || w.ID != "acme" || !slices.Equal(w.Changed, []string{"focus_kinds"}) {
		t.Errorf("acknowledgement = %+v, want focus_kinds changed on acme", w)
	}
	if got := <-notices; got != (events.Event{Type: events.ProjectChanged, Project: "acme"}) {
		t.Fatalf("got %+v, want the Project's notice", got)
	}
	noNotice(t, notices)
	if _, err := f.core.SetFocusKinds(human, "acme", core.SetFocusKindsInput{Off: []core.NeedsMeReason{core.ReasonAnswered, core.ReasonAnswered}}); err != nil {
		t.Fatal(err)
	}
	noNotice(t, notices)
}

func TestAnAgentCannotSetFocusKinds(t *testing.T) {
	f := newFixture(t)
	f.project(t, "acme")
	_, err := f.core.SetFocusKinds(ctx, "acme", core.SetFocusKindsInput{Off: []core.NeedsMeReason{core.ReasonAnswered}})
	wantCode(t, err, core.ActionNotAllowed, "")
}

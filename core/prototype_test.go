package core_test

import (
	"slices"
	"testing"

	"github.com/tomoram/code-central/core"
	"github.com/tomoram/code-central/store"
)

const themePicker = "# A theme picker in the header\n\nWould a picker beat a toggle?\n"

func TestSubmittingAPrototypeCreatesAReadyPrototypeTicketOnTheSpec(t *testing.T) {
	f := newFixture(t)
	f.project(t, "acme")
	f.spec(t, "acme", "Dark mode") // acme-1
	res, err := f.core.SubmitPrototype(ctx, "acme", core.SubmitPrototypeInput{Spec: "acme-1", Text: themePicker})
	if err != nil {
		t.Fatalf("submit_prototype: %v", err)
	}
	tk, err := f.core.GetTicket(ctx, "acme", whole("acme-2"))
	if err != nil {
		t.Fatalf("get_ticket: %v", err)
	}
	if res.ID != "acme-2" || tk.Title != "Prototype: A theme picker in the header" || tk.Description != themePicker ||
		tk.State != store.Ready || tk.Spec.ID != "acme-1" || !slices.Equal(tk.Labels, []string{"prototype"}) {
		t.Fatalf("prototype %+v (result %+v)", tk, res)
	}
	if len(tk.Moves) != 1 || tk.Moves[0].Actor != "human" || tk.Moves[0].To != store.Ready || tk.Moves[0].Via != "submit_prototype" {
		t.Fatalf("moves %+v", tk.Moves)
	}
	if !tk.Claimable {
		t.Fatal("a Prototype should be claimable straight away")
	}
}

func TestAPrototypeMayBeOnARequirement(t *testing.T) {
	f := newFixture(t)
	f.project(t, "acme")
	if _, err := f.core.SubmitRequirement(ctx, "acme", core.SubmitRequirementInput{Text: darkMode}); err != nil {
		t.Fatal(err)
	}
	res, err := f.core.SubmitPrototype(ctx, "acme", core.SubmitPrototypeInput{Spec: "1", Text: themePicker, Full: true})
	if err != nil {
		t.Fatalf("submit_prototype: %v", err)
	}
	if res.Record.Spec.ID != "acme-1" || !slices.Equal(res.Record.Labels, []string{"prototype"}) {
		t.Fatalf("prototype %+v", res.Record)
	}
}

func TestSubmittingAPrototypeAcknowledgesItsTicket(t *testing.T) {
	f := newFixture(t)
	f.project(t, "acme")
	f.spec(t, "acme", "Dark mode")
	res, err := f.core.SubmitPrototype(ctx, "acme", core.SubmitPrototypeInput{Spec: "acme-1", Text: themePicker})
	if err != nil {
		t.Fatalf("submit_prototype: %v", err)
	}
	wantTicketAcknowledgement(t, res, "acme-2", t0, store.Ready,
		[]string{"title", "state", "spec", "labels", "description", "moves", "claimable"}, 0)
}

func TestAPrototypeNeedsTextAndAKnownSpec(t *testing.T) {
	f := newFixture(t)
	f.project(t, "acme")
	f.spec(t, "acme", "Dark mode")
	for _, text := range []string{"", "  \n\t\n"} {
		_, err := f.core.SubmitPrototype(ctx, "acme", core.SubmitPrototypeInput{Spec: "acme-1", Text: text})
		wantCode(t, err, core.Invalid, "text")
	}
	_, err := f.core.SubmitPrototype(ctx, "acme", core.SubmitPrototypeInput{Text: themePicker})
	wantCode(t, err, core.Invalid, "spec")
	_, err = f.core.SubmitPrototype(ctx, "acme", core.SubmitPrototypeInput{Spec: "acme-9", Text: themePicker})
	wantCode(t, err, core.Invalid, "spec")
	f.store.View(func(r store.Reader) error {
		if len(r.Tickets("acme")) != 0 {
			t.Fatal("a rejected Prototype wrote something")
		}
		return nil
	})
}

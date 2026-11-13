package core_test

import (
	"slices"
	"testing"

	"github.com/tomoram/code-central/core"
	"github.com/tomoram/code-central/store"
)

const fixTypo = "Fix the typo in the footer\n\nIt says \"Copyrigth\".\n"

func submitSmallChange(t *testing.T, f fixture, text string) core.Written[core.TicketView] {
	t.Helper()
	res, err := f.core.SubmitSmallChange(ctx, "acme", core.SubmitSmallChangeInput{Text: text, Full: true})
	if err != nil {
		t.Fatalf("submit_small_change: %v", err)
	}
	return res
}

func TestTheFirstSmallChangeCreatesALongLivedSmallChangesSpecAndAReadyTicketOnIt(t *testing.T) {
	f := newFixture(t)
	f.project(t, "acme")
	tk := submitSmallChange(t, f, fixTypo).Record
	if tk.ID != "acme-2" || tk.Title != "Fix the typo in the footer" || tk.Description != fixTypo ||
		tk.State != store.Ready || tk.Spec.ID != "acme-1" || len(tk.Labels) != 0 || !tk.Claimable {
		t.Fatalf("Small Change %+v", tk)
	}
	if len(tk.Moves) != 1 || tk.Moves[0].Actor != "human" || tk.Moves[0].To != store.Ready || tk.Moves[0].Via != "submit_small_change" {
		t.Fatalf("moves %+v", tk.Moves)
	}
	sp, err := f.core.GetSpec(ctx, "acme", core.GetSpecInput{Spec: "acme-1"})
	if err != nil {
		t.Fatal(err)
	}
	if sp.Title != "Small Changes" || !slices.Equal(sp.Labels, []string{core.LongLivedLabel}) {
		t.Fatalf("Spec %+v", sp)
	}
}

func TestASecondSmallChangeReusesTheSpecEvenAfterItIsRetitled(t *testing.T) {
	f := newFixture(t)
	f.project(t, "acme")
	submitSmallChange(t, f, fixTypo)
	if _, err := f.core.EditSpec(ctx, "acme", core.EditSpecInput{Spec: "acme-1", Title: ptr("Odd jobs")}); err != nil {
		t.Fatal(err)
	}
	if tk := submitSmallChange(t, f, "Rename the label").Record; tk.Spec.ID != "acme-1" || tk.ID != "acme-2" && tk.ID != "acme-3" {
		t.Fatalf("second Small Change %+v", tk)
	}
	if specs, err := f.core.FindSpecs(ctx, "acme", core.FindSpecsInput{}); err != nil || len(specs) != 1 {
		t.Fatalf("specs %+v, %v", specs, err)
	}
}

func TestASmallChangeIsWrittenAsHumanWhoeverCalls(t *testing.T) {
	f := newFixture(t)
	f.project(t, "acme")
	res, err := f.core.SubmitSmallChange(ctx,"acme", core.SubmitSmallChangeInput{Text: "Fix it", Full: true})
	if err != nil {
		t.Fatal(err)
	}
	if m := res.Record.Moves; len(m) != 1 || m[0].Actor != "human" {
		t.Fatalf("moves %+v", m)
	}
}

func TestASmallChangeWithNoTextIsInvalid(t *testing.T) {
	f := newFixture(t)
	f.project(t, "acme")
	_, err := f.core.SubmitSmallChange(ctx, "acme", core.SubmitSmallChangeInput{Text: " \n "})
	wantCode(t, err, core.Invalid, "text")
}

func TestAnAgentClaimsASmallChangeWithTheImplementationGuide(t *testing.T) {
	f := newFixture(t)
	f.project(t, "acme")
	submitSmallChange(t, f, fixTypo)
	res, err := f.core.Claim(ctx, "acme", core.ClaimInput{})
	if err != nil {
		t.Fatal(err)
	}
	if res.Ticket == nil || res.StageGuide == nil || res.StageGuide.Stage != "implementation" || res.StageGuide.Guide != guide(t, f, "implementation") {
		t.Fatalf("claim carried %+v", res)
	}
}

func TestASmallChangeLandsOnItsOwnUnderLandPerSpec(t *testing.T) {
	f := landWorld(t, true)
	tk := submitSmallChange(t, f, fixTypo).Record
	if f.getTicket(t, tk.ID).SpecLanded {
		t.Error("a Small Change is Spec-landed")
	}
	if f.specStatus(t, core.Ref(tk.Spec.ID)) != nil {
		t.Error("the Small Changes Spec has a Land per Spec status")
	}
}

package core_test

import (
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/tomoram/code-central/api"
)

// The long-lived Label is reserved for Specs (ADR 0031).

func TestLongLivedIsRefusedOnATicketAndAllowedOnASpec(t *testing.T) {
	f := newFixture(t)
	f.project(t, "acme")
	f.spec(t, "acme", "Login") // acme-1
	tk := f.ticket(t, "acme", api.CreateTicketInput{Spec: "1", Title: "Form"})

	wantInvalid := func(what string, err error) {
		t.Helper()
		var e *api.Error
		if !errors.As(err, &e) || e.Code != "invalid" || !strings.Contains(e.Message, "goes on a Spec") {
			t.Errorf("%s: err = %v, want invalid saying it goes on a Spec", what, err)
		}
	}
	_, err := f.core.CreateTicket(ctx, "acme", api.CreateTicketInput{Spec: "1", Title: "Other", Labels: []string{"long-lived"}})
	wantInvalid("create_ticket", err)
	_, err = f.core.AddLabels(ctx, "acme", api.LabelsInput{Target: api.Ref(tk.ID), Labels: []string{"long-lived"}})
	wantInvalid("add_labels on a Ticket", err)

	got, err := f.core.AddLabels(ctx, "acme", api.LabelsInput{Target: "acme-1", Labels: []string{"long-lived"}})
	if err != nil || !slices.Contains(got.Labels, "long-lived") {
		t.Fatalf("add_labels on a Spec = %+v, %v", got, err)
	}
	got, err = f.core.RemoveLabels(ctx, "acme", api.LabelsInput{Target: "acme-1", Labels: []string{"long-lived"}})
	if err != nil || slices.Contains(got.Labels, "long-lived") {
		t.Fatalf("remove_labels on a Spec = %+v, %v", got, err)
	}
}

func TestACIRulesSpecIsCreatedLongLived(t *testing.T) {
	f := newFixture(t)
	f.project(t, "acme")
	if _, err := f.core.AddCIRule(human, "acme", api.CIRuleInput{Workflow: "ci.yml", Branch: "main"}); err != nil {
		t.Fatal(err)
	}
	sp, err := f.core.GetSpec(ctx, "acme", api.GetSpecInput{Spec: "1"})
	if err != nil || !slices.Contains(sp.Labels, "long-lived") {
		t.Fatalf("the CI Rule's Spec = %+v, %v; want the long-lived Label", sp, err)
	}
}

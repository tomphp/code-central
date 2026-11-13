package core_test

import (
	"errors"
	"testing"

	"github.com/tomoram/code-central/core"
	"github.com/tomoram/code-central/store"
)

// Auto-accept (Spec code-central-536, ADR 0037): the Project's setting
// per Auto-accept kind, set by the human alone, and finish carrying out
// the human's accept when the finished Ticket is eligible.

// projectView reads the Project as the UI does.
func (f fixture) projectView(t *testing.T, key string) core.ProjectView {
	t.Helper()
	p, err := f.core.GetProject(ctx, key)
	if err != nil {
		t.Fatalf("get_project %s: %v", key, err)
	}
	return p
}

// setAutoAccept turns the given kinds on or off on acme as the human.
func (f fixture) setAutoAccept(t *testing.T, kinds map[store.AutoAcceptKind]bool) core.ProjectView {
	t.Helper()
	v, err := f.core.SetAutoAccept(human, "acme", core.SetAutoAcceptInput{Kinds: kinds, Full: true})
	if err != nil {
		t.Fatalf("set_auto_accept: %v", err)
	}
	return *v.Record
}

func TestSetAutoAcceptTurnsKindsOnAndOffAndTheProjectReadsThem(t *testing.T) {
	f := newFixture(t)
	f.project(t, "acme")
	if p := f.projectView(t, "acme"); len(p.AutoAccept) != 5 || p.AutoAccept["implementation"] {
		t.Fatalf("a new Project's auto_accept = %v, want every kind off", p.AutoAccept)
	}

	p := f.setAutoAccept(t, map[store.AutoAcceptKind]bool{"implementation": true, "merge": true})
	if !p.AutoAccept["implementation"] || !p.AutoAccept["merge"] || p.AutoAccept["grilling"] || p.AutoAccept["spec"] || p.AutoAccept["tickets"] {
		t.Fatalf("auto_accept = %v, want implementation and merge on", p.AutoAccept)
	}

	// A kind left out keeps its value.
	p = f.setAutoAccept(t, map[store.AutoAcceptKind]bool{"merge": false, "grilling": true})
	if !p.AutoAccept["implementation"] || p.AutoAccept["merge"] || !p.AutoAccept["grilling"] {
		t.Fatalf("auto_accept = %v, want implementation and grilling on", p.AutoAccept)
	}
	if got := f.projectView(t, "acme").AutoAccept; got["implementation"] != true || got["grilling"] != true || got["merge"] {
		t.Fatalf("get_project auto_accept = %v", got)
	}
}

func TestSetAutoAcceptAcknowledgesTheSettingAndWritesNothingWhenUnchanged(t *testing.T) {
	f := newFixture(t)
	f.project(t, "acme")
	v, err := f.core.SetAutoAccept(human, "acme", core.SetAutoAcceptInput{Kinds: map[store.AutoAcceptKind]bool{"tickets": true}})
	if err != nil || v.ID != "acme" || len(v.Changed) != 1 || v.Changed[0] != "auto_accept" {
		t.Fatalf("acknowledgement %+v, %v", v, err)
	}
	v, err = f.core.SetAutoAccept(human, "acme", core.SetAutoAcceptInput{Kinds: map[store.AutoAcceptKind]bool{"tickets": true}})
	if err != nil || len(v.Changed) != 0 {
		t.Fatalf("acknowledgement of no change %+v, %v", v, err)
	}
}

func TestSetAutoAcceptIsTheHumansAloneAndRefusesAnUnknownKind(t *testing.T) {
	f := newFixture(t)
	f.project(t, "acme")
	for _, actor := range []core.Actor{core.Agent, core.Integration} {
		_, err := f.core.SetAutoAccept(core.WithActor(ctx, actor), "acme", core.SetAutoAcceptInput{Kinds: map[store.AutoAcceptKind]bool{"implementation": true}})
		var e *core.Error
		if !errors.As(err, &e) || e.Code != core.ActionNotAllowed {
			t.Fatalf("as %s: %v, want action_not_allowed", actor, err)
		}
	}
	_, err := f.core.SetAutoAccept(human, "acme", core.SetAutoAcceptInput{Kinds: map[store.AutoAcceptKind]bool{"prototype": true}})
	var e *core.Error
	if !errors.As(err, &e) || e.Code != core.Invalid || e.Field != "kinds" {
		t.Fatalf("unknown kind: %v, want invalid on kinds", err)
	}
	if p := f.projectView(t, "acme"); p.AutoAccept["implementation"] {
		t.Fatalf("auto_accept = %v, want nothing on after refusals", p.AutoAccept)
	}
}

package core_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/tomoram/code-central/api"
	"github.com/tomoram/code-central/domain"
)

// Auto-fix (Spec code-central-1022): the Project's setting, set by the
// human alone, reading as the defaults until it is set.

func (f fixture) trackCI(t *testing.T) {
	t.Helper()
	if _, err := f.core.SetCITracking(human, "acme", api.SetCITrackingInput{Tracking: true, Repository: "acme/web"}); err != nil {
		t.Fatalf("set_ci_tracking: %v", err)
	}
}

func TestAProjectWithoutAnAutoFixEntryReadsTheDefaults(t *testing.T) {
	f := newFixture(t)
	f.project(t, "acme")
	af := f.projectView(t, "acme").CI.AutoFix
	if af.On || af.FixInstructions != domain.DefaultFixInstructions || af.DefaultFixInstructions != domain.DefaultFixInstructions || af.Limit != 3 {
		t.Fatalf("auto_fix = %+v, want off, the default Fix instructions and limit 3", af)
	}
	if !strings.HasPrefix(af.FixInstructions, "Work out what kind of failure this is before changing anything") {
		t.Fatalf("default Fix instructions = %q", af.FixInstructions)
	}
}

func TestSetAutoFixSavesTheEntryAndTheProjectReadsItBack(t *testing.T) {
	f := newFixture(t)
	f.project(t, "acme")
	f.trackCI(t)
	w, err := f.core.SetAutoFix(human, "acme", api.SetAutoFixInput{On: true, FixInstructions: "Be careful.", Limit: 10, Full: true})
	if err != nil {
		t.Fatalf("set_auto_fix: %v", err)
	}
	want := api.AutoFixView{On: true, FixInstructions: "Be careful.", DefaultFixInstructions: domain.DefaultFixInstructions, Limit: 10}
	if got := w.Record.CI.AutoFix; got != want {
		t.Fatalf("saved auto_fix = %+v, want %+v", got, want)
	}
	if got := f.projectView(t, "acme").CI.AutoFix; got != want {
		t.Fatalf("get_project auto_fix = %+v, want %+v", got, want)
	}

	// Saving the default text reads as the default again.
	if _, err := f.core.SetAutoFix(human, "acme", api.SetAutoFixInput{On: true, FixInstructions: domain.DefaultFixInstructions, Limit: 3}); err != nil {
		t.Fatal(err)
	}
	if got := f.projectView(t, "acme").CI.AutoFix; got.FixInstructions != domain.DefaultFixInstructions || got.Limit != 3 || !got.On {
		t.Fatalf("auto_fix = %+v", got)
	}
}

func TestSetAutoFixRefusals(t *testing.T) {
	f := newFixture(t)
	f.project(t, "acme")
	ok := api.SetAutoFixInput{On: false, FixInstructions: "Fix it.", Limit: 3}

	if _, err := f.core.SetAutoFix(api.WithActor(ctx, api.Agent), "acme", ok); !isCode(err, api.ActionNotAllowed) {
		t.Errorf("as an agent: %v, want action_not_allowed", err)
	}
	if _, err := f.core.SetAutoFix(api.WithActor(ctx, api.Integration), "acme", ok); !isCode(err, api.ActionNotAllowed) {
		t.Errorf("as an integration: %v, want action_not_allowed", err)
	}
	for _, limit := range []int{0, 11, -1} {
		in := ok
		in.Limit = limit
		if _, err := f.core.SetAutoFix(human, "acme", in); !isCode(err, api.Invalid) {
			t.Errorf("limit %d: %v, want invalid", limit, err)
		}
	}
	for _, text := range []string{"", "  \n"} {
		in := ok
		in.FixInstructions = text
		if _, err := f.core.SetAutoFix(human, "acme", in); !isCode(err, api.Invalid) {
			t.Errorf("Fix instructions %q: %v, want invalid", text, err)
		}
	}
	on := ok
	on.On = true
	if _, err := f.core.SetAutoFix(human, "acme", on); !isCode(err, api.Invalid) || !strings.Contains(err.Error(), "CI Tracking") {
		t.Errorf("on with CI Tracking off: %v, want invalid naming CI Tracking", err)
	}
	if af := f.projectView(t, "acme").CI.AutoFix; af.On || af.Limit != 3 {
		t.Errorf("a refused write changed the setting: %+v", af)
	}
}

func TestTurningCITrackingOffLeavesTheAutoFixEntryAlone(t *testing.T) {
	f := newFixture(t)
	f.project(t, "acme")
	f.trackCI(t)
	if _, err := f.core.SetAutoFix(human, "acme", api.SetAutoFixInput{On: true, FixInstructions: "Mine.", Limit: 7}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.core.SetCITracking(human, "acme", api.SetCITrackingInput{Tracking: false}); err != nil {
		t.Fatal(err)
	}
	if af := f.projectView(t, "acme").CI.AutoFix; !af.On || af.FixInstructions != "Mine." || af.Limit != 7 {
		t.Fatalf("auto_fix = %+v, want the entry kept", af)
	}
}

// isCode reports whether err is a core Error with code.
func isCode(err error, code api.Code) bool {
	var e *api.Error
	return errors.As(err, &e) && e.Code == code
}

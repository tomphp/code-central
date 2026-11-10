package factory_test

import (
	"strings"
	"testing"

	"github.com/tomoram/code-central/core"
	"github.com/tomoram/code-central/factory/factorytest"
	"github.com/tomoram/code-central/store"
)

// wantFailureReport waits for id to fail and checks its one Failure
// Report says each of what it must.
func wantFailureReport(t *testing.T, h *factorytest.Harness, id string, broke, fix []string) {
	t.Helper()
	tk := h.WaitForState(id, store.Failed)
	if len(tk.FailureReports) != 1 {
		t.Fatalf("%d Failure Reports, want 1", len(tk.FailureReports))
	}
	fr := tk.FailureReports[0]
	for _, s := range broke {
		if !strings.Contains(fr.WhatBroke, s) {
			t.Errorf("what_broke %q: want it to say %q", fr.WhatBroke, s)
		}
	}
	for _, s := range fix {
		if !strings.Contains(fr.WhatToFix, s) {
			t.Errorf("what_to_fix %q: want it to say %q", fr.WhatToFix, s)
		}
	}
	if !strings.Contains(fr.WhereWorkStands, h.Worktree(id)) {
		t.Errorf("where_work_stands %q: want it to name the worktree", fr.WhereWorkStands)
	}
}

func TestAModelClaudeRejectsFailsTheClaimNamingWhereItWasSet(t *testing.T) {
	t.Parallel()
	for name, tc := range map[string]struct {
		set   func(t *testing.T, h *factorytest.Harness, id string)
		flags []string
		from  string
		fix   string
	}{
		"Ticket Tag": {
			set: func(t *testing.T, h *factorytest.Harness, id string) {
				setTags(t, h, id, map[string]string{"model": "opus-9"})
			},
			from: "the Ticket's `model` Tag",
			fix:  "Tag",
		},
		"Project kind row": {
			set: func(t *testing.T, h *factorytest.Harness, id string) {
				setModels(t, h, core.SetModelsInput{Kinds: []core.KindModelsView{{Kind: core.ImplementationStage, ModelRowView: core.ModelRowView{Model: "opus-9"}}}})
			},
			from: "the Project's Models row for implementation",
			fix:  "Models row for implementation",
		},
		"Project default": {
			set: func(t *testing.T, h *factorytest.Harness, id string) {
				setModels(t, h, core.SetModelsInput{Default: core.ModelRowView{Model: "opus-9"}})
			},
			from: "the Project's default Models row",
			fix:  "default Models row",
		},
		"Factory flag": {
			flags: []string{"--model", "opus-9"},
			from:  "the Factory's `--model` flag",
			fix:   "--model",
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			h := factorytest.New(t)
			id := h.ReadyTicket("Rejected")
			if tc.set != nil {
				tc.set(t, h, id)
			}
			h.Script(id, factorytest.ModelNotFound("opus-9")...)
			r := h.Start(tc.flags...)
			wantFailureReport(t, h, id, []string{"`opus-9`", tc.from, "model_not_found"}, []string{tc.fix})

			// Neither an outage nor fatal: the Factory carries on claiming.
			next := h.ReadyTicket("Next")
			r.Poll()
			h.WaitForState(next, store.InReview)
			if strings.Contains(r.Stdout(), "paused") {
				t.Errorf("output %q: a rejected model is no outage", r.Stdout())
			}
		})
	}
}

// fewerLevels is claude's help with xhigh and max gone from --effort.
var fewerLevels = strings.Replace(factorytest.DefaultHelp, "(low, medium, high, xhigh, max)", "(low, medium, high)", 1)

func TestAnEffortClaudeDoesntAcceptFailsTheClaimWithNoTurnStarted(t *testing.T) {
	t.Parallel()
	for name, tc := range map[string]struct {
		set  func(t *testing.T, h *factorytest.Harness, id string)
		from string
		fix  string
	}{
		"Ticket Tag": {
			set: func(t *testing.T, h *factorytest.Harness, id string) {
				setTags(t, h, id, map[string]string{"effort": "xhigh"})
			},
			from: "the Ticket's `effort` Tag",
			fix:  "Tag",
		},
		"Project kind row": {
			set: func(t *testing.T, h *factorytest.Harness, id string) {
				setModels(t, h, core.SetModelsInput{Kinds: []core.KindModelsView{{Kind: core.ImplementationStage, ModelRowView: core.ModelRowView{Effort: "xhigh"}}}})
			},
			from: "the Project's Models row for implementation",
			fix:  "Models row for implementation",
		},
		"Project default": {
			set: func(t *testing.T, h *factorytest.Harness, id string) {
				setModels(t, h, core.SetModelsInput{Default: core.ModelRowView{Effort: "xhigh"}})
			},
			from: "the Project's default Models row",
			fix:  "default Models row",
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			h := factorytest.New(t)
			id := h.ReadyTicket("Too much effort")
			// Set while the seeded levels allow it; this claude doesn't.
			tc.set(t, h, id)
			h.ClaudeHelp(fewerLevels)
			h.Start()
			wantFailureReport(t, h, id, []string{"`xhigh`", tc.from, "low, medium, high"}, []string{tc.fix, "low, medium, high"})
			if n := len(h.Calls()); n != 0 {
				t.Errorf("claude ran %d times; want no turn started", n)
			}
		})
	}
}

func TestAnEffortALaterReportMadeInvalidFailsTheResume(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t)
	id, _, r := answeredTicket(t, h)
	setTags(t, h, id, map[string]string{"effort": "xhigh"})
	r.Stop()

	h.ClaudeHelp(fewerLevels)
	h.Start()
	wantFailureReport(t, h, id, []string{"`xhigh`", "the Ticket's `effort` Tag"}, []string{"Tag"})
	if n := len(h.Calls()); n != 1 {
		t.Errorf("claude ran %d times; want the resume not started", n)
	}
}

func TestWithoutLevelsFromItsHelpTheFactoryDoesntCheckEffort(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t)
	id := h.ReadyTicket("Unchecked")
	setTags(t, h, id, map[string]string{"effort": "xhigh"})
	h.ClaudeHelp("Usage: claude [options]\n")
	h.Start()
	h.WaitForState(id, store.InReview)
	ranWith(t, h, id, h.Calls()[0], "", "xhigh")
}

func TestAModelRejectedWithOnlyItsResultFailsTheClaim(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t)
	id := h.ReadyTicket("No assistant error")
	setTags(t, h, id, map[string]string{"model": "opus-9"})
	// The result alone: terminal_reason api_error, api_error_status 404.
	h.Script(id, factorytest.ModelNotFound("opus-9")[1:]...)
	h.Start()
	wantFailureReport(t, h, id, []string{"`opus-9`", "the Ticket's `model` Tag"}, []string{"Tag"})
}

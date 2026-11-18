package core_test

import (
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/tomoram/code-central/core"
	"github.com/tomoram/code-central/domain"
)

// The Stage strip (Spec code-central-931): a Requirement's segments,
// current segment, needs and default Ticket, as the UI draws and opens
// them. Each case names the glossary rule it answers.

// segmentLines is each of r's segments as "stage progress", with its
// Ticket, Map and reason after when it has them.
func segmentLines(r *core.RequirementView) []string {
	var out []string
	for _, s := range r.Segments {
		line := []string{string(s.Stage), string(s.Progress)}
		for _, x := range []string{s.Ticket, s.Map, string(s.Reason)} {
			if x != "" {
				line = append(line, x)
			}
		}
		out = append(out, strings.Join(line, " "))
	}
	return out
}

// needLines is each of r's needs as "ticket reason", with its Stage after
// when it has one.
func needLines(r *core.RequirementView) []string {
	out := []string{}
	for _, n := range r.Needs {
		out = append(out, strings.TrimSpace(n.Ticket+" "+string(n.Reason)+" "+string(n.Stage)))
	}
	return out
}

// stripOf is Requirement ref's requirement block as get_spec reads it,
// after checking its find_specs row carries the same strip.
func (f fixture) stripOf(t *testing.T, ref core.Ref) *core.RequirementView {
	t.Helper()
	r := f.requirementOf(t, ref)
	if r == nil {
		t.Fatalf("%s is not a Requirement", ref)
	}
	rows, err := f.core.FindSpecs(ctx, "acme", core.FindSpecsInput{Requirements: true, Finished: true})
	if err != nil {
		t.Fatalf("find_specs: %v", err)
	}
	for _, row := range rows {
		if row.Requirement == nil || !slices.Equal(segmentLines(row.Requirement), segmentLines(r)) {
			continue
		}
		if row.Requirement.Current != r.Current || row.Requirement.DefaultTicket != r.DefaultTicket || !slices.Equal(needLines(row.Requirement), needLines(r)) {
			t.Fatalf("find_specs row %+v, get_spec %+v: the strip differs", row.Requirement, r)
		}
		return r
	}
	t.Fatalf("no find_specs row carries %s's strip %q", ref, segmentLines(r))
	return r
}

// stagesDone is f.requirement with all three Stages Done and these Build
// Tickets from acme-6 on, in claim order, each in its State.
func (f fixture) stagesDone(t *testing.T, states ...domain.State) {
	t.Helper()
	f.requirement(t)
	for _, n := range []int{3, 4, 5} {
		f.setState(t, n, domain.Done)
	}
	for _, s := range states {
		tk := f.ticket(t, "acme", core.CreateTicketInput{Spec: "2", Title: "Build", State: domain.Ready})
		if s != domain.Ready {
			f.setState(t, ticketNumber(t, tk.ID), s)
		}
		f.clock.Advance(1)
	}
}

// ticketNumber is the number of Ticket ID id in Project acme.
func ticketNumber(t *testing.T, id string) int {
	t.Helper()
	n, err := strconv.Atoi(strings.TrimPrefix(id, "acme-"))
	if err != nil {
		t.Fatalf("ticket ID %q: %v", id, err)
	}
	return n
}

func TestTheStageStripFollowsEachStageAndBuildThroughItsProgress(t *testing.T) {
	type want struct {
		segments []string
		current  core.SegmentStage
		needs    []string
		open     string
	}
	for name, c := range map[string]struct {
		setup func(f fixture, t *testing.T) core.Ref
		want  want
	}{
		// A Stage is queued while it is the first open one and a Factory
		// could claim it; a later Stage and Build wait behind it.
		"a new Requirement queues Grilling, and the rest wait": {
			func(f fixture, t *testing.T) core.Ref { f.requirement(t); return "2" },
			want{[]string{"grilling queued acme-3", "spec waiting acme-4", "tickets waiting acme-5", "build waiting"}, "grilling", []string{}, "acme-3"},
		},
		"a Stage a Factory holds is running": {
			func(f fixture, t *testing.T) core.Ref {
				f.requirement(t)
				f.claim(t, core.ClaimInput{Ticket: "acme-3"})
				return "2"
			},
			want{[]string{"grilling running acme-3", "spec waiting acme-4", "tickets waiting acme-5", "build waiting"}, "grilling", []string{}, "acme-3"},
		},
		"a Stage waiting on the human needs it, and the Requirement opens on it": {
			func(f fixture, t *testing.T) core.Ref {
				f.requirement(t)
				f.setState(t, 3, domain.InReview)
				return "2"
			},
			want{[]string{"grilling needs acme-3 in_review", "spec waiting acme-4", "tickets waiting acme-5", "build waiting"}, "grilling", []string{"acme-3 in_review grilling"}, "acme-3"},
		},
		"a Draft first Stage waits, as no Factory can claim it": {
			func(f fixture, t *testing.T) core.Ref { f.requirement(t); f.setState(t, 3, domain.Draft); return "2" },
			want{[]string{"grilling waiting acme-3", "spec waiting acme-4", "tickets waiting acme-5", "build waiting"}, "grilling", []string{}, "acme-3"},
		},
		"a Done Stage is done, and the next is current": {
			func(f fixture, t *testing.T) core.Ref { f.requirement(t); f.setState(t, 3, domain.Done); return "2" },
			want{[]string{"grilling done acme-3", "spec queued acme-4", "tickets waiting acme-5", "build waiting"}, "spec", []string{}, "acme-4"},
		},
		"a Cancelled Stage is skipped, passed over as a Done one is": {
			func(f fixture, t *testing.T) core.Ref {
				f.requirement(t)
				f.setState(t, 3, domain.Done)
				f.setState(t, 4, domain.Done)
				f.ticket(t, "acme", core.CreateTicketInput{Spec: "2", Title: "Everything", State: domain.Ready}) // acme-6
				f.cancel(t, core.CancelInput{Ticket: "acme-5", Replacements: []core.Ref{"acme-6"}}, core.Human)
				return "2"
			},
			want{[]string{"grilling done acme-3", "spec done acme-4", "tickets skipped acme-5", "build queued"}, "build", []string{}, "acme-6"},
		},
		// ADR 0036: neither a grilling Stage nor a Map.
		"Straight to Spec has a skipped Grilling segment with no Ticket": {
			func(f fixture, t *testing.T) core.Ref {
				f.project(t, "acme")
				if _, err := f.core.SubmitRequirement(ctx, "acme", core.SubmitRequirementInput{Text: darkMode, SkipGrilling: true}); err != nil {
					t.Fatalf("submit_requirement: %v", err)
				}
				return "1"
			},
			want{[]string{"grilling skipped", "spec queued acme-2", "tickets waiting acme-3", "build waiting"}, "spec", []string{}, "acme-2"},
		},
		"a Requirement made from a Map has a done Map segment in place of Grilling": {
			func(f fixture, t *testing.T) core.Ref {
				f.mapWith(t)
				f.setState(t, 2, domain.Done)
				if _, err := f.core.SubmitRequirement(ctx, "acme", core.SubmitRequirementInput{Text: darkMode, Map: "acme-1"}); err != nil {
					t.Fatalf("submit_requirement: %v", err)
				}
				return "3"
			},
			want{[]string{"grilling done acme-1", "spec queued acme-4", "tickets waiting acme-5", "build waiting"}, "spec", []string{}, "acme-4"},
		},
		// Build waits with no Tickets once the Stages are done, and the
		// Requirement opens on the last segment with a Ticket.
		"Build with no Tickets waits": {
			func(f fixture, t *testing.T) core.Ref { f.stagesDone(t); return "2" },
			want{[]string{"grilling done acme-3", "spec done acme-4", "tickets done acme-5", "build waiting"}, "build", []string{}, "acme-5"},
		},
		"Build is queued while a Build Ticket waits for a Factory, and opens on the first unfinished one": {
			func(f fixture, t *testing.T) core.Ref { f.stagesDone(t, domain.Done, domain.Ready); return "2" },
			want{[]string{"grilling done acme-3", "spec done acme-4", "tickets done acme-5", "build queued"}, "build", []string{}, "acme-7"},
		},
		"Build is running while a Factory holds a Build Ticket": {
			func(f fixture, t *testing.T) core.Ref {
				f.stagesDone(t, domain.Ready, domain.Ready)
				f.claim(t, core.ClaimInput{Ticket: "acme-7"})
				return "2"
			},
			want{[]string{"grilling done acme-3", "spec done acme-4", "tickets done acme-5", "build running"}, "build", []string{}, "acme-6"},
		},
		"Build needs the human when a Build Ticket does, with its first need's reason": {
			func(f fixture, t *testing.T) core.Ref {
				f.stagesDone(t, domain.Ready, domain.Failed, domain.InReview)
				return "2"
			},
			want{[]string{"grilling done acme-3", "spec done acme-4", "tickets done acme-5", "build needs failed"}, "build", []string{"acme-7 failed", "acme-8 in_review"}, "acme-7"},
		},
		"Build is done when every Build Ticket is, and opens on the last Build Ticket": {
			func(f fixture, t *testing.T) core.Ref { f.stagesDone(t, domain.Done, domain.Cancelled); return "2" },
			want{[]string{"grilling done acme-3", "spec done acme-4", "tickets done acme-5", "build done"}, "build", []string{}, "acme-7"},
		},
		// Needs: the Stage Tickets first, in Stage order, then the Build
		// Tickets in claim order; a Build Ticket needs the human even while
		// a Stage is open.
		"needs list the Stages first, then Build in claim order": {
			func(f fixture, t *testing.T) core.Ref {
				f.requirement(t)
				f.setState(t, 3, domain.Done)
				f.ticket(t, "acme", core.CreateTicketInput{Spec: "2", Title: "Toggle", State: domain.Ready}) // acme-6
				f.ticket(t, "acme", core.CreateTicketInput{Spec: "2", Title: "Docs", State: domain.Ready})   // acme-7
				for _, n := range []int{7, 6, 4} {
					f.setState(t, n, domain.InReview)
				}
				return "2"
			},
			want{
				[]string{"grilling done acme-3", "spec needs acme-4 in_review", "tickets waiting acme-5", "build needs in_review"}, "spec",
				[]string{"acme-4 in_review spec", "acme-6 in_review", "acme-7 in_review"}, "acme-4",
			},
		},
	} {
		t.Run(name, func(t *testing.T) {
			f := newFixture(t)
			r := f.stripOf(t, c.setup(f, t))
			if got := segmentLines(r); !slices.Equal(got, c.want.segments) {
				t.Errorf("segments %q, want %q", got, c.want.segments)
			}
			if r.Current != c.want.current {
				t.Errorf("current %q, want %q", r.Current, c.want.current)
			}
			if got := needLines(r); !slices.Equal(got, c.want.needs) {
				t.Errorf("needs %q, want %q", got, c.want.needs)
			}
			if r.DefaultTicket != c.want.open {
				t.Errorf("default_ticket %q, want %q", r.DefaultTicket, c.want.open)
			}
		})
	}
}

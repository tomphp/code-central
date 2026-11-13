package frontdoor_test

import (
	"context"
	"testing"

	"github.com/tomoram/code-central/codecentraltest"
	"github.com/tomoram/code-central/core"
)

// Submitting a Requirement (ticket 67).
func init() {
	cases["submit_requirement"] = opCase{
		ok: call{
			mcp:  map[string]any{"text": "Dark mode\n\nA toggle in settings."},
			http: httpCall{"POST", "/api/p/acme/submit_requirement", `{"text":"Dark mode\n\nA toggle in settings."}`},
			cli:  []string{"--text", "Dark mode\n\nA toggle in settings."},
		},
		check: func(t *testing.T, out map[string]any) {
			sp, _ := out["spec"].(map[string]any)
			acknowledgement("acme-4", "title", "body")(t, sp)
			stages, _ := out["stages"].([]any)
			if len(stages) != 3 {
				t.Fatalf("stages %v", out["stages"])
			}
			ticketAcknowledgement("acme-5", "Ready", "title", "state", "spec", "labels", "description", "moves", "claimable")(t, stages[0].(map[string]any))
			for i, id := range []string{"acme-6", "acme-7"} {
				ticketAcknowledgement(id, "Ready", "title", "state", "spec", "labels", "blocked_by", "blockers", "description", "moves", "blocked")(t, stages[i+1].(map[string]any))
			}
		},
		full: func(t *testing.T, out map[string]any) {
			sp, _ := out["spec"].(map[string]any)
			field("id", "acme-4")(t, sp)
			field("title", "Dark mode")(t, sp)
			field("body", "Dark mode\n\nA toggle in settings.")(t, sp)
			stages, _ := out["stages"].([]any)
			if len(stages) != 3 {
				t.Fatalf("stages %v", out["stages"])
			}
			for i, want := range []string{"stage:grilling", "stage:spec", "stage:tickets"} {
				st := stages[i].(map[string]any)
				field("state", "Ready")(t, st)
				labels(want)(t, st)
			}
		},
		bad: call{
			mcp:  map[string]any{"text": "  "},
			http: httpCall{"POST", "/api/p/acme/submit_requirement", `{"text":"  "}`},
			cli:  []string{"--text", "  "},
		},
		code: core.Invalid,
	}
}

// submittedRequirement submits Requirement acme-4, whose Stages are
// acme-5 (grilling), acme-6 (spec) and acme-7 (tickets).
func submittedRequirement(t *testing.T, srv *codecentraltest.Server) {
	t.Helper()
	must(t, func() error {
		_, err := srv.Core.SubmitRequirement(context.Background(), "acme", core.SubmitRequirementInput{Text: "Dark mode"})
		return err
	})
}

// requirementBlock checks out is Requirement acme-4, freshly submitted.
func requirementBlock(t *testing.T, out map[string]any) {
	t.Helper()
	field("id", "acme-4")(t, out)
	req, _ := out["requirement"].(map[string]any)
	if req == nil || req["waiting_for_factory"] != true {
		t.Fatalf("requirement %v", out["requirement"])
	}
	stages, _ := req["stages"].([]any)
	if len(stages) != 3 {
		t.Fatalf("stages %v", req["stages"])
	}
	for i, want := range [][2]string{{"grilling", "acme-5"}, {"spec", "acme-6"}, {"tickets", "acme-7"}} {
		st := stages[i].(map[string]any)
		field("name", want[0])(t, st)
		field("ticket", want[1])(t, st)
		field("state", "Ready")(t, st)
	}
	build, _ := req["build"].(map[string]any)
	if build["done"] != float64(0) || build["total"] != float64(0) {
		t.Fatalf("build %v", req["build"])
	}
}

// The requirement read and filter (ticket 68) are the same on
// every door.
func TestEveryDoorReadsARequirement(t *testing.T) {
	srv := newServer(t, submittedRequirement)
	s := connect(t, srv, "acme")
	gets := map[string]map[string]any{
		"http": httpOK(t, srv, httpCall{"GET", "/api/p/acme/specs/4", ""}),
		"mcp":  mcpOK(t, s, "get_spec", map[string]any{"spec": "acme-4"}),
		"cli":  cliOK(t, srv, []string{"get-spec", "acme-4"}),
	}
	finds := map[string]map[string]any{
		"http": httpOK(t, srv, httpCall{"GET", "/api/p/acme/specs?requirements=true", ""}),
		"mcp":  mcpOK(t, s, "find_specs", map[string]any{"requirements": true}),
		"cli":  cliOK(t, srv, []string{"find-specs", "--requirements"}),
	}
	for door, out := range gets {
		t.Run(door+"/get_spec", func(t *testing.T) { requirementBlock(t, out) })
	}
	for door, out := range finds {
		t.Run(door+"/find_specs", func(t *testing.T) {
			rows, _ := out["rows"].([]any)
			if len(rows) != 1 {
				t.Fatalf("only the Requirement, not Spec acme-1: %v", out["rows"])
			}
			requirementBlock(t, rows[0].(map[string]any))
		})
	}
	plain := httpOK(t, srv, httpCall{"GET", "/api/p/acme/specs/1", ""})
	if _, ok := plain["requirement"]; ok {
		t.Fatalf("a plain Spec has no requirement block: %v", plain)
	}

	httpRejected(t, srv, httpCall{"GET", "/api/p/acme/specs?requirements=maybe", ""}, core.Invalid)
	httpRejected(t, srv, httpCall{"GET", "/api/p/acme/specs?requirements=true&tag=Bad+Key%3Dx", ""}, core.Invalid)
	mcpRejected(t, s, "find_specs", map[string]any{"requirements": true, "tags": map[string]any{"Bad Key": "x"}}, core.Invalid)
	cliRejected(t, srv, []string{"find-specs", "--requirements", "--tag", "Bad Key=x"}, core.Invalid)
}

func TestNeedsMeRowsNameTheirRequirementOverHTTP(t *testing.T) {
	srv := newServer(t, submittedRequirement)
	must(t, func() error {
		_, err := srv.Core.Move(core.WithActor(context.Background(), core.Human), "acme", core.MoveInput{Ticket: "5", ToState: "In Review"})
		return err
	})
	out := httpOK(t, srv, httpCall{"GET", "/api/needs-me", ""})
	groups, _ := out["groups"].([]any)
	if len(groups) != 1 {
		t.Fatalf("needs-me %v", out)
	}
	row := groups[0].(map[string]any)["tickets"].([]any)[0].(map[string]any)
	req, _ := row["requirement"].(map[string]any)
	field("spec", "acme-4")(t, req)
	field("title", "Dark mode")(t, req)
	field("stage", "grilling")(t, req)
}

func TestTheSubmitRequirementCommandReadsTheTextFromAFile(t *testing.T) {
	srv := newServer(t)
	out := cliOK(t, srv, []string{"submit-requirement", "--file", "testdata/requirement.md", "--full"})
	sp, _ := out["spec"].(map[string]any)
	field("title", "Export to CSV")(t, sp)
}

// Straight to Spec (code-central-502): every door takes skip_grilling and
// refuses it with map.
func TestEveryDoorSubmitsAStraightToSpecRequirement(t *testing.T) {
	srv := newServer(t)
	s := connect(t, srv, "acme")
	mcpRejected(t, s, "submit_requirement", map[string]any{"text": "Dark mode", "skip_grilling": true, "map": "acme-1"}, core.Invalid)
	written := map[string]map[string]any{
		"mcp":  mcpOK(t, s, "submit_requirement", map[string]any{"text": "Dark mode", "skip_grilling": true, "full": true}),
		"http": httpOK(t, srv, httpCall{"POST", "/api/p/acme/submit_requirement", `{"text":"Dark mode","skip_grilling":true,"full":true}`}),
		"cli":  cliOK(t, srv, []string{"submit-requirement", "--text", "Dark mode", "--skip-grilling", "--full"}),
	}
	for door, out := range written {
		t.Run(door, func(t *testing.T) {
			stages, _ := out["stages"].([]any)
			if len(stages) != 2 {
				t.Fatalf("stages %v", out["stages"])
			}
			labels("stage:spec")(t, stages[0].(map[string]any))
			labels("stage:tickets")(t, stages[1].(map[string]any))
		})
	}
}

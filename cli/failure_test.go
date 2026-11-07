package cli_test

import (
	"context"
	"testing"

	"github.com/tomoram/code-central/codecentraltest"
	"github.com/tomoram/code-central/core"
	"github.com/tomoram/code-central/store"
)

func TestReportFailureTakesTheFieldsAsJSONOnStdinWithFlagsOverriding(t *testing.T) {
	srv := codecentraltest.NewServer(t)
	ctx := context.Background()
	srv.Core.CreateProject(ctx, core.CreateProjectInput{Key: "acme"})
	srv.Core.CreateSpec(ctx, "acme", core.CreateSpecInput{Title: "Login"})
	srv.Core.CreateTicket(ctx, "acme", core.CreateTicketInput{Spec: "1", Title: "Form", State: store.Ready})
	if _, err := srv.Core.Claim(ctx, "acme", core.ClaimInput{}); err != nil {
		t.Fatal(err)
	}
	env := map[string]string{"CODE_CENTRAL_URL": srv.URL, "CODE_CENTRAL_PROJECT": "acme"}
	stdin := `{"what_broke":"Login 500s","why":"No table","what_to_fix":"Migrate","where_work_stands":"Pushed"}`

	res := decode[core.StopResult](t, run(t, env, stdin, "report-failure", "acme-2", "--claim", "claim-1", "--file", "-", "--why", "Migration 12 never ran"))
	r := res.Ticket.FailureReports
	if res.Ticket.State != store.Failed || len(r) != 1 || r[0].WhatBroke != "Login 500s" || r[0].Why != "Migration 12 never ran" {
		t.Fatalf("result %+v", res)
	}
}

func TestReportFailureRejectsAFileThatIsNotTheFourFields(t *testing.T) {
	srv := codecentraltest.NewServer(t)
	env := map[string]string{"CODE_CENTRAL_URL": srv.URL, "CODE_CENTRAL_PROJECT": "acme"}
	if r := run(t, env, `{"broke":"x"}`, "report-failure", "acme-2", "--claim", "c", "--file", "-"); r.code != 1 {
		t.Fatalf("exit %d, want a usage error: %s", r.code, r.stderr)
	}
}

package cli_test

import (
	"context"
	"slices"
	"testing"

	"github.com/tomoram/code-central/api"
)

// The stop and resume commands (Spec code-central-1233) call the human's
// stop and resume through the client, as the human, outside the operation
// table.

func factoryClaimEnv(t *testing.T) map[string]string {
	t.Helper()
	srv, env := repositoryEnv(t)
	ctx := context.Background()
	must := func(_ any, err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	must(srv.Core.CreateSpec(ctx, "acme", api.CreateSpecInput{Title: "Sign in", Body: "b"}))
	must(srv.Core.CreateTicket(ctx, "acme", api.CreateTicketInput{Spec: "1", Title: "Form", State: "Ready"}))
	must(srv.Core.Claim(ctx, "acme", api.ClaimInput{SessionID: "s1", Factory: "cc", Host: "box"}))
	return env
}

func TestStopAndResumeAFactoryClaimAsTheHuman(t *testing.T) {
	t.Parallel()
	env := factoryClaimEnv(t)
	stopped := decode[api.Acknowledgement](t, run(t, env, "", "stop", "acme-2", "--note", "Wrong approach."))
	if !slices.Contains(stopped.Changed, "comments") || stopped.State != "In Progress" {
		t.Fatalf("stop wrote %+v, want the note and the Ticket still In Progress", stopped)
	}
	wantError(t, run(t, env, "", "stop", "acme-2"), api.ActionNotAllowed)
	if resumed := decode[api.Acknowledgement](t, run(t, env, "", "resume", "acme-2")); !slices.Contains(resumed.Changed, "claims") {
		t.Fatalf("resume wrote %+v", resumed)
	}
	wantError(t, run(t, env, "", "resume", "acme-2"), api.ActionNotAllowed)
}

func TestStopAndResumeTakeOneTicket(t *testing.T) {
	t.Parallel()
	env := factoryClaimEnv(t)
	for _, cmd := range []string{"stop", "resume"} {
		if r := run(t, env, "", cmd); r.code != 1 {
			t.Fatalf("%s with no Ticket exited %d, want a usage error", cmd, r.code)
		}
	}
}

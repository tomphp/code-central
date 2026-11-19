package server_test

import (
	"context"
	"testing"
	"time"

	"github.com/tomoram/code-central/api"
)

// A Requirement's landed and Finished comments in a Project with several
// Repositories (code-central-858): the landed comment gives one commit per
// Changed Repository, as owner/repo@sha when the Repository's Clone URL is
// on GitHub and Name@sha otherwise, and the Finished comment lists the
// Spec's Changed Repositories the same way.
func TestALandedCommentGivesOneCommitPerChangedRepository(t *testing.T) {
	srv := newServer(t, requirementFromIssue)
	asAgent(t, srv, reportPushRemote(`{"url":"git@github.com:acme/home.git"}`))
	human := api.WithActor(context.Background(), api.Human)
	for _, in := range []api.AddRepositoryInput{
		{Name: "web", CloneURL: "git@github.com:acme/web.git"},
		{Name: "docs", CloneURL: "https://dev.azure.com/org/proj/_git/docs"},
		{Name: "tools", CloneURL: "git@github.com:acme/tools.git"},
	} {
		if _, err := srv.Core.AddRepository(human, "acme", in); err != nil {
			t.Fatal(err)
		}
	}
	httpOK(t, srv, reportIssueWritten(`{"target":"acme-6","key":"context"}`))
	for _, n := range []string{"7", "8", "9"} {
		srv.Clock.Advance(time.Minute)
		httpOK(t, srv, httpCall{"POST", "/api/p/acme/tickets/" + n + "/move", `{"to_state":"Done"}`})
	}
	implementationTicket(t, srv, "Fix the login") // acme-10

	claim := claimedByAgent(t, srv, "10")
	srv.Clock.Advance(time.Minute)
	asAgent(t, srv, httpCall{"POST", "/api/p/acme/tickets/10/finish", `{"claim_id":` + quote(claim) + `,"summary":"Fixed.","branch":"acme-10","commit":"h1","repositories":[` +
		`{"name":"docs","branch":"acme-10","commit":"d1"},{"name":"web","branch":"acme-10","commit":"w1"}]}`})
	httpOK(t, srv, httpCall{"POST", "/api/p/acme/tickets/10/accept", `{}`})
	for _, l := range []string{`{"commit":"dd","repository":"docs"}`, `{"commit":"hh","repository":"acme"}`, `{"commit":"ww","repository":"web"}`} {
		srv.Clock.Advance(time.Minute)
		httpOK(t, srv, httpCall{"POST", "/api/p/acme/tickets/10/land", l})
	}

	got := writesDue(t, srv)
	wantDue(t, got, "acme-6", "code-central:done", "completed", "spec", "landed:acme-10", "closed")
	if body, want := dueComment(t, got, "acme-6", "landed:acme-10"), "<!-- code-central acme-6 landed:acme-10 -->\nLanded **Fix the login** (acme-10) in acme/home@hh, acme/web@ww, docs@dd."+backTo(srv); body != want {
		t.Fatalf("landed comment = %q, want %q", body, want)
	}
	want := "<!-- code-central acme-6 closed -->\nFinished. What landed:\n\n- **Fix the login** (acme-10) in acme/home@hh, acme/web@ww, docs@dd\n\nChanged Repositories: acme/home, acme/web, docs" + backTo(srv)
	if body := dueComment(t, got, "acme-6", "closed"); body != want {
		t.Fatalf("closed comment = %q, want %q", body, want)
	}
}

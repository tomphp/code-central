package client_test

import (
	"context"
	"errors"
	"net"
	"slices"
	"strings"
	"testing"

	"github.com/tomoram/code-central/client"
	"github.com/tomoram/code-central/codecentraltest"
	"github.com/tomoram/code-central/core"
	"github.com/tomoram/code-central/store"
	"github.com/tomoram/code-central/usage"
)

var ctx = context.Background()

func wantCode(t *testing.T, err error, code core.Code, field string) *core.Error {
	t.Helper()
	var e *core.Error
	if !errors.As(err, &e) || e.Code != code || e.Field != field {
		t.Fatalf("err = %v, want code %s field %q", err, code, field)
	}
	return e
}

func TestProjectSpecAndTicketRoundTrip(t *testing.T) {
	srv := codecentraltest.NewServer(t)
	c := srv.Client

	p, err := c.CreateProject(ctx, core.CreateProjectInput{Key: "acme"})
	if err != nil || p.Key != "acme" || p.DisplayName != "acme" || !p.Created.Equal(srv.Clock.Now()) {
		t.Fatalf("create_project: %+v, %v", p, err)
	}
	if got, err := c.GetProject(ctx, "acme"); err != nil || got.Key != "acme" {
		t.Fatalf("get_project: %+v, %v", got, err)
	}

	s, err := c.CreateSpec(ctx, "acme", core.CreateSpecInput{Title: "Login", Body: "# Login\n"})
	if err != nil || s.ID != "acme-1" || !slices.Equal(s.Changed, []string{"title", "body"}) || s.Record != nil {
		t.Fatalf("create_spec: %+v, %v", s, err)
	}
	if s, err := c.EditSpec(ctx, "acme", core.EditSpecInput{Spec: "1", Full: true}); err != nil || s.Record == nil || s.Record.Body != "# Login\n" {
		t.Fatalf("edit_spec with full: %+v, %v", s, err)
	}
	tk, err := c.CreateTicket(ctx, "acme", core.CreateTicketInput{Spec: "acme-1", Title: "Form", State: store.Ready, Labels: []string{"ui"}})
	if err != nil || tk.ID != "acme-2" || tk.State != store.Ready || tk.Record != nil {
		t.Fatalf("create_ticket: %+v, %v", tk, err)
	}
	if tk, err := c.Comment(ctx, "acme", core.CommentInput{Ticket: "2", Body: "Hi.", Full: true}); err != nil || tk.Record == nil || tk.Record.Spec.Title != "Login" {
		t.Fatalf("comment with full: %+v, %v", tk, err)
	}
	got, err := c.GetTicket(ctx, "acme", "2")
	if err != nil || got.ID != "acme-2" || got.Title != "Form" || !got.Claimable {
		t.Fatalf("get_ticket: %+v, %v", got, err)
	}
	rows, err := c.FindTickets(ctx, "acme", core.FindTicketsInput{State: []store.State{store.Ready}, Labels: []string{"ui"}, Title: "form", Spec: "1"})
	if err != nil || len(rows) != 1 || rows[0].ID != "acme-2" {
		t.Fatalf("find_tickets: %+v, %v", rows, err)
	}
	if rows, err := c.FindTickets(ctx, "acme", core.FindTicketsInput{State: []store.State{store.Draft}}); err != nil || len(rows) != 0 {
		t.Fatalf("find_tickets Draft: %+v, %v", rows, err)
	}
}

func TestTheDeliveryLoopRoundTrip(t *testing.T) {
	srv := codecentraltest.NewServer(t)
	agent := srv.Client
	agent.CreateProject(ctx, core.CreateProjectInput{Key: "acme"})
	agent.CreateSpec(ctx, "acme", core.CreateSpecInput{Title: "Login"})
	agent.CreateTicket(ctx, "acme", core.CreateTicketInput{Spec: "1", Title: "A", State: store.Ready})
	agent.CreateTicket(ctx, "acme", core.CreateTicketInput{Spec: "1", Title: "B", State: store.Ready})

	c, err := agent.Claim(ctx, "acme", core.ClaimInput{Ticket: "acme-3"})
	if err != nil || c.ClaimID == "" || c.Ticket.ID != "acme-3" {
		t.Fatalf("claim: %+v, %v", c, err)
	}
	f, err := agent.Finish(ctx, "acme", core.FinishInput{Ticket: "acme-3", ClaimID: c.ClaimID, Summary: "Done."})
	if err != nil || f.Acknowledgement == nil || f.State != store.InReview || f.NextStep != "stop working this Ticket" {
		t.Fatalf("finish: %+v, %v", f, err)
	}
	_, err = agent.Accept(ctx, "acme", core.TicketInput{Ticket: "acme-3"})
	wantCode(t, err, core.ActionNotAllowed, "")

	human := client.New(srv.URL)
	human.Actor = core.Human
	if v, err := human.Reject(ctx, "acme", core.RejectInput{Ticket: "3", Comment: "Again."}); err != nil || v.State != store.Ready {
		t.Fatalf("reject: %+v, %v", v, err)
	}
	c, _ = agent.Claim(ctx, "acme", core.ClaimInput{Ticket: "3"})
	agent.Finish(ctx, "acme", core.FinishInput{Ticket: "3", ClaimID: c.ClaimID, Summary: "Fixed."})
	if v, err := human.Accept(ctx, "acme", core.TicketInput{Ticket: "3"}); err != nil || v.State != store.Done {
		t.Fatalf("accept: %+v, %v", v, err)
	}
	if none, err := agent.Claim(ctx, "acme", core.ClaimInput{Spec: "1"}); err != nil || none.Ticket == nil || none.Ticket.ID != "acme-2" {
		t.Fatalf("claim: %+v, %v; want acme-2 left", none, err)
	}
	if none, err := agent.Claim(ctx, "acme", core.ClaimInput{}); err != nil || none.Ticket != nil {
		t.Fatalf("claim: %+v, %v; want ticket null", none, err)
	}
}

func TestErrorsArriveInTheOneShape(t *testing.T) {
	srv := codecentraltest.NewServer(t)
	c := srv.Client
	_, err := c.GetProject(ctx, "nope")
	wantCode(t, err, core.UnknownProject, "")

	c.CreateProject(ctx, core.CreateProjectInput{Key: "acme"})
	_, err = c.GetTicket(ctx, "acme", "9")
	wantCode(t, err, core.NotFound, "")
	_, err = c.CreateTicket(ctx, "acme", core.CreateTicketInput{Spec: "1", Title: "T"})
	wantCode(t, err, core.Invalid, "spec")
}

func TestUnreachableServiceIsServiceUnavailable(t *testing.T) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	url := "http://" + l.Addr().String()
	l.Close() // nothing listens here now

	_, err = client.New(url).GetProject(ctx, "acme")
	e := wantCode(t, err, core.ServiceUnavailable, "")
	for _, want := range []string{url, "code-central serve", "code-central service install"} {
		if !strings.Contains(e.Message, want) {
			t.Errorf("message %q doesn't mention %q", e.Message, want)
		}
	}
}

func TestPostUsageIsReadBackAsTheClaimsSession(t *testing.T) {
	srv := codecentraltest.NewServer(t)
	c := srv.Client
	c.CreateProject(ctx, core.CreateProjectInput{Key: "acme"})
	c.CreateSpec(ctx, "acme", core.CreateSpecInput{Title: "Login"})
	c.CreateTicket(ctx, "acme", core.CreateTicketInput{Spec: "1", Title: "A", State: store.Ready})
	if _, err := c.Claim(ctx, "acme", core.ClaimInput{SessionID: "s1"}); err != nil {
		t.Fatal(err)
	}

	sent := []core.UsageResponse{{Time: srv.Clock.Now(), Kind: "implementation", Tokens: usage.Tokens{InputTokens: 10, CacheReadTokens: 90, OutputTokens: 5}}}
	if err := c.PostUsage(ctx, "s1", sent); err != nil {
		t.Fatalf("post: %v", err)
	}
	if err := c.PostUsage(ctx, "unknown", sent); err != nil {
		t.Fatalf("post for an unknown session: %v", err)
	}
	for _, key := range []string{"acme", ""} {
		u, err := c.Usage(ctx, key, 30)
		if err != nil || len(u.Sessions) != 1 || u.Sessions[0].SessionID != "s1" || u.Sessions[0].Peak != 100 {
			t.Fatalf("Usage(%q) %+v, %v; want the one session posted, peak 100", key, u, err)
		}
	}
}

func TestPostSessionEventsReachesTheClaimsTranscript(t *testing.T) {
	srv := codecentraltest.NewServer(t)
	c := srv.Client
	c.CreateProject(ctx, core.CreateProjectInput{Key: "acme"})
	c.CreateSpec(ctx, "acme", core.CreateSpecInput{Title: "Login"})
	c.CreateTicket(ctx, "acme", core.CreateTicketInput{Spec: "1", Title: "A", State: store.Ready})
	if _, err := c.Claim(ctx, "acme", core.ClaimInput{SessionID: "s1"}); err != nil {
		t.Fatal(err)
	}

	sent := []core.SessionEvent{{Time: srv.Clock.Now(), Kind: "tool_call", Text: "Bash: make", Tool: "Bash", ToolUseID: "tu1"}}
	if err := c.PostSessionEvents(ctx, "s1", sent); err != nil {
		t.Fatalf("post: %v", err)
	}
	if err := c.PostSessionEvents(ctx, "unknown", sent); err != nil {
		t.Fatalf("post for an unknown session: %v", err)
	}
	tr, err := srv.Core.Transcript(ctx, "acme", "2", 1)
	if err != nil || len(tr.Events) != 1 || tr.Events[0].Seq != 1 || tr.Events[0].SessionEvent != sent[0] {
		t.Fatalf("Transcript %+v, %v; want the one event sent", tr, err)
	}
}

package client_test

import (
	"context"
	"errors"
	"net"
	"strings"
	"testing"

	"github.com/tomoram/code-central/client"
	"github.com/tomoram/code-central/codecentraltest"
	"github.com/tomoram/code-central/core"
	"github.com/tomoram/code-central/store"
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
	if err != nil || s.ID != "acme-1" || s.Body != "# Login\n" {
		t.Fatalf("create_spec: %+v, %v", s, err)
	}
	tk, err := c.CreateTicket(ctx, "acme", core.CreateTicketInput{Spec: "acme-1", Title: "Form", State: store.Ready, Labels: []string{"ui"}})
	if err != nil || tk.ID != "acme-2" || tk.State != store.Ready || tk.Spec.Title != "Login" {
		t.Fatalf("create_ticket: %+v, %v", tk, err)
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

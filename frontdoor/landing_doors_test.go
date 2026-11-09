package frontdoor_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/tomoram/code-central/core"
	"github.com/tomoram/code-central/events"
	"github.com/tomoram/code-central/httpapi"
)

// The Landing Method (code-central-173): the human's, so HTTP only.
func init() {
	cases["set_landing_method"] = opCase{
		ok: call{http: httpCall{"POST", "/api/p/acme/set_landing_method", `{"method":"branch","push":true,"remote":" upstream "}`}},
		check: func(t *testing.T, out map[string]any) {
			field("key", "acme")(t, out)
			landing(t, out, "branch", true, "upstream")
		},
		bad:  call{http: httpCall{"POST", "/api/p/acme/set_landing_method", `{"method":"squash"}`}},
		code: core.Invalid,
	}
}

// landing checks a Project's landing object.
func landing(t *testing.T, project map[string]any, method string, push bool, remote string) {
	t.Helper()
	l, _ := project["landing"].(map[string]any)
	if l == nil || l["method"] != method || l["push"] != push || l["remote"] != remote {
		t.Fatalf("landing = %v, want %s, push %v, remote %s in %v", project["landing"], method, push, remote, project)
	}
}

func setLandingMethod(body string) httpCall {
	return httpCall{"POST", "/api/p/acme/set_landing_method", body}
}

func TestEveryProjectReadCarriesTheDefaultLandingMethod(t *testing.T) {
	srv := newServer(t)
	landing(t, httpOK(t, srv, httpCall{"GET", "/api/projects/acme", ""}), "merge", false, "origin")
	rows := httpOK(t, srv, httpCall{"GET", "/api/projects", ""})["rows"].([]any)
	for _, row := range rows {
		landing(t, row.(map[string]any), "merge", false, "origin")
	}
}

func TestASetLandingMethodIsReadBackByEveryProjectRead(t *testing.T) {
	srv := newServer(t)
	httpOK(t, srv, setLandingMethod(`{"method":"branch","push":true,"remote":"upstream"}`))
	landing(t, httpOK(t, srv, httpCall{"GET", "/api/projects/acme", ""}), "branch", true, "upstream")
	rows := httpOK(t, srv, httpCall{"GET", "/api/projects", ""})["rows"].([]any)
	landing(t, rows[0].(map[string]any), "branch", true, "upstream")
	landing(t, rows[1].(map[string]any), "merge", false, "origin")
}

func TestSetLandingMethodIsRejectedForAnAgent(t *testing.T) {
	srv := newServer(t)
	status, b := httpDoAs(t, srv, core.Agent, setLandingMethod(`{"method":"branch"}`))
	if e := errorJSON(t, b, core.ActionNotAllowed); status != httpapi.Status(core.ActionNotAllowed) {
		t.Fatalf("status %d, want %d for %s", status, httpapi.Status(core.ActionNotAllowed), e.Code)
	}
	landing(t, httpOK(t, srv, httpCall{"GET", "/api/projects/acme", ""}), "merge", false, "origin")
}

func TestAnUnknownLandingMethodIsInvalidNamingBoth(t *testing.T) {
	srv := newServer(t)
	for _, body := range []string{`{"method":"squash"}`, `{"push":false}`} {
		e := httpRejected(t, srv, setLandingMethod(body), core.Invalid)
		if e.Field != "method" || !strings.Contains(e.Message, `"merge"`) || !strings.Contains(e.Message, `"branch"`) {
			t.Fatalf("%s: field %q message %q, want method naming merge and branch", body, e.Field, e.Message)
		}
	}
}

func TestTheRemoteIsTrimmedAndOnlyNeededWithPush(t *testing.T) {
	srv := newServer(t)
	e := httpRejected(t, srv, setLandingMethod(`{"method":"merge","push":true,"remote":"  "}`), core.Invalid)
	if e.Field != "remote" {
		t.Fatalf("field %q, want remote", e.Field)
	}
	landing(t, httpOK(t, srv, setLandingMethod(`{"method":"merge","push":true,"remote":"  backup\t"}`)), "merge", true, "backup")
	landing(t, httpOK(t, srv, setLandingMethod(`{"method":"branch","push":false,"remote":" "}`)), "branch", false, "origin")
	landing(t, httpOK(t, srv, setLandingMethod(`{"method":"branch"}`)), "branch", false, "origin")
	landing(t, httpOK(t, srv, setLandingMethod(`{"method":"branch","remote":"backup"}`)), "branch", false, "backup")
}

func TestAnArchivedProjectRejectsSetLandingMethod(t *testing.T) {
	srv := newServer(t)
	human := core.WithActor(context.Background(), core.Human)
	must(t, func() error { _, err := srv.Core.ArchiveProject(human, "other"); return err })
	httpRejected(t, srv, httpCall{"POST", "/api/p/other/set_landing_method", `{"method":"branch"}`}, core.ActionNotAllowed)
	httpRejected(t, srv, httpCall{"POST", "/api/p/nope/set_landing_method", `{"method":"branch"}`}, core.UnknownProject)
}

// Every store write sends a notice, so none means nothing was written.
func TestTheSameLandingMethodWritesNothingAndAChangeSendsANotice(t *testing.T) {
	srv := newServer(t)
	notices, stop := srv.Core.Events().Subscribe()
	defer stop()
	landing(t, httpOK(t, srv, setLandingMethod(`{"method":"merge","push":false,"remote":"origin"}`)), "merge", false, "origin")
	landing(t, httpOK(t, srv, setLandingMethod(`{"method":"merge"}`)), "merge", false, "origin")
	httpOK(t, srv, setLandingMethod(`{"method":"branch"}`))
	select {
	case got := <-notices:
		if want := (events.Event{Type: events.ProjectChanged, Project: "acme"}); got != want {
			t.Fatalf("notice %+v, want %+v: the same values sent a notice", got, want)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("a real change sent no notice")
	}
	httpOK(t, srv, setLandingMethod(`{"method":"branch"}`))
	select {
	case got := <-notices:
		t.Fatalf("the same values sent %+v", got)
	case <-time.After(100 * time.Millisecond):
	}
}

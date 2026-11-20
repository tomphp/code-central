package server_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/tomoram/code-central/api"
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
		code: api.Invalid,
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
	status, b := httpDoAs(t, srv, api.Agent, setLandingMethod(`{"method":"branch"}`))
	if e := errorJSON(t, b, api.ActionNotAllowed); status != httpapi.Status(api.ActionNotAllowed) {
		t.Fatalf("status %d, want %d for %s", status, httpapi.Status(api.ActionNotAllowed), e.Code)
	}
	landing(t, httpOK(t, srv, httpCall{"GET", "/api/projects/acme", ""}), "merge", false, "origin")
}

func TestAnUnknownLandingMethodIsInvalidNamingBoth(t *testing.T) {
	srv := newServer(t)
	for _, body := range []string{`{"method":"squash"}`, `{"push":false}`} {
		e := httpRejected(t, srv, setLandingMethod(body), api.Invalid)
		if e.Field != "method" || !strings.Contains(e.Message, `"merge"`) || !strings.Contains(e.Message, `"branch"`) {
			t.Fatalf("%s: field %q message %q, want method naming merge and branch", body, e.Field, e.Message)
		}
	}
}

func TestTheRemoteIsTrimmedAndOnlyNeededWithPush(t *testing.T) {
	srv := newServer(t)
	e := httpRejected(t, srv, setLandingMethod(`{"method":"merge","push":true,"remote":"  "}`), api.Invalid)
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
	human := api.WithActor(context.Background(), api.Human)
	must(t, func() error { _, err := srv.Core.ArchiveProject(human, "other"); return err })
	httpRejected(t, srv, httpCall{"POST", "/api/p/other/set_landing_method", `{"method":"branch"}`}, api.ActionNotAllowed)
	httpRejected(t, srv, httpCall{"POST", "/api/p/nope/set_landing_method", `{"method":"branch"}`}, api.UnknownProject)
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

func landPerSpec(t *testing.T, project map[string]any) any {
	t.Helper()
	l, _ := project["landing"].(map[string]any)
	return l["land_per_spec"]
}

func TestLandPerSpecIsOnByDefaultAndKeptWhenSetLandingMethodLeavesItOut(t *testing.T) {
	srv := newServer(t)
	if got := landPerSpec(t, httpOK(t, srv, httpCall{"GET", "/api/projects/acme", ""})); got != true {
		t.Fatalf("land_per_spec = %v, want true by default", got)
	}
	out := httpOK(t, srv, setLandingMethod(`{"method":"merge","land_per_spec":false}`))
	if got := landPerSpec(t, out); got != false {
		t.Fatalf("land_per_spec = %v, want false once set", got)
	}
	// Left out, the stored value stays, even under Branch.
	out = httpOK(t, srv, setLandingMethod(`{"method":"branch"}`))
	if got := landPerSpec(t, out); got != false {
		t.Fatalf("land_per_spec = %v, want false kept", got)
	}
	out = httpOK(t, srv, setLandingMethod(`{"method":"merge","land_per_spec":true}`))
	if got := landPerSpec(t, out); got != true {
		t.Fatalf("land_per_spec = %v, want true once set back", got)
	}
}

func TestRebaseIsOffByDefaultAndKeptWhenSetLandingMethodLeavesItOut(t *testing.T) {
	srv := newServer(t)
	rebase := func(project map[string]any) any {
		l, _ := project["landing"].(map[string]any)
		return l["rebase"]
	}
	if got := rebase(httpOK(t, srv, httpCall{"GET", "/api/projects/acme", ""})); got != false {
		t.Fatalf("rebase = %v, want false by default", got)
	}
	out := httpOK(t, srv, setLandingMethod(`{"method":"merge","rebase":true}`))
	if got := rebase(out); got != true {
		t.Fatalf("rebase = %v, want true once set", got)
	}
	// Left out, the stored value stays, even under Branch.
	out = httpOK(t, srv, setLandingMethod(`{"method":"branch"}`))
	if got := rebase(out); got != true {
		t.Fatalf("rebase = %v, want true kept", got)
	}
	out = httpOK(t, srv, setLandingMethod(`{"method":"merge","rebase":false}`))
	if got := rebase(out); got != false {
		t.Fatalf("rebase = %v, want false once set back", got)
	}
}

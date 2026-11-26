package server_test

import (
	"strings"
	"testing"

	"github.com/tomoram/code-central/api"
	"github.com/tomoram/code-central/httpapi"
)

// Check Command (Spec code-central-1229): set_check_command is the
// human's, so HTTP only, as add_repository and edit_repository are.
func init() {
	cases["set_check_command"] = opCase{
		ok:    call{http: setCheckCommand(`{"check_command":" make typecheck "}`)},
		check: acknowledgement("acme", "check_command", "repositories"),
		full: func(t *testing.T, out map[string]any) {
			if out["check_command"] != "make typecheck" {
				t.Fatalf("check_command = %v, want make typecheck", out["check_command"])
			}
		},
		bad:  call{http: setCheckCommand(`{"check_command":"make\ntest"}`)},
		code: api.Invalid,
	}
}

func setCheckCommand(body string) httpCall {
	return httpCall{"POST", "/api/p/acme/set_check_command", body}
}

func TestSetCheckCommandIsRejectedForAnAgent(t *testing.T) {
	srv := newServer(t)
	status, b := httpDoAs(t, srv, api.Agent, setCheckCommand(`{"check_command":"make check"}`))
	if e := errorJSON(t, b, api.ActionNotAllowed); status != httpapi.Status(api.ActionNotAllowed) || !strings.Contains(e.Message, "human") {
		t.Fatalf("status %d, want %d; message %q", status, httpapi.Status(api.ActionNotAllowed), e.Message)
	}
	if p := httpOK(t, srv, httpCall{"GET", "/api/projects/acme", ""}); p["check_command"] != "" {
		t.Fatalf("check_command = %v, want none still", p["check_command"])
	}
}

func TestACheckCommandWithALineBreakIsRefusedOnItsField(t *testing.T) {
	for _, c := range []httpCall{
		setCheckCommand(`{"check_command":"make\ntest"}`),
		addRepository(`{"name":"web","clone_url":"git@github.com:acme/web.git","check_command":"npm run\r\ncheck"}`),
		editRepository(`{"name":"acme","check_command":"make\rcheck"}`),
	} {
		srv := newServer(t)
		status, b := httpDoAs(t, srv, api.Human, c)
		if e := errorJSON(t, b, api.Invalid); status != httpapi.Status(api.Invalid) || e.Field != "check_command" || !strings.Contains(e.Message, "line break") {
			t.Errorf("%s: status %d, field %q, message %q; want invalid check_command naming the line break", c.path, status, e.Field, e.Message)
		}
	}
}

func TestARepositorysCheckCommandIsItsOwnElseTheProjectsElseNone(t *testing.T) {
	srv := newServer(t)
	check := func(p map[string]any, name, want string, own bool) {
		t.Helper()
		if r := repositoryIn(p, name); r["check_command"] != want || r["own_check_command"] != own {
			t.Errorf("%s: check_command %v, own %v; want %q, own %v", name, r["check_command"], r["own_check_command"], want, own)
		}
	}
	p := httpOK(t, srv, addRepository(`{"name":"web","clone_url":"git@github.com:acme/web.git","check_command":" npm run check ","full":true}`))
	check(p, "acme", "", false)
	check(p, "web", "npm run check", true)

	p = httpOK(t, srv, setCheckCommand(`{"check_command":"make check","full":true}`))
	check(p, "acme", "make check", false)
	check(p, "web", "npm run check", true)

	p = httpOK(t, srv, editRepository(`{"name":"acme","check_command":"make typecheck","full":true}`))
	check(p, "acme", "make typecheck", true)

	p = httpOK(t, srv, editRepository(`{"name":"web","landing":"branch","full":true}`))
	check(p, "web", "npm run check", true)

	p = httpOK(t, srv, editRepository(`{"name":"web","check_command":"","full":true}`))
	check(p, "web", "make check", false)

	p = httpOK(t, srv, setCheckCommand(`{"full":true}`))
	if p["check_command"] != "" {
		t.Errorf("check_command = %v, want cleared", p["check_command"])
	}
	check(p, "acme", "make typecheck", true)
	check(p, "web", "", false)
}

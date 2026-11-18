package cli_test

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"strings"
	"testing"

	"github.com/tomoram/code-central/client"
	"github.com/tomoram/code-central/codecentraltest"
	"github.com/tomoram/code-central/core"
	"github.com/tomoram/code-central/ops"
)

type result struct {
	code           int
	stdout, stderr string
}

// run runs one command in-process with only the given environment, from
// an empty temporary folder so no real .mcp.json is found.
func run(t *testing.T, env map[string]string, stdin string, args ...string) result {
	t.Helper()
	return runIn(t, t.TempDir(), env, stdin, args...)
}

// userHomeDir is os.UserHomeDir on a Windows account: the profile folder
// is env's USERPROFILE, and there is none when that isn't set.
func userHomeDir(env map[string]string) func() (string, error) {
	return func() (string, error) {
		if d := env["USERPROFILE"]; d != "" {
			return d, nil
		}
		return "", errors.New("%USERPROFILE% is not defined")
	}
}

func decode[T any](t *testing.T, r result) T {
	t.Helper()
	if r.code != 0 {
		t.Fatalf("exit %d, stderr: %s", r.code, r.stderr)
	}
	var v T
	if err := json.Unmarshal([]byte(r.stdout), &v); err != nil {
		t.Fatalf("stdout is not JSON: %v\n%s", err, r.stdout)
	}
	return v
}

func wantError(t *testing.T, r result, code core.Code) core.Error {
	t.Helper()
	if r.code != 2 {
		t.Fatalf("exit %d, want 2 (stdout %q, stderr %q)", r.code, r.stdout, r.stderr)
	}
	var e core.Error
	if err := json.Unmarshal([]byte(r.stderr), &e); err != nil {
		t.Fatalf("stderr is not JSON: %v\n%s", err, r.stderr)
	}
	if e.Code != code {
		t.Fatalf("code %s, want %s: %s", e.Code, code, e.Message)
	}
	return e
}

func TestCreateAndReadSpecsAndTickets(t *testing.T) {
	t.Parallel()
	srv := codecentraltest.NewServer(t)
	if _, err := srv.Client.CreateProject(context.Background(), core.CreateProjectInput{Key: "acme"}); err != nil {
		t.Fatal(err)
	}
	env := map[string]string{"CODE_CENTRAL_URL": srv.URL, "CODE_CENTRAL_PROJECT": "acme"}

	spec := decode[core.SpecView](t, run(t, env, "# Login\n\nUsers log in.\n", "create-spec", "--title", "Login", "--file", "-", "--full"))
	if spec.ID != "acme-1" || spec.Body != "# Login\n\nUsers log in.\n" {
		t.Fatalf("spec: %+v", spec)
	}

	tk := decode[core.TicketView](t, run(t, env, "", "create-ticket", "--spec", "acme-1", "--title", "Form",
		"--state", "Ready", "--label", "ui", "--label", "auth", "--description", "Build it.", "--tag", "effort=high", "--tag", "area=web", "--full"))
	if tk.ID != "acme-2" || tk.State != "Ready" || len(tk.Labels) != 2 || tk.Description != "Build it." || tk.Tags["effort"] != "high" || tk.Tags["area"] != "web" {
		t.Fatalf("ticket: %+v", tk)
	}

	got := decode[core.TicketView](t, run(t, env, "", "get-ticket", "acme-2"))
	if got.Title != "Form" || got.Spec.Title != "Login" {
		t.Fatalf("get-ticket: %+v", got)
	}

	rows := decode[[]core.TicketRow](t, run(t, env, "", "find-tickets", "--state", "Ready", "--label", "ui", "--title", "FORM", "--spec", "1"))
	if len(rows) != 1 || rows[0].ID != "acme-2" {
		t.Fatalf("find-tickets: %+v", rows)
	}
	if rows := decode[[]core.TicketRow](t, run(t, env, "", "find-tickets", "--state", "Draft")); len(rows) != 0 {
		t.Fatalf("find-tickets Draft: %+v", rows)
	}
}

func TestProjectFlagBeatsEnvironment(t *testing.T) {
	t.Parallel()
	srv := codecentraltest.NewServer(t)
	srv.Client.CreateProject(context.Background(), core.CreateProjectInput{Key: "acme"})
	env := map[string]string{"CODE_CENTRAL_URL": srv.URL, "CODE_CENTRAL_PROJECT": "other"}
	spec := decode[core.SpecView](t, run(t, env, "", "create-spec", "--project", "acme", "--title", "S"))
	if spec.ID != "acme-1" {
		t.Fatalf("spec: %+v", spec)
	}
	// flags may follow positional arguments
	decode[core.TicketView](t, run(t, env, "", "create-ticket", "--project", "acme", "--spec", "1", "--title", "T"))
	decode[core.TicketView](t, run(t, env, "", "get-ticket", "2", "--project", "acme"))
}

func TestErrorsAreJSONOnStderrWithExitTwo(t *testing.T) {
	t.Parallel()
	srv := codecentraltest.NewServer(t)
	srv.Client.CreateProject(context.Background(), core.CreateProjectInput{Key: "acme"})
	env := map[string]string{"CODE_CENTRAL_URL": srv.URL, "CODE_CENTRAL_PROJECT": "acme"}

	wantError(t, run(t, env, "", "get-ticket", "9"), core.NotFound)
	e := wantError(t, run(t, env, "", "create-ticket", "--spec", "1", "--title", "T"), core.Invalid)
	if e.Field != "spec" {
		t.Fatalf("field %q", e.Field)
	}
	wantError(t, run(t, map[string]string{"CODE_CENTRAL_URL": srv.URL}, "", "get-ticket", "1", "--project", "nope"), core.UnknownProject)
}

func TestUsageErrorsExitOne(t *testing.T) {
	t.Parallel()
	env := map[string]string{"CODE_CENTRAL_URL": "http://127.0.0.1:1"}
	for _, args := range [][]string{
		{"frobnicate"},
		{"get-ticket", "1"}, // no Project
		{"get-ticket", "--project", "acme"},
		{"create-spec", "--project", "acme", "--nope"},
	} {
		if r := run(t, env, "", args...); r.code != 1 || r.stderr == "" {
			t.Errorf("%v: exit %d, stderr %q", args, r.code, r.stderr)
		}
	}
}

func TestACommandsHelpIsItsUsageAndItsOperationsDocumentation(t *testing.T) {
	t.Parallel()
	env := map[string]string{"CODE_CENTRAL_URL": "http://127.0.0.1:1"}
	for _, tc := range []struct {
		args []string
		want string
	}{
		{[]string{"approve", "--help"}, "usage: code-central approve <id> [--full]\n\n" + ops.Approve.Doc + "\n"},
		{[]string{"comment", "3", "-h"}, "usage: code-central comment <id> [--body <md> | --file <path|->] [--full]\n\n" + ops.Comment.Doc + "\n"},
		{[]string{"wait", "--help"}, "usage: code-central wait <id>  wait for the Ticket to leave Needs Answer: exit 0 when Answered, 3 when it left for another State\n"},
	} {
		r := run(t, env, "", tc.args...)
		if r.code != 0 || r.stdout != tc.want || r.stderr != "" {
			t.Errorf("%v: exit %d, stdout\n%s\nwant\n%s\nstderr %q", tc.args, r.code, r.stdout, tc.want, r.stderr)
		}
	}
}

func TestServiceUnavailableNamesHowToStartIt(t *testing.T) {
	t.Parallel()
	url := "http://" + freeAddr(t)
	r := run(t, map[string]string{"CODE_CENTRAL_URL": url, "CODE_CENTRAL_PROJECT": "acme"}, "", "get-ticket", "1")
	e := wantError(t, r, core.ServiceUnavailable)
	for _, want := range []string{url, "code-central serve", "code-central service install"} {
		if !strings.Contains(e.Message, want) {
			t.Errorf("message %q doesn't mention %q", e.Message, want)
		}
	}
}

func TestDefaultURL(t *testing.T) {
	t.Parallel()
	if client.DefaultURL != "http://localhost:7733" {
		t.Fatal(client.DefaultURL)
	}
}

// freeAddr returns a loopback address nothing is listening on.
func freeAddr(t *testing.T) string {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := l.Addr().String()
	l.Close()
	return addr
}

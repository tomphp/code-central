package cli_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/tomoram/code-central/cli"
	"github.com/tomoram/code-central/client"
	"github.com/tomoram/code-central/codecentraltest"
	"github.com/tomoram/code-central/core"
)

type result struct {
	code           int
	stdout, stderr string
}

// run runs one command in-process with only the given environment.
func run(t *testing.T, env map[string]string, stdin string, args ...string) result {
	t.Helper()
	var out, errOut bytes.Buffer
	code := cli.Run(context.Background(), args, cli.Env{
		Stdin:  strings.NewReader(stdin),
		Stdout: &out,
		Stderr: &errOut,
		Getenv: func(k string) string { return env[k] },
	})
	return result{code, out.String(), errOut.String()}
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
	srv := codecentraltest.NewServer(t)
	if _, err := srv.Client.CreateProject(context.Background(), core.CreateProjectInput{Key: "acme"}); err != nil {
		t.Fatal(err)
	}
	env := map[string]string{"CODE_CENTRAL_URL": srv.URL, "CODE_CENTRAL_PROJECT": "acme"}

	spec := decode[core.SpecView](t, run(t, env, "# Login\n\nUsers log in.\n", "create-spec", "--title", "Login", "--file", "-"))
	if spec.ID != "acme-1" || spec.Body != "# Login\n\nUsers log in.\n" {
		t.Fatalf("spec: %+v", spec)
	}

	tk := decode[core.TicketView](t, run(t, env, "", "create-ticket", "--spec", "acme-1", "--title", "Form",
		"--state", "Ready", "--label", "ui", "--label", "auth", "--description", "Build it."))
	if tk.ID != "acme-2" || tk.State != "Ready" || len(tk.Labels) != 2 || tk.Description != "Build it." {
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

func TestServiceUnavailableNamesHowToStartIt(t *testing.T) {
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

// serving is a `code-central serve` running in-process.
type serving struct {
	url    string
	cancel context.CancelFunc
	done   chan result
}

// startServe runs serve on a test-chosen address (never 7733) and waits
// until it listens, or returns its result if it exits first.
func startServe(t *testing.T, storeDir string, addrs ...string) (*serving, *result) {
	t.Helper()
	if len(addrs) == 0 {
		addrs = []string{"127.0.0.1:0"}
	}
	ctx, cancel := context.WithCancel(context.Background())
	listening := make(chan []string, 1)
	done := make(chan result, 1)
	go func() {
		var out, errOut bytes.Buffer
		code := cli.Run(ctx, []string{"serve", "--store", storeDir}, cli.Env{
			Stdin: strings.NewReader(""), Stdout: &out, Stderr: &errOut,
			Getenv:      func(string) string { return "" },
			ServeAddrs:  addrs,
			OnListening: func(a []string) { listening <- a },
		})
		done <- result{code, out.String(), errOut.String()}
	}()
	select {
	case a := <-listening:
		s := &serving{url: "http://" + a[0], cancel: cancel, done: done}
		t.Cleanup(func() { s.stop(t) })
		return s, nil
	case r := <-done:
		cancel()
		return nil, &r
	case <-time.After(5 * time.Second):
		cancel()
		t.Fatal("serve neither listened nor exited")
		return nil, nil
	}
}

func (s *serving) stop(t *testing.T) {
	if s.cancel == nil {
		return
	}
	s.cancel()
	s.cancel = nil
	if r := <-s.done; r.code != 0 {
		t.Errorf("serve exited %d: %s", r.code, r.stderr)
	}
}

func TestATicketSurvivesARestart(t *testing.T) {
	dir := t.TempDir()
	s, failed := startServe(t, dir)
	if failed != nil {
		t.Fatalf("serve: %+v", *failed)
	}
	if _, err := client.New(s.url).CreateProject(context.Background(), core.CreateProjectInput{Key: "acme", DisplayName: "Acme"}); err != nil {
		t.Fatal(err)
	}
	env := map[string]string{"CODE_CENTRAL_URL": s.url, "CODE_CENTRAL_PROJECT": "acme"}
	decode[core.SpecView](t, run(t, env, "", "create-spec", "--title", "Login"))
	decode[core.TicketView](t, run(t, env, "", "create-ticket", "--spec", "1", "--title", "Form", "--state", "Ready"))
	s.stop(t)

	for _, f := range []string{"store.yaml", "projects/acme/project.yaml", "projects/acme/specs/1.md", "projects/acme/tickets/2.yaml"} {
		if _, err := os.Stat(filepath.Join(dir, f)); err != nil {
			t.Errorf("missing %s: %v", f, err)
		}
	}

	s2, failed := startServe(t, dir)
	if failed != nil {
		t.Fatalf("restart: %+v", *failed)
	}
	env["CODE_CENTRAL_URL"] = s2.url
	got := decode[core.TicketView](t, run(t, env, "", "get-ticket", "acme-2"))
	if got.Title != "Form" || got.State != "Ready" {
		t.Fatalf("after restart: %+v", got)
	}
	// the counter is derived again at startup
	if next := decode[core.SpecView](t, run(t, env, "", "create-spec", "--title", "Next")); next.ID != "acme-3" {
		t.Fatalf("after restart, next ID %s", next.ID)
	}
}

func TestServeRefusesATakenPort(t *testing.T) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	_, failed := startServe(t, t.TempDir(), l.Addr().String())
	if failed == nil {
		t.Fatal("serve started on a taken port")
	}
	if failed.code != 1 || !strings.Contains(failed.stderr, l.Addr().String()) || !strings.Contains(failed.stderr, "in use") {
		t.Fatalf("exit %d, stderr %q", failed.code, failed.stderr)
	}
}

func TestSecondServeOnTheSameStoreRefuses(t *testing.T) {
	dir := t.TempDir()
	if _, failed := startServe(t, dir); failed != nil {
		t.Fatalf("first serve: %+v", *failed)
	}
	_, failed := startServe(t, dir)
	if failed == nil {
		t.Fatal("second serve started")
	}
	if failed.code != 1 || !strings.Contains(failed.stderr, "locked") {
		t.Fatalf("exit %d, stderr %q", failed.code, failed.stderr)
	}
}

func TestServeRefusesAnInvalidStoreNamingFileAndField(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "store.yaml"), []byte("format: 1\n"), 0o644)
	os.MkdirAll(filepath.Join(dir, "projects", "acme"), 0o755)
	os.WriteFile(filepath.Join(dir, "projects", "acme", "project.yaml"), []byte("key: acme\ncolour: red\n"), 0o644)
	_, failed := startServe(t, dir)
	if failed == nil {
		t.Fatal("serve started on an invalid store")
	}
	if failed.code != 1 || !strings.Contains(failed.stderr, "projects/acme/project.yaml") || !strings.Contains(failed.stderr, "colour") {
		t.Fatalf("exit %d, stderr %q", failed.code, failed.stderr)
	}
}

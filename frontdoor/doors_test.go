package frontdoor_test

// The cross-door suite: every core operation through every front door
// that exposes it (MCP, the HTTP API, the CLI), one successful and one
// rejected call each. It tests translation only: the domain rules live
// in core's tests.

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"slices"
	"sort"
	"strings"
	"testing"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/tomoram/code-central/cli"
	"github.com/tomoram/code-central/codecentraltest"
	"github.com/tomoram/code-central/core"
	"github.com/tomoram/code-central/httpapi"
)

// A call is one invocation of an operation, spelled for each door. A door
// left empty does not expose the operation.
type call struct {
	mcp  map[string]any // tool arguments
	http httpCall
	cli  []string // arguments after the command name
}

type httpCall struct{ method, path, body string }

// An opCase is an operation's two calls. check inspects a successful
// result's JSON; code is the rejected call's error code. setup, when set,
// prepares the fixture first.
type opCase struct {
	setup func(t *testing.T, srv *codecentraltest.Server)
	ok    call
	check func(t *testing.T, out map[string]any)
	bad   call
	code  core.Code
}

// Every case runs against a fresh server holding Project acme with Spec
// acme-1 and Ticket acme-2, and Project other.
var cases = map[string]opCase{
	"create_project": {
		ok:    call{http: httpCall{"POST", "/api/projects/create_project", `{"key":"new","display_name":"New"}`}},
		check: field("key", "new"),
		bad:   call{http: httpCall{"POST", "/api/projects/create_project", `{"key":"Bad Key"}`}},
		code:  core.Invalid,
	},
	"get_project": {
		ok:    call{http: httpCall{"GET", "/api/projects/acme", ""}},
		check: field("key", "acme"),
		bad:   call{http: httpCall{"GET", "/api/projects/nope", ""}},
		code:  core.UnknownProject,
	},
	"create_spec": {
		ok: call{
			mcp:  map[string]any{"title": "Search", "body": "Find things."},
			http: httpCall{"POST", "/api/p/acme/create_spec", `{"title":"Search","body":"Find things."}`},
			cli:  []string{"--title", "Search", "--body", "Find things."},
		},
		check: field("id", "acme-3"),
		bad: call{
			mcp:  map[string]any{"title": ""},
			http: httpCall{"POST", "/api/p/acme/create_spec", `{"title":""}`},
			cli:  []string{"--title", ""},
		},
		code: core.Invalid,
	},
	"create_ticket": {
		ok: call{
			mcp:  map[string]any{"spec": 1, "title": "Form", "state": "Ready", "labels": []string{"ui"}, "description": "Build it."},
			http: httpCall{"POST", "/api/p/acme/create_ticket", `{"spec":1,"title":"Form","state":"Ready","labels":["ui"],"description":"Build it."}`},
			cli:  []string{"--spec", "1", "--title", "Form", "--state", "Ready", "--label", "ui", "--description", "Build it."},
		},
		check: field("id", "acme-3"),
		bad: call{
			mcp:  map[string]any{"spec": "acme-9", "title": "Form"},
			http: httpCall{"POST", "/api/p/acme/create_ticket", `{"spec":"acme-9","title":"Form"}`},
			cli:  []string{"--spec", "acme-9", "--title", "Form"},
		},
		code: core.Invalid,
	},
	"get_ticket": {
		ok: call{
			mcp:  map[string]any{"ticket": "acme-2"},
			http: httpCall{"GET", "/api/p/acme/tickets/2", ""},
			cli:  []string{"acme-2"},
		},
		check: field("title", "Login form"),
		bad: call{
			mcp:  map[string]any{"ticket": 9},
			http: httpCall{"GET", "/api/p/acme/tickets/9", ""},
			cli:  []string{"9"},
		},
		code: core.NotFound,
	},
	"find_tickets": {
		ok: call{
			mcp:  map[string]any{"state": []string{"Draft"}, "spec": "acme-1"},
			http: httpCall{"GET", "/api/p/acme/tickets?state=Draft&spec=acme-1", ""},
			cli:  []string{"--state", "Draft", "--spec", "acme-1"},
		},
		check: func(t *testing.T, out map[string]any) {
			rows, _ := out["rows"].([]any)
			if len(rows) != 1 || rows[0].(map[string]any)["id"] != "acme-2" {
				t.Fatalf("rows %v", out["rows"])
			}
		},
		bad: call{
			mcp:  map[string]any{"spec": "other-1"},
			http: httpCall{"GET", "/api/p/acme/tickets?spec=other-1", ""},
			cli:  []string{"--spec", "other-1"},
		},
		code: core.Invalid,
	},

	"claim": {
		setup: readyTicket,
		ok: call{
			mcp:  map[string]any{"session_id": "s1"},
			http: httpCall{"POST", "/api/p/acme/claim", `{"session_id":"s1"}`},
			cli:  []string{"--session", "s1"},
		},
		check: func(t *testing.T, out map[string]any) {
			field("claim_id", "claim-1")(t, out)
			field("id", "acme-3")(t, out["ticket"].(map[string]any))
		},
		bad: call{
			mcp:  map[string]any{"spec": 1, "ticket": 3},
			http: httpCall{"POST", "/api/p/acme/claim", `{"spec":1,"ticket":3}`},
			cli:  []string{"--spec", "1", "--ticket", "3"},
		},
		code: core.Invalid,
	},
	"finish": {
		setup: claimedTicket,
		ok: call{
			mcp:  map[string]any{"ticket": "acme-3", "claim_id": "claim-1", "summary": "Done."},
			http: httpCall{"POST", "/api/p/acme/tickets/3/finish", `{"claim_id":"claim-1","summary":"Done."}`},
			cli:  []string{"acme-3", "--claim", "claim-1", "--summary", "Done."},
		},
		check: func(t *testing.T, out map[string]any) {
			field("next_step", "stop working this Ticket")(t, out)
			field("state", "In Review")(t, out["ticket"].(map[string]any))
		},
		bad: call{
			mcp:  map[string]any{"ticket": "acme-3", "claim_id": "claim-9", "summary": "Done."},
			http: httpCall{"POST", "/api/p/acme/tickets/3/finish", `{"claim_id":"claim-9","summary":"Done."}`},
			cli:  []string{"acme-3", "--claim", "claim-9", "--summary", "Done."},
		},
		code: core.StaleClaim,
	},
	"accept": {
		setup: ticketInReview,
		ok:    call{http: httpCall{"POST", "/api/p/acme/tickets/3/accept", `{}`}},
		check: field("state", "Done"),
		bad:   call{http: httpCall{"POST", "/api/p/acme/tickets/3/accept", `{}`}},
		code:  core.ActionNotAllowed,
	},
	"reject": {
		setup: ticketInReview,
		ok:    call{http: httpCall{"POST", "/api/p/acme/tickets/3/reject", `{"comment":"Not yet."}`}},
		check: field("state", "Ready"),
		bad:   call{http: httpCall{"POST", "/api/p/acme/tickets/3/reject", `{"comment":""}`}},
		code:  core.Invalid,
	},
}

// readyTicket adds Ticket acme-3, Ready.
func readyTicket(t *testing.T, srv *codecentraltest.Server) {
	t.Helper()
	must(t, func() error {
		_, err := srv.Core.CreateTicket(context.Background(), "acme", core.CreateTicketInput{Spec: "1", Title: "Logout", State: "Ready"})
		return err
	})
}

// claimedTicket adds acme-3 and claims it as claim-1.
func claimedTicket(t *testing.T, srv *codecentraltest.Server) {
	t.Helper()
	readyTicket(t, srv)
	must(t, func() error { _, err := srv.Core.Claim(context.Background(), "acme", core.ClaimInput{}); return err })
}

// ticketInReview adds acme-3, claims it and finishes it.
func ticketInReview(t *testing.T, srv *codecentraltest.Server) {
	t.Helper()
	claimedTicket(t, srv)
	must(t, func() error {
		_, err := srv.Core.Finish(context.Background(), "acme", core.FinishInput{Ticket: "3", ClaimID: "claim-1", Summary: "Done."})
		return err
	})
}

func field(name, want string) func(*testing.T, map[string]any) {
	return func(t *testing.T, out map[string]any) {
		t.Helper()
		if out[name] != want {
			t.Fatalf("%s = %v, want %q in %v", name, out[name], want, out)
		}
	}
}

// newServer starts the fixture, then runs each setup given.
func newServer(t *testing.T, setups ...func(*testing.T, *codecentraltest.Server)) *codecentraltest.Server {
	t.Helper()
	srv := codecentraltest.NewServer(t)
	ctx := context.Background()
	must(t, func() error { _, err := srv.Core.CreateProject(ctx, core.CreateProjectInput{Key: "acme"}); return err })
	must(t, func() error { _, err := srv.Core.CreateProject(ctx, core.CreateProjectInput{Key: "other"}); return err })
	must(t, func() error {
		_, err := srv.Core.CreateSpec(ctx, "acme", core.CreateSpecInput{Title: "Login"})
		return err
	})
	must(t, func() error {
		_, err := srv.Core.CreateTicket(ctx, "acme", core.CreateTicketInput{Spec: "1", Title: "Login form"})
		return err
	})
	for _, setup := range setups {
		if setup != nil {
			setup(t, srv)
		}
	}
	return srv
}

func must(t *testing.T, f func() error) {
	t.Helper()
	if err := f(); err != nil {
		t.Fatal(err)
	}
}

func agentOps() []string {
	var out []string
	for _, op := range core.Operations {
		if op.Audience == core.AgentAndHuman {
			out = append(out, op.Name)
		}
	}
	sort.Strings(out)
	return out
}

func TestEveryOperationHasACase(t *testing.T) {
	for _, op := range core.Operations {
		c, ok := cases[op.Name]
		if !ok {
			t.Errorf("operation %s has no cross-door case", op.Name)
			continue
		}
		agent := op.Audience == core.AgentAndHuman
		for _, cl := range []call{c.ok, c.bad} {
			if cl.http.path == "" || agent != (cl.mcp != nil) || agent != (cl.cli != nil) {
				t.Errorf("operation %s: a call does not cover exactly its doors", op.Name)
			}
		}
	}
}

func TestMCPExposesExactlyTheAgentOperations(t *testing.T) {
	srv := newServer(t)
	var names []string
	for tool, err := range connect(t, srv, "acme").Tools(context.Background(), nil) {
		if err != nil {
			t.Fatal(err)
		}
		names = append(names, tool.Name)
		if tool.Description == "" || strings.Contains(tool.Description, "\n") {
			t.Errorf("tool %s: description %q is not one line", tool.Name, tool.Description)
		}
		if props, _ := tool.InputSchema.(map[string]any)["properties"].(map[string]any); props["project"] != nil {
			t.Errorf("tool %s takes a Project argument", tool.Name)
		}
	}
	sort.Strings(names)
	if !slices.Equal(names, agentOps()) {
		t.Fatalf("MCP tools %v, want the agent operations %v", names, agentOps())
	}
}

func TestCLIExposesExactlyTheAgentOperations(t *testing.T) {
	var ops []string
	for name, op := range cli.Commands() {
		if op == "" {
			continue
		}
		if name != strings.ReplaceAll(op, "_", "-") {
			t.Errorf("command %s exposes %s: not its kebab-case name", name, op)
		}
		ops = append(ops, op)
	}
	sort.Strings(ops)
	if !slices.Equal(ops, agentOps()) {
		t.Fatalf("CLI operations %v, want the agent operations %v", ops, agentOps())
	}
}

func TestHTTPExposesEveryOperation(t *testing.T) {
	var routes, all []string
	for name := range httpapi.Routes {
		routes = append(routes, name)
	}
	for _, op := range core.Operations {
		all = append(all, op.Name)
	}
	sort.Strings(routes)
	sort.Strings(all)
	if !slices.Equal(routes, all) {
		t.Fatalf("HTTP routes %v, want every operation %v", routes, all)
	}
}

func TestEveryDoorTranslatesEveryOperation(t *testing.T) {
	for _, op := range core.Operations {
		c := cases[op.Name]
		t.Run(op.Name+"/http", func(t *testing.T) {
			srv := newServer(t, c.setup)
			c.check(t, httpOK(t, srv, c.ok.http))
			httpRejected(t, srv, c.bad.http, c.code)
		})
		if op.Audience != core.AgentAndHuman {
			continue
		}
		t.Run(op.Name+"/mcp", func(t *testing.T) {
			srv := newServer(t, c.setup)
			s := connect(t, srv, "acme")
			c.check(t, mcpOK(t, s, op.Name, c.ok.mcp))
			mcpRejected(t, s, op.Name, c.bad.mcp, c.code)
		})
		t.Run(op.Name+"/cli", func(t *testing.T) {
			srv := newServer(t, c.setup)
			command := strings.ReplaceAll(op.Name, "_", "-")
			c.check(t, cliOK(t, srv, append([]string{command}, c.ok.cli...)))
			cliRejected(t, srv, append([]string{command}, c.bad.cli...), c.code)
		})
	}
}

func TestMCPOnAnUnknownProjectRejectsEveryTool(t *testing.T) {
	srv := newServer(t)
	s := connect(t, srv, "nope")
	for _, name := range agentOps() {
		e := mcpRejected(t, s, name, cases[name].ok.mcp, core.UnknownProject)
		if !strings.Contains(e.Message, `"nope"`) || !strings.Contains(e.Message, "code-central init") {
			t.Errorf("%s: message %q should name the key and say to run code-central init", name, e.Message)
		}
	}
}

func TestIDsFromAnotherProjectAreRejectedNamingTheConnectionsProject(t *testing.T) {
	srv := newServer(t)
	s := connect(t, srv, "acme")
	for _, e := range []core.Error{
		mcpRejected(t, s, "get_ticket", map[string]any{"ticket": "other-2"}, core.Invalid),
		cliRejected(t, srv, []string{"get-ticket", "other-2"}, core.Invalid),
	} {
		if !strings.Contains(e.Message, `"acme"`) {
			t.Errorf("message %q does not name Project acme", e.Message)
		}
	}
	// A bare number and a full ID name the same Ticket.
	for _, ref := range []any{2, "2", "acme-2"} {
		field("id", "acme-2")(t, mcpOK(t, s, "get_ticket", map[string]any{"ticket": ref}))
	}
	field("id", "acme-2")(t, cliOK(t, srv, []string{"get-ticket", "2"}))
}

func TestMCPRejectsUnknownArguments(t *testing.T) {
	srv := newServer(t)
	e := mcpRejected(t, connect(t, srv, "acme"), "create_spec", map[string]any{"title": "S", "project": "other"}, core.Invalid)
	if e.Field != "project" {
		t.Fatalf("field %q, want project", e.Field)
	}
}

func TestMCPAndTheCLIAlwaysWriteAsTheAgent(t *testing.T) {
	srv := newServer(t, readyTicket, readyTicket) // acme-3 and acme-4
	viaMCP := mcpOK(t, connect(t, srv, "acme"), "claim", map[string]any{})
	viaCLI := cliOK(t, srv, []string{"claim"})
	for door, out := range map[string]map[string]any{"mcp": viaMCP, "cli": viaCLI} {
		moves := out["ticket"].(map[string]any)["moves"].([]any)
		if a := moves[0].(map[string]any)["actor"]; a != "agent" {
			t.Errorf("%s: the claim was made as %v, want agent", door, a)
		}
	}
}

// --- HTTP ---

func httpDo(t *testing.T, srv *codecentraltest.Server, c httpCall) (int, []byte) {
	t.Helper()
	req, err := http.NewRequest(c.method, srv.URL+c.path, strings.NewReader(c.body))
	if err != nil {
		t.Fatal(err)
	}
	// The suite calls the HTTP API as the UI does, as the human.
	req.Header.Set(httpapi.ActorHeader, string(core.Human))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, b
}

func httpOK(t *testing.T, srv *codecentraltest.Server, c httpCall) map[string]any {
	t.Helper()
	status, b := httpDo(t, srv, c)
	if status != http.StatusOK {
		t.Fatalf("status %d: %s", status, b)
	}
	return object(t, b)
}

func httpRejected(t *testing.T, srv *codecentraltest.Server, c httpCall, code core.Code) {
	t.Helper()
	status, b := httpDo(t, srv, c)
	e := errorJSON(t, b, code)
	if status != httpapi.Status(code) {
		t.Fatalf("status %d, want %d for %s", status, httpapi.Status(code), e.Code)
	}
}

// --- MCP ---

func connect(t *testing.T, srv *codecentraltest.Server, key string) *sdk.ClientSession {
	t.Helper()
	c := sdk.NewClient(&sdk.Implementation{Name: "doors-test", Version: "v0"}, nil)
	s, err := c.Connect(context.Background(), &sdk.StreamableClientTransport{
		Endpoint: srv.URL + "/p/" + key + "/mcp", DisableStandaloneSSE: true, MaxRetries: -1,
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	// The stateless revision, as Claude Code negotiates it.
	if v := s.InitializeResult().ProtocolVersion; v != "2026-07-28" {
		t.Fatalf("negotiated MCP %s, want the stateless 2026-07-28", v)
	}
	return s
}

func mcpCall(t *testing.T, s *sdk.ClientSession, tool string, args map[string]any) (*sdk.CallToolResult, []byte) {
	t.Helper()
	res, err := s.CallTool(context.Background(), &sdk.CallToolParams{Name: tool, Arguments: args})
	if err != nil {
		t.Fatalf("%s: protocol error %v", tool, err)
	}
	if len(res.Content) != 1 {
		t.Fatalf("%s: %d content items, want 1", tool, len(res.Content))
	}
	text, ok := res.Content[0].(*sdk.TextContent)
	if !ok {
		t.Fatalf("%s: content is %T, want text", tool, res.Content[0])
	}
	return res, []byte(text.Text)
}

func mcpOK(t *testing.T, s *sdk.ClientSession, tool string, args map[string]any) map[string]any {
	t.Helper()
	res, b := mcpCall(t, s, tool, args)
	if res.IsError {
		t.Fatalf("%s: isError: %s", tool, b)
	}
	return object(t, b)
}

func mcpRejected(t *testing.T, s *sdk.ClientSession, tool string, args map[string]any, code core.Code) core.Error {
	t.Helper()
	res, b := mcpCall(t, s, tool, args)
	if !res.IsError {
		t.Fatalf("%s: isError not set: %s", tool, b)
	}
	return errorJSON(t, b, code)
}

// --- CLI ---

func cliRun(t *testing.T, srv *codecentraltest.Server, args []string) (int, []byte, []byte) {
	t.Helper()
	env := map[string]string{"CODE_CENTRAL_URL": srv.URL, "CODE_CENTRAL_PROJECT": "acme"}
	var out, errOut bytes.Buffer
	code := cli.Run(context.Background(), args, cli.Env{
		Stdin: strings.NewReader(""), Stdout: &out, Stderr: &errOut,
		Getenv: func(k string) string { return env[k] },
	})
	return code, out.Bytes(), errOut.Bytes()
}

func cliOK(t *testing.T, srv *codecentraltest.Server, args []string) map[string]any {
	t.Helper()
	code, out, errOut := cliRun(t, srv, args)
	if code != 0 {
		t.Fatalf("%v: exit %d: %s", args, code, errOut)
	}
	return object(t, out)
}

func cliRejected(t *testing.T, srv *codecentraltest.Server, args []string, code core.Code) core.Error {
	t.Helper()
	exit, out, errOut := cliRun(t, srv, args)
	if exit != 2 || len(out) != 0 {
		t.Fatalf("%v: exit %d, stdout %q; want exit 2 and no output", args, exit, out)
	}
	return errorJSON(t, errOut, code)
}

// --- shapes ---

// object decodes a result; an array result is wrapped as {"rows": [...]}.
func object(t *testing.T, b []byte) map[string]any {
	t.Helper()
	var v any
	if err := json.Unmarshal(b, &v); err != nil {
		t.Fatalf("result is not JSON: %v: %s", err, b)
	}
	if rows, ok := v.([]any); ok {
		return map[string]any{"rows": rows}
	}
	m, ok := v.(map[string]any)
	if !ok {
		t.Fatalf("result is not an object or array: %s", b)
	}
	return m
}

// errorJSON checks the one error shape {code, message, allowed_actions?, field?}.
func errorJSON(t *testing.T, b []byte, code core.Code) core.Error {
	t.Helper()
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.DisallowUnknownFields()
	var e core.Error
	if err := dec.Decode(&e); err != nil {
		t.Fatalf("not the error shape: %v: %s", err, b)
	}
	if e.Code != code || e.Message == "" {
		t.Fatalf("error %s %q, want code %s with a message", e.Code, e.Message, code)
	}
	return e
}

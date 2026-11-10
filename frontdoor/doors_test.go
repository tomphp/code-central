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
	"github.com/tomoram/code-central/frontdoor"
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
	// full, on a write that acknowledges (ADR 0009), checks the record
	// the same call returns with full.
	full func(t *testing.T, out map[string]any)
	bad  call
	code core.Code
	// cliUsage says the CLI's rejected call is a usage error (exit 1):
	// the operation takes no input the service could reject.
	cliUsage bool
}

// Every case runs against a fresh server holding Project acme with Spec
// acme-1, Draft Ticket acme-2 and Ready Ticket acme-3, and Project other.
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
		check: acknowledgement("acme-4", "title", "body"),
		full: func(t *testing.T, out map[string]any) {
			field("id", "acme-4")(t, out)
			field("body", "Find things.")(t, out)
		},
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
		check: ticketAcknowledgement("acme-4", "Ready", "title", "state", "spec", "labels", "description", "claimable"),
		full:  field("id", "acme-4"),
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
		ok: call{
			mcp:  map[string]any{"session_id": "s1", "working_dir": "/w/acme-3", "resume_command": "agent resume s1"},
			http: httpCall{"POST", "/api/p/acme/claim", `{"session_id":"s1","working_dir":"/w/acme-3","resume_command":"agent resume s1"}`},
			cli:  []string{"--session", "s1", "--working-dir", "/w/acme-3", "--resume-command", "agent resume s1"},
		},
		check: func(t *testing.T, out map[string]any) {
			field("claim_id", "claim-1")(t, out)
			tk := out["ticket"].(map[string]any)
			field("id", "acme-3")(t, tk)
			// claim's Ticket is get_ticket's default, which counts the
			// session fields it leaves out (code-central-320).
			if lo := tk["left_out"].(map[string]any); lo["claims"] != float64(1) {
				t.Fatalf("left_out %v, want claims 1", lo)
			}
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
			mcp:  map[string]any{"ticket": "acme-3", "claim_id": "claim-1", "summary": "Done.", "branch": "acme-3", "commit": "abc123"},
			http: httpCall{"POST", "/api/p/acme/tickets/3/finish", `{"claim_id":"claim-1","summary":"Done.","branch":"acme-3","commit":"abc123"}`},
			cli:  []string{"acme-3", "--claim", "claim-1", "--summary", "Done.", "--branch", "acme-3", "--commit", "abc123"},
		},
		check: stopAcknowledgement("In Review", "state", "branch", "commit", "claims", "comments", "moves", "needs_me_reason"),
		full: func(t *testing.T, out map[string]any) {
			field("next_step", "stop working this Ticket")(t, out)
			tk := out["ticket"].(map[string]any)
			field("state", "In Review")(t, tk)
			field("branch", "acme-3")(t, tk)
			field("commit", "abc123")(t, tk)
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
		check: ticketAcknowledgement("acme-3", "Done", "state", "moves", "needs_me_reason"),
		full:  field("state", "Done"),
		bad:   call{http: httpCall{"POST", "/api/p/acme/tickets/3/accept", `{}`}},
		code:  core.ActionNotAllowed,
	},
	"reject": {
		setup: ticketInReview,
		ok:    call{http: httpCall{"POST", "/api/p/acme/tickets/3/reject", `{"comment":"Not yet."}`}},
		check: ticketAcknowledgement("acme-3", "Ready", "state", "comments", "moves", "claimable", "needs_me_reason"),
		full:  field("state", "Ready"),
		bad:   call{http: httpCall{"POST", "/api/p/acme/tickets/3/reject", `{"comment":""}`}},
		code:  core.Invalid,
	},
}

// readyTicket adds Ticket acme-4, Ready.
func readyTicket(t *testing.T, srv *codecentraltest.Server) {
	t.Helper()
	must(t, func() error {
		_, err := srv.Core.CreateTicket(context.Background(), "acme", core.CreateTicketInput{Spec: "1", Title: "Logout", State: "Ready"})
		return err
	})
}

// claimedTicket claims acme-3 as claim-1.
func claimedTicket(t *testing.T, srv *codecentraltest.Server) {
	t.Helper()
	must(t, func() error { _, err := srv.Core.Claim(context.Background(), "acme", core.ClaimInput{}); return err })
}

// ticketInReview claims acme-3 and finishes it.
func ticketInReview(t *testing.T, srv *codecentraltest.Server) {
	t.Helper()
	claimedTicket(t, srv)
	must(t, func() error {
		_, err := srv.Core.Finish(context.Background(), "acme", core.FinishInput{Ticket: "3", ClaimID: "claim-1", Summary: "Done."})
		return err
	})
}

// Defining work (ticket 27).
func init() {
	for name, c := range map[string]opCase{
		"edit_spec": {
			ok: call{
				mcp:  map[string]any{"spec": "acme-1", "title": "Sign in", "body": "New."},
				http: httpCall{"POST", "/api/p/acme/specs/1/edit_spec", `{"title":"Sign in","body":"New."}`},
				cli:  []string{"acme-1", "--title", "Sign in", "--body", "New."},
			},
			check: acknowledgement("acme-1", "title", "body", "edits"),
			full: func(t *testing.T, out map[string]any) {
				field("title", "Sign in")(t, out)
				field("body", "New.")(t, out)
				if edits, _ := out["edits"].([]any); len(edits) != 1 {
					t.Fatalf("edits %v", out["edits"])
				}
			},
			bad: call{
				mcp:  map[string]any{"spec": 2, "title": "X"},
				http: httpCall{"POST", "/api/p/acme/specs/2/edit_spec", `{"title":"X"}`},
				cli:  []string{"2", "--title", "X"},
			},
			code: core.NotFound,
		},
		"edit_ticket": {
			ok: call{
				mcp:  map[string]any{"ticket": 2, "title": "Form", "description": "Build it."},
				http: httpCall{"POST", "/api/p/acme/tickets/2/edit_ticket", `{"title":"Form","description":"Build it."}`},
				cli:  []string{"2", "--title", "Form", "--description", "Build it."},
			},
			check: ticketAcknowledgement("acme-2", "Draft", "title", "description", "edits"),
			full: func(t *testing.T, out map[string]any) {
				field("title", "Form")(t, out)
				field("description", "Build it.")(t, out)
			},
			bad: call{
				mcp:  map[string]any{"ticket": 2, "title": ""},
				http: httpCall{"POST", "/api/p/acme/tickets/2/edit_ticket", `{"title":""}`},
				cli:  []string{"2", "--title", ""},
			},
			code: core.Invalid,
		},
		"add_labels": {
			ok: call{
				mcp:  map[string]any{"target": "acme-2", "labels": []string{"ui"}},
				http: httpCall{"POST", "/api/p/acme/add_labels", `{"target":"acme-2","labels":["ui"]}`},
				cli:  []string{"acme-2", "ui"},
			},
			check: labels("ui"),
			bad: call{
				mcp:  map[string]any{"target": 9, "labels": []string{"ui"}},
				http: httpCall{"POST", "/api/p/acme/add_labels", `{"target":9,"labels":["ui"]}`},
				cli:  []string{"9", "ui"},
			},
			code: core.NotFound,
		},
		"remove_labels": {
			ok: call{
				mcp:  map[string]any{"target": 1, "labels": []string{"ui"}},
				http: httpCall{"POST", "/api/p/acme/remove_labels", `{"target":1,"labels":["ui"]}`},
				cli:  []string{"1", "ui"},
			},
			check: func(t *testing.T, out map[string]any) {
				field("id", "acme-1")(t, out)
				field("kind", "spec")(t, out)
				labels()(t, out)
			},
			bad: call{
				mcp:  map[string]any{"target": 1, "labels": []string{""}},
				http: httpCall{"POST", "/api/p/acme/remove_labels", `{"target":1,"labels":[""]}`},
				cli:  []string{"1", ""},
			},
			code: core.Invalid,
		},
		"set_tags": {
			ok: call{
				mcp:  map[string]any{"target": 2, "tags": map[string]string{"area": "web"}},
				http: httpCall{"POST", "/api/p/acme/set_tags", `{"target":2,"tags":{"area":"web"}}`},
				cli:  []string{"2", "area=web"},
			},
			check: func(t *testing.T, out map[string]any) {
				if tags, _ := out["tags"].(map[string]any); tags["area"] != "web" {
					t.Fatalf("tags %v", out["tags"])
				}
			},
			bad: call{
				mcp:  map[string]any{"target": 2, "tags": map[string]string{"Area": "web"}},
				http: httpCall{"POST", "/api/p/acme/set_tags", `{"target":2,"tags":{"Area":"web"}}`},
				cli:  []string{"2", "Area=web"},
			},
			code: core.Invalid,
		},
		"remove_tags": {
			ok: call{
				mcp:  map[string]any{"target": 2, "keys": []string{"area"}},
				http: httpCall{"POST", "/api/p/acme/remove_tags", `{"target":2,"keys":["area"]}`},
				cli:  []string{"2", "area"},
			},
			check: field("id", "acme-2"),
			bad: call{
				mcp:  map[string]any{"target": 2, "keys": []string{"Area"}},
				http: httpCall{"POST", "/api/p/acme/remove_tags", `{"target":2,"keys":["Area"]}`},
				cli:  []string{"2", "Area"},
			},
			code: core.Invalid,
		},
		"comment": {
			ok: call{
				mcp:  map[string]any{"ticket": 3, "body": "Looks fine."},
				http: httpCall{"POST", "/api/p/acme/tickets/3/comment", `{"body":"Looks fine."}`},
				cli:  []string{"3", "--body", "Looks fine."},
			},
			check: ticketAcknowledgement("acme-3", "Ready", "comments"),
			full: func(t *testing.T, out map[string]any) {
				cs, _ := out["comments"].([]any)
				if len(cs) != 1 || cs[0].(map[string]any)["body"] != "Looks fine." {
					t.Fatalf("comments %v", out["comments"])
				}
			},
			bad: call{
				mcp:  map[string]any{"ticket": 3, "body": ""},
				http: httpCall{"POST", "/api/p/acme/tickets/3/comment", `{"body":""}`},
				cli:  []string{"3", "--body", ""},
			},
			code: core.Invalid,
		},
		"approve": {
			ok: call{
				mcp:  map[string]any{"ticket": "acme-2"},
				http: httpCall{"POST", "/api/p/acme/tickets/2/approve", `{}`},
				cli:  []string{"acme-2"},
			},
			check: ticketAcknowledgement("acme-2", "Ready", "state", "moves", "claimable"),
			full:  field("state", "Ready"),
			bad: call{
				mcp:  map[string]any{"ticket": 3},
				http: httpCall{"POST", "/api/p/acme/tickets/3/approve", ``},
				cli:  []string{"3"},
			},
			code: core.ActionNotAllowed,
		},
		"defer": {
			ok: call{
				mcp:  map[string]any{"ticket": 3, "reason": "Not yet."},
				http: httpCall{"POST", "/api/p/acme/tickets/3/defer", `{"reason":"Not yet."}`},
				cli:  []string{"3", "--reason", "Not yet."},
			},
			check: ticketAcknowledgement("acme-3", "Draft", "state", "comments", "moves", "claimable"),
			full:  field("state", "Draft"),
			bad: call{
				mcp:  map[string]any{"ticket": 3},
				http: httpCall{"POST", "/api/p/acme/tickets/3/defer", `{}`},
				cli:  []string{"3"},
			},
			code: core.Invalid,
		},
	} {
		cases[name] = c
	}
}

// The published workflow (ticket 26).
func init() {
	cases["get_workflow"] = opCase{
		ok: call{
			mcp:  map[string]any{},
			http: httpCall{"GET", "/api/workflow", ""},
			cli:  []string{},
		},
		check: func(t *testing.T, out map[string]any) {
			states, _ := out["states"].([]any)
			actions, _ := out["actions"].([]any)
			if len(states) != 10 || len(actions) != len(core.Workflow) || out["prose"] == "" {
				t.Fatalf("workflow %v", out)
			}
		},
		bad: call{
			mcp:  map[string]any{"ticket": 2},
			http: httpCall{"GET", "/api/workflow?ticket=2", ""},
			cli:  []string{"2"},
		},
		code:     core.Invalid,
		cliUsage: true,
	}
}

func TestMCPSendsTheInstructions(t *testing.T) {
	if got := connect(t, newServer(t), "acme").InitializeResult().Instructions; got != core.Instructions {
		t.Fatalf("instructions %q, want core.Instructions", got)
	}
}

func TestTheWorkflowCommandIsGetWorkflow(t *testing.T) {
	srv := newServer(t)
	a, b := cliOK(t, srv, []string{"workflow"}), cliOK(t, srv, []string{"get-workflow"})
	if a["prose"] == nil || a["prose"] != b["prose"] {
		t.Fatal("code-central workflow should print get_workflow")
	}
}

// labels checks a result's Labels are exactly want.
func labels(want ...string) func(*testing.T, map[string]any) {
	return func(t *testing.T, out map[string]any) {
		t.Helper()
		got, _ := out["labels"].([]any)
		if len(got) != len(want) {
			t.Fatalf("labels %v, want %v", out["labels"], want)
		}
		for i := range want {
			if got[i] != want[i] {
				t.Fatalf("labels %v, want %v", out["labels"], want)
			}
		}
	}
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
	return newServerWith(t, nil, setups...)
}

// newServerWith is newServer started with opts.
func newServerWith(t *testing.T, opts []codecentraltest.Option, setups ...func(*testing.T, *codecentraltest.Server)) *codecentraltest.Server {
	t.Helper()
	srv := codecentraltest.NewServer(t, opts...)
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
	must(t, func() error {
		_, err := srv.Core.CreateTicket(ctx, "acme", core.CreateTicketInput{Spec: "1", Title: "Login API", State: "Ready"})
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
			if c.cliUsage {
				if exit, _, _ := cliRun(t, srv, append([]string{command}, c.bad.cli...)); exit != 1 {
					t.Fatalf("%v: exit %d, want the usage error 1", c.bad.cli, exit)
				}
				return
			}
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

func TestIDsOfTheWrongKindAreRejectedNamingTheOperationToUse(t *testing.T) {
	srv := newServer(t) // acme-1 is a Spec, acme-2 a Ticket of it
	s := connect(t, srv, "acme")
	for want, errs := range map[string][]core.Error{
		"acme-1 is a Spec; use get_spec": {
			mcpRejected(t, s, "get_ticket", map[string]any{"ticket": "acme-1"}, core.NotFound),
			httpRejected(t, srv, httpCall{"GET", "/api/p/acme/tickets/1", ""}, core.NotFound),
			cliRejected(t, srv, []string{"get-ticket", "acme-1"}, core.NotFound),
		},
		"acme-2 is a Ticket of Spec acme-1; use get_ticket": {
			mcpRejected(t, s, "get_spec", map[string]any{"spec": "acme-2"}, core.NotFound),
			httpRejected(t, srv, httpCall{"GET", "/api/p/acme/specs/2", ""}, core.NotFound),
			cliRejected(t, srv, []string{"get-spec", "acme-2"}, core.NotFound),
		},
	} {
		for i, e := range errs {
			if e.Message != want {
				t.Errorf("%s: message %q, want %q", []string{"mcp", "http", "cli"}[i], e.Message, want)
			}
		}
	}
}

func TestATicketGivenAsSpecIsRejectedNamingTheFieldAndItsSpec(t *testing.T) {
	srv := newServer(t) // acme-1 is a Spec, acme-2 a Ticket of it
	s := connect(t, srv, "acme")
	want := "acme-2 is a Ticket of Spec acme-1; pass acme-1 as spec"
	for i, e := range []core.Error{
		mcpRejected(t, s, "create_ticket", map[string]any{"spec": "acme-2", "title": "Form"}, core.Invalid),
		httpRejected(t, srv, httpCall{"POST", "/api/p/acme/create_ticket", `{"spec":"acme-2","title":"Form"}`}, core.Invalid),
		cliRejected(t, srv, []string{"create-ticket", "--spec", "acme-2", "--title", "Form"}, core.Invalid),
	} {
		if e.Field != "spec" || e.Message != want {
			t.Errorf("%s: field %q message %q, want spec %q", []string{"mcp", "http", "cli"}[i], e.Field, e.Message, want)
		}
	}
}

func TestMCPRejectsUnknownArguments(t *testing.T) {
	srv := newServer(t)
	e := mcpRejected(t, connect(t, srv, "acme"), "create_spec", map[string]any{"title": "S", "project": "other"}, core.Invalid)
	if e.Field != "project" {
		t.Fatalf("field %q, want project", e.Field)
	}
}

func TestMCPAndTheCLIAlwaysWriteAsTheAgent(t *testing.T) {
	srv := newServer(t, readyTicket) // acme-3 and acme-4 are Ready
	viaMCP := mcpOK(t, connect(t, srv, "acme"), "claim", map[string]any{})
	viaCLI := cliOK(t, srv, []string{"claim"})
	for door, out := range map[string]map[string]any{"mcp": viaMCP, "cli": viaCLI} {
		// claim's Ticket leaves its moves out, so read the whole Ticket.
		tk, err := srv.Client.GetTicket(context.Background(), "acme", core.Ref(out["ticket"].(map[string]any)["id"].(string)))
		if err != nil {
			t.Fatal(err)
		}
		if a := tk.Moves[0].Actor; a != "agent" {
			t.Errorf("%s: the claim was made as %v, want agent", door, a)
		}
	}
}

// --- HTTP ---

func httpDo(t *testing.T, srv *codecentraltest.Server, c httpCall) (int, []byte) {
	t.Helper()
	// The suite calls the HTTP API as the UI does, as the human, except
	// for an Integration's operations (ticket 42).
	actor := core.Human
	if strings.Contains(c.path, "/integration/") {
		actor = core.Integration
	}
	return httpDoAs(t, srv, actor, c)
}

// httpDoAs calls the HTTP API as the given Actor.
func httpDoAs(t *testing.T, srv *codecentraltest.Server, actor core.Actor, c httpCall) (int, []byte) {
	t.Helper()
	req, err := http.NewRequest(c.method, srv.URL+c.path, strings.NewReader(c.body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set(frontdoor.ActorHeader, string(actor))
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

func httpRejected(t *testing.T, srv *codecentraltest.Server, c httpCall, code core.Code) core.Error {
	t.Helper()
	status, b := httpDo(t, srv, c)
	e := errorJSON(t, b, code)
	if status != httpapi.Status(code) {
		t.Fatalf("status %d, want %d for %s", status, httpapi.Status(code), e.Code)
	}
	return e
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

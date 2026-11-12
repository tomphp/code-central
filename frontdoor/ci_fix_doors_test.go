package frontdoor_test

import (
	"context"
	"encoding/json"
	"slices"
	"strings"
	"testing"

	"github.com/tomoram/code-central/codecentraltest"
	"github.com/tomoram/code-central/core"
	"github.com/tomoram/code-central/store"
)

// Send back to fix and New fix Ticket (code-central-264): the failure
// options that fix forward. send_back gains the human's row from
// Accepted to Ready; new_fix_ticket, the human's, opens a Ready Ticket on
// the CI Fixes Spec and leaves the failed one waiting (code-central-433).
func init() {
	cases["new_fix_ticket"] = opCase{
		setup: failedCI,
		ok:    call{http: newFixTicket("3", fixNote)},
		check: ticketAcknowledgement("acme-5", "Ready", "title", "state", "spec", "description", "claimable"),
		full: func(t *testing.T, out map[string]any) {
			field("id", "acme-5")(t, out)
			field("state", "Ready")(t, out)
		},
		bad:  call{http: newFixTicket("2", fixNote)},
		code: core.ActionNotAllowed,
	}
}

const fixNote = "CI failed at abc123: [CI](https://github.com/acme/web/actions/runs/1), failed jobs: test."

func newFixTicket(n, note string) httpCall {
	return httpCall{"POST", "/api/p/acme/tickets/" + n + "/new_fix_ticket", `{"note":` + quote(note) + `}`}
}

func sendBack(n, note string) httpCall {
	return httpCall{"POST", "/api/p/acme/tickets/" + n + "/send_back", `{"note":` + quote(note) + `}`}
}

func quote(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}

func TestACIFailedTicketIsOfferedBothFixesUnderMergeAndBranch(t *testing.T) {
	for method, want := range map[string][]string{
		`{"method":"merge","push":true,"remote":"origin"}`:  {"rerun_ci", "land_anyway", "send_back", "new_fix_ticket"},
		`{"method":"branch","push":true,"remote":"origin"}`: {"rerun_ci", "send_back", "new_fix_ticket"},
	} {
		srv := newServer(t, failedCI)
		httpOK(t, srv, setLandingMethod(method))
		if got := ticket3(t, srv).CIFailureOptions; !slices.Equal(got, want) {
			t.Errorf("under %s options %v, want %v", method, got, want)
		}
	}
}

func TestTheHumanSendsACIFailedTicketBackToFix(t *testing.T) {
	srv := newServer(t, failedCI)
	out := httpOK(t, srv, sendBack("3", fixNote))
	ticketAcknowledgement("acme-3", "Ready", "state", "ci_item", "ci_failure_options", "comments", "moves", "claimable", "needs_me_reason")(t, out)

	tk := ticket3(t, srv)
	if m := tk.Moves[len(tk.Moves)-1]; m.From != store.Accepted || m.To != store.Ready || m.Actor != string(core.Human) {
		t.Fatalf("move %+v, want the human's Accepted to Ready", m)
	}
	if c := tk.Comments[len(tk.Comments)-1]; c.Body != fixNote || c.Actor != string(core.Human) {
		t.Fatalf("comment %+v, want the human's note", c)
	}
	if row, ok := needsMeRowOf(t, srv, "acme-3"); ok {
		t.Fatalf("row %+v, want acme-3 out of Needs Me once sent back", row)
	}
	// Its branch and pushed commit stay, for the next Claim and the CI
	// result of its re-accept.
	if tk.Branch != "acme-3" || tk.PushedCommit == nil || tk.PushedCommit.Commit != "abc123" {
		t.Fatalf("branch %q, pushed commit %+v, want both kept", tk.Branch, tk.PushedCommit)
	}
}

func TestTheHumansSendBackFromAcceptedNeedsANote(t *testing.T) {
	srv := newServer(t, failedCI)
	httpRejected(t, srv, sendBack("3", " "), core.Invalid)
	if s := ticket3(t, srv).State; s != store.Accepted {
		t.Fatalf("%s, want it left Accepted", s)
	}
}

// The workflow gains the human's row; the agent keeps only its conflict
// row.
func TestTheWorkflowHasTheHumansSendBackFromAccepted(t *testing.T) {
	w, err := newServer(t).Core.GetWorkflow(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	var human, agent [][]store.State
	for _, a := range w.Actions {
		if a.Name != "send_back" {
			continue
		}
		switch a.Who {
		case core.HumanActor:
			human = append(human, a.From)
		case core.AgentActor:
			agent = append(agent, a.From)
			if !strings.Contains(a.Requires, "conflict") {
				t.Errorf("agent row requires %q, want the conflict row", a.Requires)
			}
		}
	}
	if !slices.ContainsFunc(human, func(f []store.State) bool { return slices.Equal(f, []store.State{store.Accepted}) }) {
		t.Errorf("human rows from %v, want one from Accepted", human)
	}
	if len(agent) != 1 || !slices.Equal(agent[0], []store.State{store.Accepted}) {
		t.Errorf("agent rows from %v, want only the conflict row from Accepted", agent)
	}
}

func TestNewFixTicketOpensAReadyTicketOnTheCIFixesSpecAndLeavesTheFailedOneWaiting(t *testing.T) {
	srv := newServer(t, failedCI)
	httpOK(t, srv, newFixTicket("3", fixNote))

	fix, err := srv.Client.GetTicket(context.Background(), "acme", "acme-5")
	if err != nil {
		t.Fatal(err)
	}
	if fix.State != store.Ready || fix.Spec.ID != "acme-4" || fix.Spec.Title != "CI Fixes" || !strings.Contains(fix.Description, fixNote) || !strings.Contains(fix.Description, "acme-3") {
		t.Fatalf("fix Ticket %s on %s %q with description %q, want Ready on the CI Fixes Spec acme-4 carrying the note and naming acme-3", fix.State, fix.Spec.ID, fix.Spec.Title, fix.Description)
	}
	if !strings.Contains(fix.Title, "acme-3") {
		t.Fatalf("fix Ticket titled %q, want it naming acme-3", fix.Title)
	}

	tk := ticket3(t, srv)
	if tk.State != store.Accepted || tk.LandAnyway || tk.CIFailureOptions != nil {
		t.Fatalf("%s, land anyway %v, options %v: want it Accepted, not landing anyway, with no options while the fix Ticket is open", tk.State, tk.LandAnyway, tk.CIFailureOptions)
	}
	if c := tk.Comments[len(tk.Comments)-1]; !strings.Contains(c.Body, "acme-5") {
		t.Fatalf("comment %q, want it naming the fix Ticket", c.Body)
	}
	if row, ok := needsMeRowOf(t, srv, "acme-3"); ok {
		t.Fatalf("row %+v, want acme-3 out of Needs Me once a fix Ticket is opened", row)
	}
}

func TestNewFixTicketIsOfferedUnderBranchToo(t *testing.T) {
	srv := newServer(t, failedCI)
	httpOK(t, srv, setLandingMethod(`{"method":"branch","push":true,"remote":"origin"}`))
	httpOK(t, srv, newFixTicket("3", fixNote))
	if tk := ticket3(t, srv); tk.NeedsMeReason == core.ReasonCIFailed {
		t.Fatal("still CI failed after New fix Ticket under Branch")
	}
}

func TestNewFixTicketNeedsANoteAndAFailedCIResult(t *testing.T) {
	srv := newServer(t, failedCI)
	httpRejected(t, srv, newFixTicket("3", " "), core.Invalid)
	for name, setup := range map[string]func(*testing.T, *codecentraltest.Server){
		"running": pushedAccepted,
		"chosen already": func(t *testing.T, srv *codecentraltest.Server) {
			failedCI(t, srv)
			httpOK(t, srv, landAnyway("3"))
		},
	} {
		t.Run(name, func(t *testing.T) {
			srv := newServer(t, setup)
			httpRejected(t, srv, newFixTicket("3", fixNote), core.ActionNotAllowed)
			if _, err := srv.Client.GetTicket(context.Background(), "acme", "acme-4"); err == nil {
				t.Fatal("a fix Ticket was opened on a refused call")
			}
		})
	}
}

func TestNewFixTicketIsRejectedForAnAgent(t *testing.T) {
	srv := newServer(t, failedCI)
	status, b := httpDoAs(t, srv, core.Agent, newFixTicket("3", fixNote))
	errorJSON(t, b, core.ActionNotAllowed)
	if status == 200 || ticket3(t, srv).LandAnyway {
		t.Fatal("an agent's New fix Ticket was recorded")
	}
}

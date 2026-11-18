package core

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/tomoram/code-central/store"
)

// Failure Reports, giving up, and the human's way out of Failed (ticket 30).

// ReportFailureInput is report_failure's input. Every field is required.
type ReportFailureInput struct {
	Ticket          Ref    `json:"ticket" path:"n" desc:"The Ticket." required:"true"`
	ClaimID         string `json:"claim_id" desc:"The Claim ID claim returned." required:"true"`
	WhatBroke       string `json:"what_broke" desc:"A headline plus the symptom, in Markdown." required:"true"`
	Why             string `json:"why" desc:"Why it broke, in Markdown." required:"true"`
	WhatToFix       string `json:"what_to_fix" desc:"What has to change before this Ticket can go on, in Markdown." required:"true"`
	WhereWorkStands string `json:"where_work_stands" desc:"Where the work stands: branch, commits, what is done, in Markdown." required:"true"`
	// Full asks for the Ticket in place of the Acknowledgement.
	Full bool `json:"full,omitempty" desc:"Return the whole Ticket instead of the acknowledgement."`
}

// GiveUpInput is give_up's input; the note is required.
type GiveUpInput struct {
	Ticket  Ref    `json:"ticket" path:"n" desc:"The Ticket." required:"true"`
	ClaimID string `json:"claim_id" desc:"The Claim ID claim returned." required:"true"`
	Note    string `json:"note" desc:"How far you got and why you are stopping, in Markdown." required:"true"`
	// Full asks for the Ticket in place of the Acknowledgement.
	Full bool `json:"full,omitempty" desc:"Return the whole Ticket instead of the acknowledgement."`
}

// SendBackInput is send_back's input. From Failed (the human's) the note
// is optional, and a send-back with no note means nothing has changed.
// From Accepted (the agent's on a conflict, ticket 72, or the human's to
// fix a CI failure, code-central-264) the note is required.
type SendBackInput struct {
	Ticket Ref    `json:"ticket" path:"n" desc:"The Ticket." required:"true"`
	Note   string `json:"note,omitempty" desc:"Why it can't land: the base commit and the conflicting files, in Markdown." required:"true"`
	// Full asks for the Ticket in place of the Acknowledgement.
	Full bool `json:"full,omitempty" desc:"Return the whole Ticket instead of the acknowledgement."`
}

// StopResult is the result of an action that ends the agent's work: the
// Acknowledgement, or with full the Ticket, and the next step, "stop
// working this Ticket".
type StopResult = FinishResult

// FailureReportView is one Failure Report as reads show it.
type FailureReportView struct {
	Number int `json:"number"`
	Claim  int `json:"claim"`
	// SessionID, WorkingDir and ResumeCommand are copies of the Claim's,
	// made when the report was written.
	SessionID       string    `json:"session_id,omitempty"`
	WorkingDir      string    `json:"working_dir,omitempty"`
	ResumeCommand   string    `json:"resume_command,omitempty"`
	Actor           string    `json:"actor"`
	Time            time.Time `json:"time"`
	WhatBroke       string    `json:"what_broke"`
	Why             string    `json:"why"`
	WhatToFix       string    `json:"what_to_fix"`
	WhereWorkStands string    `json:"where_work_stands"`
}

// ReportFailure is report_failure(ticket, claim_id, what_broke, why,
// what_to_fix, where_work_stands): the holder posts a numbered, permanent
// Failure Report, which moves In Progress to Failed and ends the Claim.
// It and the Factory's fail_finish are the only ways into Failed.
func (c *Core) ReportFailure(ctx context.Context, key string, in ReportFailureInput) (StopResult, error) {
	parts := store.FailureReport{WhatBroke: in.WhatBroke, Why: in.Why, WhatToFix: in.WhatToFix, WhereWorkStands: in.WhereWorkStands}
	if err := checkFailureReport(parts); err != nil {
		return StopResult{}, err
	}
	v, err := c.updateTicket(key, "report_failure", in.Ticket, in.Full, func(t *store.Ticket) (bool, error) {
		now := c.now()
		live, _ := liveClaim(*t)
		if err := transition(ctx, t, key, "report_failure", in.ClaimID, now); err != nil {
			return false, err
		}
		// transition checked live is the holder's Claim.
		addFailureReport(t, *live, ActorOf(ctx), now, parts)
		return true, nil
	})
	if err != nil {
		return StopResult{}, err
	}
	return stopResult(v, "report_failure"), nil
}

// FailFinishInput is the Factory's fail_finish(ticket, what_broke, why,
// what_to_fix, where_work_stands, full?): a Failure Report on a finish
// the finish check found wrong. Every field is required.
type FailFinishInput struct {
	Ticket          Ref    `json:"ticket" path:"n"`
	WhatBroke       string `json:"what_broke"`
	Why             string `json:"why"`
	WhatToFix       string `json:"what_to_fix"`
	WhereWorkStands string `json:"where_work_stands"`
	// Full asks for the Ticket in place of the Acknowledgement.
	Full bool `json:"full,omitempty"`
}

// failFinish is the Factory's workflow action that fails a finish.
const failFinish = "fail_finish"

// FailFinish is the Factory's fail_finish (Spec code-central-848): its
// finish check found a finish that leaves out a Repository with work or
// names a commit that isn't its branch's head. It adds a Failure Report on
// the finishing Claim, which has already ended, and moves the Ticket from
// In Review, or from Accepted while it has landed nowhere, to Failed.
func (c *Core) FailFinish(ctx context.Context, key string, in FailFinishInput) (Written[TicketView], error) {
	parts := store.FailureReport{WhatBroke: in.WhatBroke, Why: in.Why, WhatToFix: in.WhatToFix, WhereWorkStands: in.WhereWorkStands}
	if err := checkFailureReport(parts); err != nil {
		return Written[TicketView]{}, err
	}
	return c.updateTicket(key, failFinish, in.Ticket, in.Full, func(t *store.Ticket) (bool, error) {
		if t.State == store.Accepted && landedSomewhere(key, *t) {
			return false, &Error{Code: ActionNotAllowed, Message: fmt.Sprintf("%s has landed in some of its Changed Repositories, so its finish can't fail: land the rest, or send it back", DisplayID(key, t.Number))}
		}
		// The Claim the finish ended; none for a Ticket moved there by hand.
		var finishing store.Claim
		for _, cl := range t.Claims {
			if cl.EndedBy == "finish" {
				finishing = cl
			}
		}
		now := c.now()
		if err := transition(ctx, t, key, failFinish, "", now); err != nil {
			return false, err
		}
		addFailureReport(t, finishing, ActorOf(ctx), now, parts)
		return true, nil
	})
}

// checkFailureReport refuses parts, a Failure Report's, missing any of
// its four.
func checkFailureReport(parts store.FailureReport) error {
	for _, f := range []struct{ name, value, hint string }{
		{"what_broke", parts.WhatBroke, "a headline plus the symptom"},
		{"why", parts.Why, "why it broke"},
		{"what_to_fix", parts.WhatToFix, "what has to change before this Ticket can go on"},
		{"where_work_stands", parts.WhereWorkStands, "where the work stands: branch, commits, what is done"},
	} {
		if strings.TrimSpace(f.value) == "" {
			return invalid(f.name, "a Failure Report needs %s: %s", f.name, f.hint)
		}
	}
	return nil
}

// addFailureReport adds parts, a Failure Report's four, to t as its next
// Failure Report, made by actor at now on Claim cl, with copies of its
// session, working directory and Resume Command.
func addFailureReport(t *store.Ticket, cl store.Claim, actor Actor, now time.Time, parts store.FailureReport) {
	parts.Number, parts.Claim = len(t.FailureReports)+1, cl.Number
	parts.SessionID, parts.WorkingDir, parts.ResumeCommand = cl.SessionID, cl.WorkingDir, cl.ResumeCommand
	parts.Actor, parts.Time = string(actor), now
	t.FailureReports = append(t.FailureReports, parts)
}

// GiveUp is give_up(ticket, claim_id, note): the holder, finding nothing
// wrong but unable to finish, moves In Progress back to Ready with the
// note stored as a comment.
func (c *Core) GiveUp(ctx context.Context, key string, in GiveUpInput) (StopResult, error) {
	if strings.TrimSpace(in.Note) == "" {
		return StopResult{}, invalid("note", "give_up needs a note saying how far you got and why you are stopping")
	}
	v, err := c.act(ctx, key, in.Ticket, in.Full, "give_up", in.ClaimID, in.Note)
	if err != nil {
		return StopResult{}, err
	}
	return stopResult(v, "give_up"), nil
}

// SendBack is send_back(ticket, note): the human's from Failed to Ready,
// with an optional note, or from Accepted to Ready, with the note
// required: the agent's for work that can't land, such as a merge
// conflict, and the human's for work to fix, such as a CI failure.
// The note is stored as a comment. The Ticket keeps its created time, so
// it keeps its place near the front of the claim order. While none of
// its pull requests in several Repositories has merged, its open ones
// are marked for the Pull Request Watcher to close; once one has, the
// Repositories it merged in keep their landed commits. A Spec-landed
// Ticket's pull requests are its Spec's, so they are left open.
func (c *Core) SendBack(ctx context.Context, key string, in SendBackInput) (Written[TicketView], error) {
	if ActorOf(ctx) == Human {
		if err := c.refuseSendBackOfShared(key, in.Ticket); err != nil {
			return Written[TicketView]{}, err
		}
	}
	shared := false
	_ = c.store.View(func(r store.Reader) error {
		if t, err := resolveTicket(r, key, "send_back", "ticket", in.Ticket); err == nil {
			p, _ := r.Project(key)
			shared = specLanded(r, p, t)
		}
		return nil // the send-back itself reports what is wrong
	})
	return c.actThen(ctx, key, in.Ticket, in.Full, "send_back", "", in.Note, func(t *store.Ticket) error {
		if from := t.Moves[len(t.Moves)-1].From; from == store.Accepted && strings.TrimSpace(in.Note) == "" {
			return invalid("note", "sending back Accepted work needs a note saying why: the base commit and the conflicting files of a merge conflict, or the failed CI run's link and its failed jobs")
		}
		if !shared {
			markPullRequestsToClose(t)
		}
		return nil
	})
}

// refuseSendBackOfShared rejects the human's send_back of an Accepted
// pushed Ticket that is Spec-landed (code-central-379), whose work is in
// the base with the Spec's in a Changed Repository where Land per Spec is
// in force, so there is nothing of its own to send back to fix, or whose
// CI failed or CI cancelled item lists other Tickets too
// (code-central-432), which the commit's fix must cover as well.
func (c *Core) refuseSendBackOfShared(key string, ref Ref) error {
	return c.store.View(func(r store.Reader) error {
		t, err := resolveTicket(r, key, "send_back", "ticket", ref)
		if err != nil || t.State != store.Accepted || len(t.PushedCommits) == 0 {
			return nil // the send-back itself reports what is wrong
		}
		p, _ := r.Project(key)
		id := DisplayID(key, t.Number)
		if specLanded(r, p, t) && ticketLandingPolicies(p, t).LandPerSpecInForce() {
			return &Error{Code: ActionNotAllowed, Message: fmt.Sprintf("%s's work is in the base with its Spec's, so there is no branch of its own to send back: land it anyway or open a fix Ticket", id)}
		}
		if it, ok := c.evaluate(r, p).itemOf(t); ok && it.reason != ReasonCIUnavailable && len(it.tickets) > 1 {
			var others []string
			for _, o := range it.tickets {
				if o.Number != t.Number {
					others = append(others, DisplayID(key, o.Number))
				}
			}
			return &Error{Code: ActionNotAllowed, Message: fmt.Sprintf("%s waits on %s with %s, so there is no fix of its own to send back: re-run the failed jobs, land them anyway or open a fix Ticket", id, shortCommit(it.commit), strings.Join(others, ", "))}
		}
		return nil
	})
}

// ToDraft is the human's to_draft(ticket): Failed to Draft.
func (c *Core) ToDraft(ctx context.Context, key string, in TicketInput) (Written[TicketView], error) {
	return c.act(ctx, key, in.Ticket, in.Full, "to_draft", "", "")
}

func failureReportViews(rs []store.FailureReport) []FailureReportView {
	out := []FailureReportView{}
	for _, r := range rs {
		out = append(out, FailureReportView{
			Number: r.Number, Claim: r.Claim, SessionID: r.SessionID, WorkingDir: r.WorkingDir, ResumeCommand: r.ResumeCommand,
			Actor: r.Actor, Time: r.Time,
			WhatBroke: r.WhatBroke, Why: r.Why, WhatToFix: r.WhatToFix, WhereWorkStands: r.WhereWorkStands,
		})
	}
	return out
}

// lastActivity is the time of the latest stored write a Claim's agent made
// on the Ticket: the claim itself, its moves and Failure Reports, and any
// non-human comment or edit made while the Claim was live. Writes by the
// human (a comment, a release) are not the agent's activity. Pings are
// memory-only and are added on top by the Silent Claim check.
func lastActivity(t store.Ticket, cl store.Claim) time.Time {
	last := cl.Started
	during := func(actor string, at time.Time) bool {
		return actor != string(Human) && !at.Before(cl.Started) && (cl.Live() || !at.After(cl.Ended))
	}
	see := func(at time.Time) {
		if at.After(last) {
			last = at
		}
	}
	for _, m := range t.Moves {
		if m.Claim == cl.Number && m.Actor != string(Human) {
			see(m.Time)
		}
	}
	for _, r := range t.Rounds {
		if r.Claim == cl.Number {
			see(r.Posted)
		}
	}
	for _, r := range t.FailureReports {
		if r.Claim == cl.Number {
			see(r.Time)
		}
	}
	for _, cm := range t.Comments {
		if during(cm.Actor, cm.Time) {
			see(cm.Time)
		}
	}
	for _, e := range t.Edits {
		if during(e.Actor, e.Time) {
			see(e.Time)
		}
	}
	return last
}

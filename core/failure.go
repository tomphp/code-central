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
	Ticket          Ref    `json:"ticket"`
	ClaimID         string `json:"claim_id"`
	WhatBroke       string `json:"what_broke"`
	Why             string `json:"why"`
	WhatToFix       string `json:"what_to_fix"`
	WhereWorkStands string `json:"where_work_stands"`
	// Full asks for the Ticket in place of the Acknowledgement.
	Full bool `json:"full,omitempty"`
}

// GiveUpInput is give_up's input; the note is required.
type GiveUpInput struct {
	Ticket  Ref    `json:"ticket"`
	ClaimID string `json:"claim_id"`
	Note    string `json:"note"`
	// Full asks for the Ticket in place of the Acknowledgement.
	Full bool `json:"full,omitempty"`
}

// SendBackInput is send_back's input. From Failed (the human's) the note
// is optional, and a send-back with no note means nothing has changed.
// From Accepted (the agent's on a conflict, ticket 72, or the human's to
// fix a CI failure, code-central-264) the note is required.
type SendBackInput struct {
	Ticket Ref    `json:"ticket"`
	Note   string `json:"note,omitempty"`
	// Full asks for the Ticket in place of the Acknowledgement.
	Full bool `json:"full,omitempty"`
}

// StopResult is the result of an action that ends the agent's work: the
// Acknowledgement, or with full the Ticket, and the next step, "stop
// working this Ticket".
type StopResult = FinishResult

// FailureReportView is one Failure Report as reads show it.
type FailureReportView struct {
	Number          int       `json:"number"`
	Claim           int       `json:"claim"`
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
// It is the only way into Failed.
func (c *Core) ReportFailure(ctx context.Context, key string, in ReportFailureInput) (StopResult, error) {
	for _, f := range []struct{ name, value, hint string }{
		{"what_broke", in.WhatBroke, "a headline plus the symptom"},
		{"why", in.Why, "why it broke"},
		{"what_to_fix", in.WhatToFix, "what has to change before this Ticket can go on"},
		{"where_work_stands", in.WhereWorkStands, "where the work stands: branch, commits, what is done"},
	} {
		if strings.TrimSpace(f.value) == "" {
			return StopResult{}, invalid(f.name, "a Failure Report needs %s: %s", f.name, f.hint)
		}
	}
	v, err := c.updateTicket(key, "report_failure", in.Ticket, in.Full, func(t *store.Ticket) (bool, error) {
		now := c.now()
		live, _ := liveClaim(*t)
		if err := transition(ctx, t, key, "report_failure", in.ClaimID, now); err != nil {
			return false, err
		}
		// transition checked live is the holder's Claim.
		t.FailureReports = append(t.FailureReports, store.FailureReport{
			Number: len(t.FailureReports) + 1, Claim: live.Number,
			SessionID: live.SessionID, WorkingDir: live.WorkingDir, ResumeCommand: live.ResumeCommand,
			Actor: string(ActorOf(ctx)), Time: now,
			WhatBroke: in.WhatBroke, Why: in.Why, WhatToFix: in.WhatToFix, WhereWorkStands: in.WhereWorkStands,
		})
		return true, nil
	})
	if err != nil {
		return StopResult{}, err
	}
	return stopResult(v, "report_failure"), nil
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
// it keeps its place near the front of the claim order.
func (c *Core) SendBack(ctx context.Context, key string, in SendBackInput) (Written[TicketView], error) {
	if ActorOf(ctx) == Human {
		if err := c.refuseSendBackOfShared(key, in.Ticket); err != nil {
			return Written[TicketView]{}, err
		}
	}
	return c.actThen(ctx, key, in.Ticket, in.Full, "send_back", "", in.Note, func(t *store.Ticket) error {
		if from := t.Moves[len(t.Moves)-1].From; from == store.Accepted && strings.TrimSpace(in.Note) == "" {
			return invalid("note", "sending back Accepted work needs a note saying why: the base commit and the conflicting files of a merge conflict, or the failed CI run's link and its failed jobs")
		}
		return nil
	})
}

// refuseSendBackOfShared rejects the human's send_back of an Accepted
// pushed Ticket that is Spec-landed (code-central-379), whose work is in
// the base with the Spec's, so there is nothing of its own to send back
// to fix, or whose CI failed or CI cancelled item lists other Tickets
// too (code-central-432), which the commit's fix must cover as well.
func (c *Core) refuseSendBackOfShared(key string, ref Ref) error {
	return c.store.View(func(r store.Reader) error {
		t, err := resolveTicket(r, key, "send_back", "ticket", ref)
		if err != nil || t.State != store.Accepted || len(t.PushedCommits) == 0 {
			return nil // the send-back itself reports what is wrong
		}
		p, _ := r.Project(key)
		id := DisplayID(key, t.Number)
		if specLanded(r, p.Landing, key, t) {
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

package core

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/tomoram/code-central/store"
)

// CI failed (code-central-261): an Accepted Ticket whose CI result is
// failed waits on the human in Needs Me until they choose one of the
// failure options. Land anyway, the human's land_anyway, accepts the
// failed CI result, and the Factory lands the Ticket on its next poll.

// The failure options, each named for its operation.
const (
	// OptionLandAnyway is land_anyway, offered only under Merge: under
	// Branch the human's merge decides.
	OptionLandAnyway = "land_anyway"
	// OptionSendBack is the human's send_back from Accepted: the next
	// Claim fixes the work on its branch (code-central-264).
	OptionSendBack = "send_back"
	// OptionNewFixTicket is new_fix_ticket: a Ready Ticket on the same
	// Spec fixes it forward, and the failed Ticket lands
	// (code-central-264).
	OptionNewFixTicket = "new_fix_ticket"
)

// CIFailureSummary is what failed on a CI failed Ticket's pushed commit:
// the commit and its failed runs' workflow file names.
type CIFailureSummary struct {
	Commit    string   `json:"commit"`
	Workflows []string `json:"workflows"`
}

// ciFailure is t's failure while it is CI failed: an Accepted Ticket
// whose latest pushed commit's CI result is failed and that the human
// hasn't dealt with, from its runs kept at now. since is when its first
// failed run finished.
func ciFailure(r store.Reader, key string, t store.Ticket, now time.Time) (s CIFailureSummary, since time.Time, ok bool) {
	if t.State != store.Accepted || len(t.PushedCommits) == 0 {
		return CIFailureSummary{}, time.Time{}, false
	}
	latest := latestPushed(t)
	if !latest.LandAnyway.IsZero() {
		return CIFailureSummary{}, time.Time{}, false
	}
	mine, _ := ticketRuns(r, key, t, now)
	if latestCIResult(latest, mine, now) != CIFailed {
		return CIFailureSummary{}, time.Time{}, false
	}
	s = CIFailureSummary{Commit: latest.Commit, Workflows: []string{}}
	for _, run := range countedRuns(latest.Commit, mine) {
		if run.Status != store.CICompleted || !run.Conclusion.Failed() {
			continue
		}
		s.Workflows = append(s.Workflows, run.Workflow)
		if at := run.Finished; !at.IsZero() && (since.IsZero() || at.Before(since)) {
			since = at
		}
	}
	slices.Sort(s.Workflows)
	s.Workflows = slices.Compact(s.Workflows)
	if since.IsZero() {
		since = latest.Pushed
	}
	return s, since, true
}

// ciFailureOptions are the failure options the human may choose for t
// while it is CI failed, nil otherwise: Land anyway under Merge only,
// since under Branch the human's merge decides; Send back to fix and New
// fix Ticket under both.
func ciFailureOptions(r store.Reader, key string, t store.Ticket, now time.Time) []string {
	if _, _, ok := ciFailure(r, key, t, now); !ok {
		return nil
	}
	fixes := []string{OptionSendBack, OptionNewFixTicket}
	p, _ := r.Project(key)
	if p.Landing.Method == store.LandingBranch {
		return fixes
	}
	return append([]string{OptionLandAnyway}, fixes...)
}

// latestPushed is t's latest pushed commit, the one its CI result comes
// from; t must have one.
func latestPushed(t store.Ticket) store.PushedCommit {
	return t.PushedCommits[len(t.PushedCommits)-1]
}

// choseLandAnyway reports whether the human chose Land anyway for t's
// latest pushed commit.
func choseLandAnyway(t store.Ticket) bool {
	return len(t.PushedCommits) > 0 && !latestPushed(t).LandAnyway.IsZero()
}

// LandAnyway is the human's land_anyway(ticket): it accepts the failed CI
// result of an Accepted Ticket's latest pushed commit, recorded on that
// commit, so the Factory lands the Ticket on its next poll. A newer
// pushed commit has its own CI result, which it doesn't cover. Choosing
// it again changes nothing.
func (c *Core) LandAnyway(ctx context.Context, key string, in TicketInput) (Written[TicketView], error) {
	if err := humanOnly(ctx, "land_anyway"); err != nil {
		return Written[TicketView]{}, err
	}
	return c.updateTicketIn(key, "land_anyway", in.Ticket, in.Full, func(r store.Reader, t *store.Ticket) (bool, error) {
		if choseLandAnyway(*t) && t.State == store.Accepted {
			return false, nil
		}
		if !slices.Contains(ciFailureOptions(r, key, *t, c.now()), OptionLandAnyway) {
			return false, &Error{Code: ActionNotAllowed, Message: whyNotLandAnyway(r, key, *t, c.now())}
		}
		t.PushedCommits[len(t.PushedCommits)-1].LandAnyway = c.now()
		return true, nil
	})
}

// whyNotLandAnyway says why t isn't offered Land anyway.
func whyNotLandAnyway(r store.Reader, key string, t store.Ticket, now time.Time) string {
	id := DisplayID(key, t.Number)
	if t.State != store.Accepted {
		return fmt.Sprintf("%s is %s: only an Accepted Ticket whose CI failed lands anyway", id, t.State)
	}
	if _, _, ok := ciFailure(r, key, t, now); !ok {
		return fmt.Sprintf("%s has no failed CI result to land anyway", id)
	}
	return fmt.Sprintf("%s's Project lands by Branch, so your merge decides whether it lands, whatever its CI result", id)
}

// NewFixTicketInput is new_fix_ticket's input: the CI failed Ticket and
// the note, which is required.
type NewFixTicketInput struct {
	Ticket Ref    `json:"ticket"`
	Note   string `json:"note"`
	// Full asks for the fix Ticket in place of the Acknowledgement.
	Full bool `json:"full,omitempty"`
}

// NewFixTicket is the human's new_fix_ticket(ticket, note): it opens a
// Ready Ticket on a CI failed Ticket's Spec, its description the note,
// and records on the failed Ticket the same acceptance as Land anyway, so
// it lands, with a comment naming the fix Ticket. Under Merge + Push the
// failed work is already on the remote's base, so the fix Ticket fixes it
// forward. An update writes one record, so the fix Ticket is written
// first and the acceptance second: the failed Ticket never lands without
// its fix Ticket. Its result is the fix Ticket's create Acknowledgement.
func (c *Core) NewFixTicket(ctx context.Context, key string, in NewFixTicketInput) (Written[TicketView], error) {
	if err := humanOnly(ctx, "new_fix_ticket"); err != nil {
		return Written[TicketView]{}, err
	}
	if strings.TrimSpace(in.Note) == "" {
		return Written[TicketView]{}, invalid("note", "a fix Ticket needs a note saying what failed, such as the failed CI run's link and its failed jobs")
	}
	offered := func(r store.Reader, t store.Ticket) error {
		if !slices.Contains(ciFailureOptions(r, key, t, c.now()), OptionNewFixTicket) {
			return &Error{Code: ActionNotAllowed, Message: whyNotFix(key, t)}
		}
		return nil
	}
	var v Written[TicketView]
	var failed, fix string
	err := c.store.Update(func(w store.Writer) error {
		if err := writable(w, key); err != nil {
			return err
		}
		t, err := resolveTicket(w, key, "new_fix_ticket", "ticket", in.Ticket)
		if err != nil {
			return err
		}
		if err := offered(w, t); err != nil {
			return err
		}
		now := c.now()
		failed = DisplayID(key, t.Number)
		f := store.Ticket{
			Number: w.NextNumber(key), Title: fmt.Sprintf("Fix the CI failure of %s: %s", failed, t.Title), State: store.Ready, Spec: t.Spec,
			Created: now, Updated: now,
			Description: fmt.Sprintf("%s's CI failed, and it lands anyway: fix the failure forward, on top of its work in the base.\n\n%s", failed, in.Note),
		}
		if err := w.PutTicket(key, f); err != nil {
			return err
		}
		fix = DisplayID(key, f.Number)
		v, err = acknowledgeCreate(c.ticketView(w, key, store.Ticket{}), c.ticketView(w, key, f), in.Full)
		return err
	})
	if err != nil {
		return Written[TicketView]{}, err
	}
	_, err = c.updateTicketIn(key, "new_fix_ticket", in.Ticket, true, func(r store.Reader, t *store.Ticket) (bool, error) {
		if err := offered(r, *t); err != nil {
			return false, err
		}
		now := c.now()
		t.PushedCommits[len(t.PushedCommits)-1].LandAnyway = now
		addComment(t, ActorOf(ctx), now, 0, fmt.Sprintf("New fix Ticket: %s fixes this CI failure forward, and %s lands anyway.", fix, failed))
		return true, nil
	})
	if err != nil {
		return Written[TicketView]{}, err
	}
	return v, nil
}

// whyNotFix says why t isn't offered New fix Ticket.
func whyNotFix(key string, t store.Ticket) string {
	id := DisplayID(key, t.Number)
	if t.State != store.Accepted {
		return fmt.Sprintf("%s is %s: only an Accepted Ticket whose CI failed gets a fix Ticket", id, t.State)
	}
	return fmt.Sprintf("%s has no failed CI result waiting on you to fix", id)
}

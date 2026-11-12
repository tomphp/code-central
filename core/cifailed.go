package core

import (
	"cmp"
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
// Re-run failed jobs, the human's rerun_ci (code-central-265), asks the
// CI Watcher to re-run the failed runs, and the Ticket waits on CI again.

// The failure options, each named for its operation.
const (
	// OptionRerunCI is rerun_ci, offered under both Push Landing Methods.
	OptionRerunCI = "rerun_ci"
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
	latest, mine, ok := unhandledCI(r, key, t, CIFailed, now)
	if !ok {
		return CIFailureSummary{}, time.Time{}, false
	}
	s = CIFailureSummary{Commit: latest.Commit, Workflows: []string{}}
	for _, run := range failedRuns(latest.Commit, mine) {
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

// unhandledCI is t's latest pushed commit and its runs when t is
// Accepted, the human hasn't chosen Land anyway for the commit and t's
// CI result is result at now.
func unhandledCI(r store.Reader, key string, t store.Ticket, result CIResult, now time.Time) (store.PushedCommit, []store.CIRun, bool) {
	if t.State != store.Accepted || len(t.PushedCommits) == 0 || !isCILead(r, key, t) {
		return store.PushedCommit{}, nil, false
	}
	latest := latestPushed(t)
	if !latest.LandAnyway.IsZero() {
		return store.PushedCommit{}, nil, false
	}
	mine, _ := ticketRuns(r, key, t, now)
	if _, got := ticketResult(r, key, t, mine, now); got != result {
		return store.PushedCommit{}, nil, false
	}
	return latest, mine, true
}

// ciCancelled is when t's CI became cancelled while it is CI cancelled:
// an Accepted Ticket whose latest pushed commit's CI result is cancelled
// and that the human hasn't dealt with, since its first cancelled run
// finished.
func ciCancelled(r store.Reader, key string, t store.Ticket, now time.Time) (since time.Time, ok bool) {
	latest, mine, ok := unhandledCI(r, key, t, CICancelled, now)
	if !ok {
		return time.Time{}, false
	}
	for _, run := range rerunnableRuns(latest.Commit, mine) {
		if at := run.Finished; !at.IsZero() && (since.IsZero() || at.Before(since)) {
			since = at
		}
	}
	if since.IsZero() {
		since = latest.Pushed
	}
	return since, true
}

// ciFailureOptions are the failure options the human may choose for t
// while it is CI failed or CI cancelled, nil otherwise: Re-run under
// both Landing Methods' pushes, Land anyway under Merge only, since
// under Branch and Pull Request the human's merge decides, Send back to fix, and for a
// failure New fix Ticket. A cancelled run has nothing to fix forward.
func ciFailureOptions(r store.Reader, key string, t store.Ticket, now time.Time) []string {
	fixes := []string{OptionSendBack, OptionNewFixTicket}
	if _, _, ok := ciFailure(r, key, t, now); !ok {
		if _, ok := ciCancelled(r, key, t, now); !ok {
			return nil
		}
		fixes = []string{OptionSendBack}
	}
	// A Spec-landed Ticket's work is in the base with its Spec's, so
	// there is no branch of its own to send back for a fix.
	if p, _ := r.Project(key); specLanded(r, p.Landing, key, t) {
		fixes = slices.DeleteFunc(fixes, func(o string) bool { return o == OptionSendBack })
	}
	p, _ := r.Project(key)
	if p.Landing.Method != store.LandingMerge {
		return append([]string{OptionRerunCI}, fixes...)
	}
	return append([]string{OptionRerunCI, OptionLandAnyway}, fixes...)
}

// latestPushed is t's latest pushed commit, the one its CI result comes
// from; t must have one.
func latestPushed(t store.Ticket) store.PushedCommit {
	return t.PushedCommits[len(t.PushedCommits)-1]
}

// acceptFailedCI records the human's acceptance of t's latest pushed
// commit's failed CI result at now, as Land anyway and New fix Ticket
// both do; t must have a pushed commit.
func acceptFailedCI(t *store.Ticket, now time.Time) {
	t.PushedCommits[len(t.PushedCommits)-1].LandAnyway = now
}

// writeGroupOthers applies apply to each Ticket waiting with the named
// one on its pushed commit, the named Ticket aside, when allowed accepts
// the named Ticket's choice of option. An update writes one record, so
// each is its own write; the caller then writes the named Ticket last.
// Ticket not Accepted or in no group has no others, and allowed alone
// speaks for it.
func (c *Core) writeGroupOthers(key string, ref Ref, option string, allowed func(store.Reader, store.Ticket) error, apply func(store.Reader, *store.Ticket)) error {
	var others []int
	err := c.store.View(func(r store.Reader) error {
		if err := writable(r, key); err != nil {
			return err
		}
		t, err := resolveTicket(r, key, option, "ticket", ref)
		if err != nil {
			return err
		}
		if choseLandAnyway(t) && t.State == store.Accepted && option == OptionLandAnyway {
			return nil
		}
		if rerunning(r, key, t, c.now()) && option == OptionRerunCI {
			return nil
		}
		if err := allowed(r, t); err != nil {
			return err
		}
		for _, o := range ciGroup(r, key, t) {
			if o.Number != t.Number {
				others = append(others, o.Number)
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	for _, n := range others {
		err := c.store.Update(func(w store.Writer) error {
			o, ok := w.Ticket(key, n)
			if !ok {
				return nil
			}
			apply(w, &o)
			o.Updated = c.now()
			return w.PutTicket(key, o)
		})
		if err != nil {
			return err
		}
	}
	return nil
}

// rerunning is whether a Re-run request for t waits on the CI Watcher.
func rerunning(r store.Reader, key string, t store.Ticket, now time.Time) bool {
	mine, _ := ticketRuns(r, key, t, now)
	return t.State == store.Accepted && rerunPending(t, mine, now)
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
	// A Spec's group lands together: the others first, so a failed write
	// leaves the lead, which carries the options, still asking.
	err := c.writeGroupOthers(key, in.Ticket, OptionLandAnyway, func(r store.Reader, t store.Ticket) error {
		if !slices.Contains(ciFailureOptions(r, key, t, c.now()), OptionLandAnyway) {
			return &Error{Code: ActionNotAllowed, Message: whyNotLandAnyway(r, key, t, c.now())}
		}
		return nil
	}, func(_ store.Reader, o *store.Ticket) { acceptFailedCI(o, c.now()) })
	if err != nil {
		return Written[TicketView]{}, err
	}
	return c.updateTicketIn(key, "land_anyway", in.Ticket, in.Full, func(r store.Reader, t *store.Ticket) (bool, error) {
		if choseLandAnyway(*t) && t.State == store.Accepted {
			return false, nil
		}
		if !slices.Contains(ciFailureOptions(r, key, *t, c.now()), OptionLandAnyway) {
			return false, &Error{Code: ActionNotAllowed, Message: whyNotLandAnyway(r, key, *t, c.now())}
		}
		acceptFailedCI(t, c.now())
		return true, nil
	})
}

// whyNotLandAnyway says why t isn't offered Land anyway.
func whyNotLandAnyway(r store.Reader, key string, t store.Ticket, now time.Time) string {
	id := DisplayID(key, t.Number)
	if t.State != store.Accepted {
		return fmt.Sprintf("%s is %s: only an Accepted Ticket whose CI failed lands anyway", id, t.State)
	}
	if g := ciGroup(r, key, t); len(g) > 0 && g[0].Number != t.Number {
		return fmt.Sprintf("%s waits with %s on one pushed commit: choose on %s, which acts on all of them", id, DisplayID(key, t.Spec), DisplayID(key, g[0].Number))
	}
	if _, _, ok := ciFailure(r, key, t, now); !ok {
		return fmt.Sprintf("%s has no failed or cancelled CI result to land anyway", id)
	}
	if p, _ := r.Project(key); p.Landing.Method == store.LandingPullRequest {
		return fmt.Sprintf("%s's Project lands by Pull Request, so your merge on the Code Host decides whether it lands, whatever its CI result", id)
	}
	return fmt.Sprintf("%s's Project lands by Branch, so your merge decides whether it lands, whatever its CI result", id)
}

// RerunCI is the human's rerun_ci(ticket): a Re-run request for each run
// that failed on a CI failed Ticket's latest pushed commit, at its latest
// attempt, recorded on that commit in place of any earlier request. The
// Ticket's CI result reads running until the CI Watcher, which re-runs
// the runs' failed jobs, reports each run's new attempt, which answers
// the request, or the request lapses. Asking again while it waits
// changes nothing.
func (c *Core) RerunCI(ctx context.Context, key string, in TicketInput) (Written[TicketView], error) {
	if err := humanOnly(ctx, "rerun_ci"); err != nil {
		return Written[TicketView]{}, err
	}
	// The others in the Spec's group share the pushed commit, so each asks
	// for the same re-run, before the named Ticket.
	err := c.writeGroupOthers(key, in.Ticket, OptionRerunCI, func(r store.Reader, t store.Ticket) error {
		if !slices.Contains(ciFailureOptions(r, key, t, c.now()), OptionRerunCI) {
			return &Error{Code: ActionNotAllowed, Message: whyNotRerun(r, key, t, c.now())}
		}
		return nil
	}, func(r store.Reader, o *store.Ticket) {
		runs, _ := ticketRuns(r, key, *o, c.now())
		requestRerun(o, runs, c.now())
	})
	if err != nil {
		return Written[TicketView]{}, err
	}
	return c.updateTicketIn(key, "rerun_ci", in.Ticket, in.Full, func(r store.Reader, t *store.Ticket) (bool, error) {
		mine, _ := ticketRuns(r, key, *t, c.now())
		if rerunPending(*t, mine, c.now()) && t.State == store.Accepted {
			return false, nil
		}
		if !slices.Contains(ciFailureOptions(r, key, *t, c.now()), OptionRerunCI) {
			return false, &Error{Code: ActionNotAllowed, Message: whyNotRerun(r, key, *t, c.now())}
		}
		requestRerun(t, mine, c.now())
		return true, nil
	})
}

// requestRerun records on t's latest pushed commit a Re-run request for
// each of its rerunnable runs, in place of any earlier request.
func requestRerun(t *store.Ticket, runs []store.CIRun, now time.Time) {
	latest := &t.PushedCommits[len(t.PushedCommits)-1]
	latest.Reruns = nil
	for _, run := range rerunnableRuns(latest.Commit, runs) {
		latest.Reruns = append(latest.Reruns, store.CIRunAttempt{ID: run.ID, Attempt: run.Attempt})
	}
	slices.SortFunc(latest.Reruns, func(a, b store.CIRunAttempt) int { return cmp.Compare(a.ID, b.ID) })
	latest.Rerun = now
}

// rerunPending reports whether a Re-run request for t's latest pushed
// commit waits on the CI Watcher, among t's runs at now.
func rerunPending(t store.Ticket, runs []store.CIRun, now time.Time) bool {
	return len(t.PushedCommits) > 0 && len(pendingReruns(latestPushed(t), runs, now)) > 0
}

// whyNotRerun says why t isn't offered Re-run failed jobs.
func whyNotRerun(r store.Reader, key string, t store.Ticket, now time.Time) string {
	id := DisplayID(key, t.Number)
	if t.State != store.Accepted {
		return fmt.Sprintf("%s is %s: only an Accepted Ticket whose CI failed has failed jobs to re-run", id, t.State)
	}
	return fmt.Sprintf("%s has no failed or cancelled CI result to re-run", id)
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
	if v, handled, err := c.newSpecFixTicket(ctx, key, in); handled {
		return v, err
	}
	refuseUnlessOffered := func(r store.Reader, t store.Ticket) error {
		if !slices.Contains(ciFailureOptions(r, key, t, c.now()), OptionNewFixTicket) {
			return &Error{Code: ActionNotAllowed, Message: whyNotNewFixTicket(key, t)}
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
		if err := refuseUnlessOffered(w, t); err != nil {
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
	// The Tickets waiting with it land too, before it.
	err = c.writeGroupOthers(key, in.Ticket, OptionNewFixTicket, func(r store.Reader, t store.Ticket) error {
		return refuseUnlessOffered(r, t)
	}, func(_ store.Reader, o *store.Ticket) { acceptFailedCI(o, c.now()) })
	if err != nil {
		return Written[TicketView]{}, err
	}
	_, err = c.updateTicketIn(key, "new_fix_ticket", in.Ticket, true, func(r store.Reader, t *store.Ticket) (bool, error) {
		if err := refuseUnlessOffered(r, *t); err != nil {
			return false, err
		}
		now := c.now()
		acceptFailedCI(t, now)
		addComment(t, ActorOf(ctx), now, 0, fmt.Sprintf("New fix Ticket: %s fixes this CI failure forward, and %s lands anyway.", fix, failed))
		return true, nil
	})
	if err != nil {
		return Written[TicketView]{}, err
	}
	return v, nil
}

// whyNotNewFixTicket says why t isn't offered New fix Ticket.
func whyNotNewFixTicket(key string, t store.Ticket) string {
	id := DisplayID(key, t.Number)
	if t.State != store.Accepted {
		return fmt.Sprintf("%s is %s: only an Accepted Ticket whose CI failed gets a fix Ticket", id, t.State)
	}
	return fmt.Sprintf("%s has no failed CI result waiting on you to fix", id)
}

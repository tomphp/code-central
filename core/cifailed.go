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
// failed waits on the human in Needs Me, in its commit's CI item, until
// they choose one of the failure options. Land anyway, the human's
// land_anyway, accepts the failed CI result, and the Factory lands every
// Ticket the item lists on its next poll. Re-run failed jobs, the human's
// rerun_ci (code-central-265), asks the CI Watcher to re-run the failed
// runs, and the listed Tickets wait on CI again. Each is named on one
// listed Ticket and recorded once on the Project for the commit
// (code-central-432).

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

// CIFailureSummary is what failed on a CI failed item's pushed commit:
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

// unhandledCI is t's latest pushed commit, with the Project's response
// to it, and its runs when t is Accepted, the human hasn't chosen Land
// anyway for the commit and t's CI result is result at now.
func unhandledCI(r store.Reader, key string, t store.Ticket, result CIResult, now time.Time) (store.PushedCommit, []store.CIRun, bool) {
	if t.State != store.Accepted || len(t.PushedCommits) == 0 {
		return store.PushedCommit{}, nil, false
	}
	p, _ := r.Project(key)
	latest := respondedLatest(p, t)
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

// ciFailureOptions are the failure options the human may choose on t's
// CI item while t is CI failed or CI cancelled, nil otherwise; see
// ciItemOptions.
func (c *Core) ciFailureOptions(r store.Reader, key string, t store.Ticket) []string {
	p, _ := r.Project(key)
	it, ok := c.ciItemOf(r, p, t)
	if !ok || it.reason == ReasonCIUnavailable {
		return nil
	}
	return ciItemOptions(r, p, it)
}

// latestPushed is t's latest pushed commit, the one its CI result comes
// from, as stored; t must have one. respondedLatest adds the Project's
// response to it.
func latestPushed(t store.Ticket) store.PushedCommit {
	return t.PushedCommits[len(t.PushedCommits)-1]
}

// acceptFailedCI records on p the human's acceptance of commit's failed
// CI result at now, as Land anyway and New fix Ticket both do.
func acceptFailedCI(p *store.Project, commit string, now time.Time) {
	respond(p, commit, func(res *store.CIResponse) { res.LandAnyway = now })
}

// respondCI is the human's op, land_anyway or rerun_ci, on the Ticket
// named: when the Ticket's CI item offers op, apply records the response
// on the Project for the Ticket's latest pushed commit, so it acts on
// every Ticket the item lists, each of which gets a change notice. done
// says the response is already recorded, so choosing it again changes
// nothing; whyNot says why op isn't offered.
func (c *Core) respondCI(key, op string, ref Ref, full bool, done func(store.Project, store.Ticket, []store.CIRun) bool,
	whyNot func(store.Reader, store.Ticket) string, apply func(*store.Project, store.PushedCommit, []store.CIRun)) (Written[TicketView], error) {
	var v Written[TicketView]
	var listed []int
	err := c.store.Update(func(w store.Writer) error {
		if err := writable(w, key); err != nil {
			return err
		}
		t, err := resolveTicket(w, key, op, "ticket", ref)
		if err != nil {
			return err
		}
		p, _ := w.Project(key)
		// With full the before view isn't compared, so it isn't built.
		var before TicketView
		if !full {
			before = c.ticketView(w, key, t)
		}
		runs, _ := ticketRuns(w, key, t, c.now())
		if t.State == store.Accepted && len(t.PushedCommits) > 0 && done(p, t, runs) {
			v, err = acknowledge(before, c.ticketView(w, key, t), full)
			return err
		}
		if !slices.Contains(c.ciFailureOptions(w, key, t), op) {
			return &Error{Code: ActionNotAllowed, Message: whyNot(w, t)}
		}
		it, _ := c.ciItemOf(w, p, t)
		for _, o := range it.tickets {
			listed = append(listed, o.Number)
		}
		apply(&p, respondedLatest(p, t), runs)
		if err := w.PutProject(p); err != nil {
			return err
		}
		v, err = acknowledge(before, c.ticketView(w, key, t), full)
		return err
	})
	if err != nil {
		return Written[TicketView]{}, err
	}
	for _, n := range listed {
		c.publishTicketChanged(key, n)
	}
	return v, nil
}

// choseLandAnyway reports whether the human chose Land anyway for t's
// latest pushed commit.
func choseLandAnyway(p store.Project, t store.Ticket) bool {
	return len(t.PushedCommits) > 0 && !respondedLatest(p, t).LandAnyway.IsZero()
}

// LandAnyway is the human's land_anyway(ticket): it accepts the failed CI
// result of an Accepted Ticket's latest pushed commit, recorded once on
// the Project for that commit, so the Factory lands every Ticket its CI
// item lists on its next poll. A newer pushed commit has its own CI
// result, which it doesn't cover. Choosing it again changes nothing.
func (c *Core) LandAnyway(ctx context.Context, key string, in TicketInput) (Written[TicketView], error) {
	if err := humanOnly(ctx, "land_anyway"); err != nil {
		return Written[TicketView]{}, err
	}
	return c.respondCI(key, OptionLandAnyway, in.Ticket, in.Full,
		func(p store.Project, t store.Ticket, _ []store.CIRun) bool { return choseLandAnyway(p, t) },
		func(r store.Reader, t store.Ticket) string { return c.whyNotLandAnyway(r, key, t) },
		func(p *store.Project, latest store.PushedCommit, _ []store.CIRun) {
			acceptFailedCI(p, latest.Commit, c.now())
		})
}

// whyNotLandAnyway says why t isn't offered Land anyway.
func (c *Core) whyNotLandAnyway(r store.Reader, key string, t store.Ticket) string {
	id := DisplayID(key, t.Number)
	if t.State != store.Accepted {
		return fmt.Sprintf("%s is %s: only an Accepted Ticket whose CI failed lands anyway", id, t.State)
	}
	if _, _, ok := ciFailure(r, key, t, c.now()); !ok {
		if _, ok := ciCancelled(r, key, t, c.now()); !ok {
			return fmt.Sprintf("%s has no failed or cancelled CI result to land anyway", id)
		}
	}
	if p, _ := r.Project(key); p.Landing.Method == store.LandingPullRequest {
		return fmt.Sprintf("%s's Project lands by Pull Request, so your merge on the Code Host decides whether it lands, whatever its CI result", id)
	}
	return fmt.Sprintf("%s's Project lands by Branch, so your merge decides whether it lands, whatever its CI result", id)
}

// RerunCI is the human's rerun_ci(ticket): a Re-run request for each run
// that failed on a CI failed Ticket's latest pushed commit, at its latest
// attempt, recorded once on the Project for that commit in place of any
// earlier request, so every Ticket its CI item lists waits on it. Their
// CI result reads running until the CI Watcher, which re-runs the runs'
// failed jobs once, reports each run's new attempt, which answers the
// request, or the request lapses. Asking again while it waits changes
// nothing.
func (c *Core) RerunCI(ctx context.Context, key string, in TicketInput) (Written[TicketView], error) {
	if err := humanOnly(ctx, "rerun_ci"); err != nil {
		return Written[TicketView]{}, err
	}
	return c.respondCI(key, OptionRerunCI, in.Ticket, in.Full,
		func(p store.Project, t store.Ticket, runs []store.CIRun) bool {
			return rerunPending(p, t, runs, c.now())
		},
		func(_ store.Reader, t store.Ticket) string { return whyNotRerun(key, t) },
		func(p *store.Project, latest store.PushedCommit, runs []store.CIRun) {
			requestRerun(p, latest.Commit, runs, c.now())
		})
}

// requestRerun records on p a Re-run request for each of commit's
// rerunnable runs, in place of any earlier request.
func requestRerun(p *store.Project, commit string, runs []store.CIRun, now time.Time) {
	respond(p, commit, func(res *store.CIResponse) {
		res.Reruns = nil
		for _, run := range rerunnableRuns(commit, runs) {
			res.Reruns = append(res.Reruns, store.CIRunAttempt{ID: run.ID, Attempt: run.Attempt})
		}
		slices.SortFunc(res.Reruns, func(a, b store.CIRunAttempt) int { return cmp.Compare(a.ID, b.ID) })
		res.Rerun = now
	})
}

// rerunPending reports whether a Re-run request for t's latest pushed
// commit waits on the CI Watcher, among t's runs at now.
func rerunPending(p store.Project, t store.Ticket, runs []store.CIRun, now time.Time) bool {
	return len(t.PushedCommits) > 0 && len(pendingReruns(respondedLatest(p, t), runs, now)) > 0
}

// whyNotRerun says why t isn't offered Re-run failed jobs.
func whyNotRerun(key string, t store.Ticket) string {
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
// and records on the Project the same acceptance as Land anyway for the
// Ticket's pushed commit, so every Ticket its CI item lists lands, with a
// comment on the named Ticket naming the fix Ticket. Under Merge + Push
// the failed work is already on the remote's base, so the fix Ticket
// fixes it forward. An update writes one record, so the fix Ticket is
// written first and the acceptance second: the failed Ticket never
// lands without its fix Ticket. Its result is the fix Ticket's create
// Acknowledgement.
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
		if !slices.Contains(c.ciFailureOptions(r, key, t), OptionNewFixTicket) {
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
	// The acceptance lands every listed Ticket, before the comment.
	_, err = c.respondCI(key, OptionNewFixTicket, in.Ticket, true,
		func(store.Project, store.Ticket, []store.CIRun) bool { return false },
		func(_ store.Reader, t store.Ticket) string { return whyNotNewFixTicket(key, t) },
		func(p *store.Project, latest store.PushedCommit, _ []store.CIRun) {
			acceptFailedCI(p, latest.Commit, c.now())
		})
	if err != nil {
		return Written[TicketView]{}, err
	}
	_, err = c.updateTicketIn(key, "new_fix_ticket", in.Ticket, true, func(_ store.Reader, t *store.Ticket) (bool, error) {
		addComment(t, ActorOf(ctx), c.now(), 0, fmt.Sprintf("New fix Ticket: %s fixes this CI failure forward, and %s lands anyway.", fix, failed))
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

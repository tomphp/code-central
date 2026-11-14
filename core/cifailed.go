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
	// OptionNewFixTicket is new_fix_ticket: a Ready Ticket on the
	// CI Fixes Spec fixes it forward, and the failed Tickets keep
	// waiting for a later commit that passes (code-central-433).
	OptionNewFixTicket = "new_fix_ticket"
)

// CIFailureSummary is what failed on a CI failed item's pushed commit:
// the commit and its failed runs' workflow file names.
type CIFailureSummary struct {
	Commit    string   `json:"commit"`
	Workflows []string `json:"workflows"`
}

// ciFailureOptions are the failure options the human may choose on t's
// CI item while t is CI failed or CI cancelled, nil otherwise, from its
// Project's evaluation.
func (c *Core) ciFailureOptions(r store.Reader, key string, t store.Ticket) []string {
	p, _ := r.Project(key)
	return c.evaluate(r, p).failureOptions(t)
}

// latestPushed is t's latest pushed commit, the one its CI result comes
// from; t must have one.
func latestPushed(t store.Ticket) store.PushedCommit {
	return t.PushedCommits[len(t.PushedCommits)-1]
}

// acceptFailedCI records on p the human's acceptance of commit's failed
// CI result at now, as Land anyway does.
func acceptFailedCI(p *store.Project, commit string, now time.Time) {
	respond(p, commit, func(res *store.CIResponse) { res.LandAnyway = now })
}

// fixTicketOpen reports whether the human opened a fix Ticket for
// commit's failure that is neither Done nor Cancelled, which leaves the
// commit's CI failed item out of Needs Me.
func fixTicketOpen(r store.Reader, p store.Project, commit string) bool {
	n := responseOf(p, commit).FixTicket
	if n == 0 {
		return false
	}
	fix, ok := r.Ticket(p.Key, n)
	return ok && fix.State != store.Done && fix.State != store.Cancelled
}

// respondCI is the human's op, land_anyway or rerun_ci, on the Ticket
// named: when the Ticket's CI item offers op, apply records the response
// on the Project for the Ticket's latest pushed commit, so it acts on
// every Ticket the item lists, each of which gets a change notice. done
// says the response is already recorded, so choosing it again changes
// nothing; whyNot says why op isn't offered. Each reads the Project's CI
// state as it was before the response, and the item and its options
// come from the Project's evaluation then.
func (c *Core) respondCI(key, op string, ref Ref, full bool, done func(*ciState, store.Ticket) bool,
	whyNot func(*ciState, store.Ticket) string, apply func(p *store.Project, commit string, runs []store.CIRun)) (Written[TicketView], error) {
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
		e := c.evaluate(w, p)
		if t.State == store.Accepted && len(t.PushedCommits) > 0 && done(e.ci, t) {
			v, err = acknowledge(before, c.ticketView(w, key, t), full)
			return err
		}
		if !slices.Contains(e.failureOptions(t), op) {
			return &Error{Code: ActionNotAllowed, Message: whyNot(e.ci, t)}
		}
		it, _ := e.itemOf(t)
		for _, o := range it.tickets {
			listed = append(listed, o.Number)
		}
		apply(&p, latestPushed(t).Commit, e.ci.ticket(t).runs)
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
		func(ci *ciState, t store.Ticket) bool { return ci.landAnyway(t) },
		whyNotLandAnyway,
		func(p *store.Project, commit string, _ []store.CIRun) {
			acceptFailedCI(p, commit, c.now())
		})
}

// whyNotLandAnyway says why t isn't offered Land anyway.
func whyNotLandAnyway(ci *ciState, t store.Ticket) string {
	id := DisplayID(ci.p.Key, t.Number)
	if t.State != store.Accepted {
		return fmt.Sprintf("%s is %s: only an Accepted Ticket whose CI failed lands anyway", id, t.State)
	}
	if a := ci.ticket(t); a.failure == nil && !a.cancelled {
		return fmt.Sprintf("%s has no failed or cancelled CI result to land anyway", id)
	}
	if lp := landingPolicyOf(ci.p); !lp.OffersLandAnyway() {
		if lp.LandsByPullRequest() {
			return fmt.Sprintf("%s's Project lands by Pull Request, so your merge on the Code Host decides whether it lands, whatever its CI result", id)
		}
		return fmt.Sprintf("%s's Project lands by Branch, so your merge decides whether it lands, whatever its CI result", id)
	}
	// Unreached: under Merge + Push an Accepted Ticket whose CI failed or
	// was cancelled is on a CI item, which offers Land anyway.
	return fmt.Sprintf("%s's CI item doesn't offer Land anyway", id)
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
		func(ci *ciState, t store.Ticket) bool { return len(ci.ticket(t).reruns) > 0 },
		func(_ *ciState, t store.Ticket) string { return whyNotRerun(key, t) },
		func(p *store.Project, commit string, runs []store.CIRun) {
			requestRerun(p, commit, runs, c.now())
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
// Ready Ticket on the Project's CI Fixes Spec, created on first use as a
// Long-lived Spec, its description the note, and stores it once on the
// failed Ticket's pushed commit's response. It doesn't accept the failed
// CI result: every Ticket the item lists stays Accepted, and lands when a
// later commit passes CI. The item is left out of Needs Me while the fix
// Ticket is neither Done nor Cancelled, and returns if it is Cancelled
// while the commit is still failing. A comment on the named Ticket names
// the fix Ticket. Under Merge + Push the failed work is already on the
// remote's base, so the fix Ticket fixes it forward. An update writes one
// record, so the Spec, the fix Ticket and the response are written in
// that order: nothing names what doesn't exist. Its result is the fix
// Ticket's create Acknowledgement.
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
	var failed store.Ticket
	err := c.store.View(func(r store.Reader) error {
		if err := writable(r, key); err != nil {
			return err
		}
		t, err := resolveTicket(r, key, "new_fix_ticket", "ticket", in.Ticket)
		if err != nil {
			return err
		}
		if !slices.Contains(c.ciFailureOptions(r, key, t), OptionNewFixTicket) {
			return &Error{Code: ActionNotAllowed, Message: whyNotNewFixTicket(key, t)}
		}
		failed = t
		return nil
	})
	if err != nil {
		return Written[TicketView]{}, err
	}
	spec, err := c.ciFixesSpec(key)
	if err != nil {
		return Written[TicketView]{}, err
	}
	var v Written[TicketView]
	var fix int
	err = c.store.Update(func(w store.Writer) error {
		if err := writable(w, key); err != nil {
			return err
		}
		now := c.now()
		id := DisplayID(key, failed.Number)
		f := store.Ticket{
			Number: w.NextNumber(key), Title: fmt.Sprintf("Fix the CI failure of %s: %s", id, failed.Title), State: store.Ready, Spec: spec,
			Created: now, Updated: now,
			Description: fmt.Sprintf("%s's CI failed: fix the failure forward, on top of its work in the base. It stays Accepted, and lands when a later commit passes CI.\n\n%s", id, in.Note),
		}
		if err := w.PutTicket(key, f); err != nil {
			return err
		}
		fix = f.Number
		v, err = acknowledgeCreate(c.ticketView(w, key, store.Ticket{}), c.ticketView(w, key, f), in.Full)
		return err
	})
	if err != nil {
		return Written[TicketView]{}, err
	}
	_, err = c.respondCI(key, OptionNewFixTicket, in.Ticket, true,
		func(*ciState, store.Ticket) bool { return false },
		func(_ *ciState, t store.Ticket) string { return whyNotNewFixTicket(key, t) },
		func(p *store.Project, commit string, _ []store.CIRun) {
			respond(p, commit, func(res *store.CIResponse) { res.FixTicket = fix })
		})
	if err != nil {
		return Written[TicketView]{}, err
	}
	_, err = c.updateTicketIn(key, "new_fix_ticket", in.Ticket, true, func(_ store.Reader, t *store.Ticket) (bool, error) {
		addComment(t, ActorOf(ctx), c.now(), 0, fmt.Sprintf("New fix Ticket: %s fixes this CI failure forward; %s stays Accepted and lands when a later commit passes CI.", DisplayID(key, fix), DisplayID(key, t.Number)))
		return true, nil
	})
	if err != nil {
		return Written[TicketView]{}, err
	}
	return v, nil
}

// ciFixesSpecBody is the CI Fixes Spec's body.
const ciFixesSpecBody = "This Spec holds the fix Tickets the human opens with New fix Ticket when a commit's CI failed. " +
	"Each fixes its failure forward, and the Tickets waiting on that commit land when a later commit passes CI.\n"

// ciFixesSpec is key's CI Fixes Spec, a Long-lived Spec created on first
// use; the Spec is written before the Project names it.
func (c *Core) ciFixesSpec(key string) (int, error) {
	n := 0
	err := c.store.Update(func(w store.Writer) error {
		if err := writable(w, key); err != nil {
			return err
		}
		if p, _ := w.Project(key); p.CIFixesSpec != 0 {
			n = p.CIFixesSpec
			return nil
		}
		now := c.now()
		n = w.NextNumber(key)
		return w.PutSpec(key, store.Spec{Number: n, Title: "CI Fixes", Body: ciFixesSpecBody, Labels: []string{LongLivedLabel}, Created: now, Updated: now})
	})
	if err != nil {
		return 0, err
	}
	err = c.store.Update(func(w store.Writer) error {
		p, _ := w.Project(key)
		if p.CIFixesSpec != 0 {
			n = p.CIFixesSpec
			return nil
		}
		p.CIFixesSpec = n
		return w.PutProject(p)
	})
	return n, err
}

// whyNotNewFixTicket says why t isn't offered New fix Ticket.
func whyNotNewFixTicket(key string, t store.Ticket) string {
	id := DisplayID(key, t.Number)
	if t.State != store.Accepted {
		return fmt.Sprintf("%s is %s: only an Accepted Ticket whose CI failed gets a fix Ticket", id, t.State)
	}
	return fmt.Sprintf("%s has no failed CI result waiting on you to fix", id)
}

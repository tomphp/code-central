package core

import (
	"cmp"
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/tomoram/code-central/domain"
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

// ciFailureOptions are the failure options the human may choose on t's
// first CI item while t is CI failed or CI cancelled, nil otherwise.
func (e *evaluation) ciFailureOptions(t domain.Ticket) []CIFailureOption {
	it, ok := e.itemOf(t)
	if !ok {
		return nil
	}
	return e.failureOptions(t, it.repository)
}

// latestPushed is t's latest pushed commit in any Repository; t must have
// one.
func latestPushed(t domain.Ticket) domain.PushedCommit {
	return t.PushedCommits[len(t.PushedCommits)-1]
}

// acceptFailedCI records on p the human's acceptance of the failed CI
// result of commit, in the Repository record names, at now, as Land
// anyway does.
func acceptFailedCI(p *domain.Project, record, commit string, now time.Time) {
	respond(p, record, commit, func(res *domain.CIResponse) { res.LandAnyway = now })
}

// fixTicketOpen is the fix Ticket the human, or Auto-fix, opened for the
// failure of commit, in the Repository record names, while it is neither
// Done nor Cancelled, which leaves the commit's CI failed item out of
// Needs Me.
func fixTicketOpen(r store.Reader, p domain.Project, record, commit string) (domain.Ticket, bool) {
	n := responseOf(p, record, commit).FixTicket
	if n == 0 {
		return domain.Ticket{}, false
	}
	fix, ok := r.Ticket(p.Key, n)
	return fix, ok && !finished(fix.State)
}

// itemRepository is the Repository Name of t's CI item an op acts on:
// given, which must name one of the Project's Repositories, or with none
// given, that of t's only CI item, the Home Repository when it has none.
// A Ticket with CI items in several Repositories needs one given.
func itemRepository(e *evaluation, t domain.Ticket, given string) (string, error) {
	if strings.TrimSpace(given) != "" {
		return repositoryOf(e.p, "repository", given)
	}
	items := e.itemsOf(t)
	switch len(items) {
	case 0:
		return e.p.Key, nil
	case 1:
		return items[0].repository, nil
	}
	var names []string
	for _, it := range items {
		names = append(names, it.repository)
	}
	return "", invalid("repository", "%s has CI items in %s: name the Repository whose item to act on", DisplayID(e.p.Key, t.Number), strings.Join(names, " and "))
}

// respondCI is the human's op, land_anyway, rerun_ci or new_fix_ticket,
// on the Ticket named, in the Repository given: when the Ticket's CI item
// there offers op, apply records the response on the Project for the
// Ticket's latest pushed commit there, so it acts on every Ticket the
// item lists, each of which gets a change notice, and on that Repository
// only. done says the response is already recorded, so choosing it again
// changes nothing; whyNot says why op isn't offered, given the Ticket's
// answers in the Repository, nil while it has none. Each reads the
// Project's CI state as it was before the response, and the item and its
// options come from the Project's evaluation then.
func (c *Core) respondCI(key string, op CIFailureOption, in CIResponseInput, done func(*ciState, *repoCI) bool,
	whyNot func(*ciState, domain.Ticket, *repoCI) string, apply func(p *domain.Project, rc *repoCI)) (Written[TicketView], error) {
	var v Written[TicketView]
	var listed []int
	err := c.store.Update(func(w store.Writer) error {
		listed = nil
		if err := writable(w, key); err != nil {
			return err
		}
		// Its view is the result, so it is read whole.
		t, err := resolveTicket(w, key, string(op), "ticket", in.Ticket, whole...)
		if err != nil {
			return err
		}
		p, _ := w.Project(key)
		// With full the before view isn't compared, so it isn't built.
		var before TicketView
		if !in.Full {
			before = c.ticketView(w, key, t)
		}
		e := c.evaluate(w, p)
		name, err := itemRepository(e, t, in.Repository)
		if err != nil {
			return err
		}
		rc := e.ci.ticket(t).in(name)
		if t.State == domain.Accepted && rc != nil && done(e.ci, rc) {
			v, err = acknowledge(before, c.ticketView(w, key, t), in.Full)
			return err
		}
		if !slices.Contains(e.failureOptions(t, name), op) {
			return &Error{Code: ActionNotAllowed, Message: whyNot(e.ci, t, rc)}
		}
		it, _ := e.itemIn(t, name)
		for _, o := range it.tickets {
			listed = append(listed, o.Number)
		}
		apply(&p, rc)
		if err := w.PutProject(p); err != nil {
			return err
		}
		v, err = acknowledge(before, c.ticketView(w, key, t), in.Full)
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

// inRepository is " in <name>" while p has more than one Repository, for
// a message about the Repository name; "" otherwise.
func inRepository(p domain.Project, name string) string {
	if severalRepositories(p) {
		return " in " + name
	}
	return ""
}

// LandAnyway is the human's land_anyway(ticket, repository?): it accepts
// the failed CI result of an Accepted Ticket's latest pushed commit in
// the Repository, recorded once on the Project for that commit, so the
// Factory lands every Ticket its CI item lists in that Repository on its
// next poll. A newer pushed commit has its own CI result, which it
// doesn't cover. Choosing it again changes nothing.
func (c *Core) LandAnyway(ctx context.Context, key string, in CIResponseInput) (Written[TicketView], error) {
	if err := humanOnly(ctx, "land_anyway"); err != nil {
		return Written[TicketView]{}, err
	}
	// Not between an Auto-fix sweep's read and its fix Ticket's response.
	defer c.lockAutoFix(key)()
	return c.respondCI(key, OptionLandAnyway, in,
		func(ci *ciState, rc *repoCI) bool { return ci.landAnyway(rc) },
		whyNotLandAnyway,
		func(p *domain.Project, rc *repoCI) {
			acceptFailedCI(p, rc.record, rc.latest.Commit, c.now())
		})
}

// whyNotLandAnyway says why t isn't offered Land anyway in the
// Repository rc answers for, nil while t has no CI result there.
func whyNotLandAnyway(ci *ciState, t domain.Ticket, rc *repoCI) string {
	id := DisplayID(ci.p.Key, t.Number)
	if t.State != domain.Accepted {
		return fmt.Sprintf("%s is %s: only an Accepted Ticket whose CI failed lands anyway", id, t.State)
	}
	if rc == nil || rc.failure == nil && !rc.cancelled {
		where := ""
		if rc != nil {
			where = inRepository(ci.p, rc.name)
		}
		return fmt.Sprintf("%s has no failed or cancelled CI result%s to land anyway", id, where)
	}
	switch lp := landingPolicyIn(ci.p, rc.name); {
	case lp.OffersLandAnyway():
	case lp.LandsByPullRequest():
		return fmt.Sprintf("%s lands by Pull Request%s, so your merge on the Code Host decides whether it lands, whatever its CI result", id, inRepository(ci.p, rc.name))
	case lp.LandsOnHumanMerge():
		return fmt.Sprintf("%s lands by Branch%s, so your merge decides whether it lands, whatever its CI result", id, inRepository(ci.p, rc.name))
	case localOnlyIn(ci.p, rc.name):
		return fmt.Sprintf("%s lands in %s once merged, whatever its CI result: %s is local-only, with no Clone URL, so nothing is pushed there", id, rc.name, rc.name)
	default:
		return fmt.Sprintf("%s lands%s once merged, whatever its CI result: it lands by Merge without Push there", id, inRepository(ci.p, rc.name))
	}
	// Unreached: under a Repository's Merge + Push an Accepted Ticket
	// whose CI failed or was cancelled there is on a CI item, which offers
	// Land anyway.
	return fmt.Sprintf("%s's CI item doesn't offer Land anyway", id)
}

// RerunCI is the human's rerun_ci(ticket, repository?): a Re-run request
// for each run that failed on a CI failed Ticket's latest pushed commit
// in the Repository, at its latest attempt, recorded once on the Project
// for that commit in place of any earlier request, so every Ticket its CI
// item lists waits on it. Their CI result there reads running until the
// CI Watcher, which re-runs the runs' failed jobs once, reports each
// run's new attempt, which answers the request, or the request lapses.
// Asking again while it waits changes nothing.
func (c *Core) RerunCI(ctx context.Context, key string, in CIResponseInput) (Written[TicketView], error) {
	if err := humanOnly(ctx, "rerun_ci"); err != nil {
		return Written[TicketView]{}, err
	}
	// Not between an Auto-fix sweep's read and its fix Ticket's response.
	defer c.lockAutoFix(key)()
	return c.respondCI(key, OptionRerunCI, in,
		func(_ *ciState, rc *repoCI) bool { return len(rc.reruns) > 0 },
		func(_ *ciState, t domain.Ticket, _ *repoCI) string { return whyNotRerun(key, t) },
		func(p *domain.Project, rc *repoCI) {
			requestRerun(p, rc.record, rc.latest.Commit, rc.runs, c.now())
		})
}

// requestRerun records on p a Re-run request for each of commit's
// rerunnable runs, in the Repository record names, in place of any
// earlier request.
func requestRerun(p *domain.Project, record, commit string, runs []domain.CIRun, now time.Time) {
	respond(p, record, commit, func(res *domain.CIResponse) {
		res.Reruns = nil
		for _, run := range rerunnableRuns(commit, runs) {
			res.Reruns = append(res.Reruns, domain.CIRunAttempt{ID: run.ID, Attempt: run.Attempt})
		}
		slices.SortFunc(res.Reruns, func(a, b domain.CIRunAttempt) int { return cmp.Compare(a.ID, b.ID) })
		res.Rerun = now
	})
}

// whyNotRerun says why t isn't offered Re-run failed jobs.
func whyNotRerun(key string, t domain.Ticket) string {
	id := DisplayID(key, t.Number)
	if t.State != domain.Accepted {
		return fmt.Sprintf("%s is %s: only an Accepted Ticket whose CI failed has failed jobs to re-run", id, t.State)
	}
	return fmt.Sprintf("%s has no failed or cancelled CI result to re-run", id)
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
	// One fix Ticket per commit: an Auto-fix sweep can't open its own
	// between this check and the response.
	defer c.lockAutoFix(key)()
	var failed domain.Ticket
	// name is the failed item's Repository, which where names in the fix
	// Ticket's title while the Project has more than one.
	var name, where string
	err := c.store.View(func(r store.Reader) error {
		if err := writable(r, key); err != nil {
			return err
		}
		t, err := resolveTicket(r, key, "new_fix_ticket", "ticket", in.Ticket)
		if err != nil {
			return err
		}
		p, _ := r.Project(key)
		e := c.evaluate(r, p)
		if name, err = itemRepository(e, t, in.Repository); err != nil {
			return err
		}
		if !slices.Contains(e.failureOptions(t, name), OptionNewFixTicket) {
			return &Error{Code: ActionNotAllowed, Message: whyNotNewFixTicket(key, t)}
		}
		failed, where = t, inRepository(p, name)
		return nil
	})
	if err != nil {
		return Written[TicketView]{}, err
	}
	return c.openFixTicket(ctx, key, failed, name, where, in.Note, nil, in.Full)
}

// fixTicketDescription is a fix Ticket's description: the header naming
// the failed Ticket and where, then body, which is the Run summary and the
// Fix instructions, as the human edited them or as Auto-fix wrote them.
func fixTicketDescription(id, where, body string) string {
	return fmt.Sprintf("%s's CI failed%s: fix the failure forward, on top of its work in the base. It stays Accepted, and lands when a later commit passes CI.\n\n%s", id, where, body)
}

// openFixTicket is the path New fix Ticket takes, shared with Auto-fix:
// it creates the Project's CI Fixes Spec on first use, opens the Ready fix
// Ticket on it with description as the body after the header and mark, nil
// for a manual fix Ticket, stores it on the response of failed's pushed
// commit in the Repository name, and comments on failed as ctx's Actor. An
// update writes one record, so they are written in that order: nothing
// names what doesn't exist. Its result is the fix Ticket's create
// Acknowledgement.
func (c *Core) openFixTicket(ctx context.Context, key string, failed domain.Ticket, name, where, body string, mark *domain.AutoFixMark, full bool) (Written[TicketView], error) {
	spec, err := c.projectSpecNumber(key, ciFixesSpec)
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
		f := domain.Ticket{
			Number: w.NextNumber(key), Title: fmt.Sprintf("Fix the CI failure of %s%s: %s", id, where, failed.Title), State: domain.Ready, Spec: spec,
			Created: now, Updated: now,
			Description: fixTicketDescription(id, where, body), AutoFix: mark,
		}
		if err := whole.put(w, key, &f); err != nil {
			return err
		}
		fix = f.Number
		v, err = acknowledgeCreate(c.ticketView(w, key, domain.Ticket{}), c.ticketView(w, key, f), full)
		return err
	})
	if err != nil {
		return Written[TicketView]{}, err
	}
	if err := c.recordFixTicket(ctx, key, failed, name, fix); err != nil {
		return Written[TicketView]{}, err
	}
	return v, nil
}

// recordFixTicket ends openFixTicket once the fix Ticket fix exists: it
// stores fix on the response of failed's pushed commit in the Repository
// name, then comments on failed, naming fix, as ctx's Actor.
func (c *Core) recordFixTicket(ctx context.Context, key string, failed domain.Ticket, name string, fix int) error {
	ref := Ref(DisplayID(key, failed.Number))
	_, err := c.respondCI(key, OptionNewFixTicket, CIResponseInput{Ticket: ref, Repository: name, Full: true},
		func(*ciState, *repoCI) bool { return false },
		func(_ *ciState, t domain.Ticket, _ *repoCI) string { return whyNotNewFixTicket(key, t) },
		func(p *domain.Project, rc *repoCI) {
			respond(p, rc.record, rc.latest.Commit, func(res *domain.CIResponse) { res.FixTicket = fix })
		})
	if err != nil {
		return err
	}
	_, err = c.updateTicketIn(key, "new_fix_ticket", ref, true, func(_ store.Reader, t *domain.Ticket) (bool, error) {
		addComment(t, ActorOf(ctx), c.now(), 0, fmt.Sprintf("New fix Ticket: %s fixes this CI failure forward; %s stays Accepted and lands when a later commit passes CI.", DisplayID(key, fix), DisplayID(key, t.Number)))
		return true, nil
	})
	return err
}

// ciFixesSpecBody is the CI Fixes Spec's body.
const ciFixesSpecBody = "This Spec holds the fix Tickets the human opens with New fix Ticket, or Auto-fix opens, when a commit's CI failed. " +
	"Each fixes its failure forward, and the Tickets waiting on that commit land when a later commit passes CI.\n"

// whyNotNewFixTicket says why t isn't offered New fix Ticket.
func whyNotNewFixTicket(key string, t domain.Ticket) string {
	id := DisplayID(key, t.Number)
	if t.State != domain.Accepted {
		return fmt.Sprintf("%s is %s: only an Accepted Ticket whose CI failed gets a fix Ticket", id, t.State)
	}
	return fmt.Sprintf("%s has no failed CI result waiting on you to fix", id)
}

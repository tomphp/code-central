package core

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/tomoram/code-central/store"
)

// Blockers (ticket 28). A Blocker is a many-to-many link between Tickets
// in one Project, across Specs, stored on the dependant as blocked_by.
// Blocking is computed on read, by blockRule: Done clears a Blocker, as
// does, under Unblock on push, a CI result running or passed while the CI
// gate holds it (code-central-298). A Cancelled Blocker is followed
// through its Replacements (store.Follow), so cancelling never rewrites
// another Ticket's file.

// BlockerInput is add_blocker's and remove_blocker's input: ticket waits
// on blocker.
type BlockerInput struct {
	Ticket  Ref `json:"ticket"`
	Blocker Ref `json:"blocker"`
	// Full asks for the Ticket in place of the Acknowledgement.
	Full bool `json:"full,omitempty"`
}

// BlockerView is one Blocker as get_ticket shows it, after following
// Replacements. Via names the Cancelled Blocker it stands in for, if any.
type BlockerView struct {
	ID    string      `json:"id"`
	Title string      `json:"title"`
	State store.State `json:"state"`
	Via   string      `json:"via,omitempty"`
	// CIResult is the Blocker's CI result while the CI gate holds it,
	// whether or not Unblock on push is on, and Cleared is whether it no
	// longer blocks though it isn't Done (code-central-299).
	CIResult CIResult `json:"ci_result,omitempty"`
	Cleared  bool     `json:"cleared"`
}

// AddBlocker is add_blocker(ticket, blocker). A link already there
// writes nothing; one that would form a cycle is rejected with cycle.
func (c *Core) AddBlocker(_ context.Context, key string, in BlockerInput) (Written[TicketView], error) {
	return c.updateTicketIn(key, "add_blocker", in.Ticket, in.Full, func(r store.Reader, t *store.Ticket) (bool, error) {
		blocker, err := resolveTicket(r, key, "add_blocker", "blocker", in.Blocker)
		if err != nil {
			return false, err
		}
		b := blocker.Number
		if b == t.Number {
			return false, &Error{Code: Cycle, Message: fmt.Sprintf("%s can't block itself", DisplayID(key, b))}
		}
		if slices.Contains(t.BlockedBy, b) {
			return false, nil
		}
		next := *t
		next.BlockedBy = append(slices.Clone(t.BlockedBy), b)
		ov := overlay{Reader: r, key: key, t: next}
		if path := waitPath(ov, key, store.Follow(ov, key, b), func(n int) bool { return n == t.Number }); path != nil {
			return false, cycleError(key, append([]int{t.Number}, path...))
		}
		t.BlockedBy = next.BlockedBy
		return true, nil
	})
}

// RemoveBlocker is remove_blocker(ticket, blocker): it removes a stored
// link. A Ticket that blocks only through a Cancelled Blocker's
// Replacements is unlinked by removing that Cancelled Blocker.
func (c *Core) RemoveBlocker(_ context.Context, key string, in BlockerInput) (Written[TicketView], error) {
	return c.updateTicketIn(key, "remove_blocker", in.Ticket, in.Full, func(r store.Reader, t *store.Ticket) (bool, error) {
		b, err := in.Blocker.number(key, "blocker")
		if err != nil {
			return false, err
		}
		if err := specNotTicket(r, key, "remove_blocker", "blocker", b); err != nil {
			return false, err
		}
		if slices.Contains(t.BlockedBy, b) {
			t.BlockedBy = slices.DeleteFunc(t.BlockedBy, func(n int) bool { return n == b })
			return true, nil
		}
		for _, bv := range c.blockRule(r, key).blockerViews(*t) {
			if bv.ID == DisplayID(key, b) && bv.Via != "" {
				return false, invalid("blocker", "%s blocks %s only as a Replacement for Cancelled %s; remove %s instead",
					bv.ID, DisplayID(key, t.Number), bv.Via, bv.Via)
			}
		}
		return false, invalid("blocker", "%s is not a Blocker of %s", DisplayID(key, b), DisplayID(key, t.Number))
	})
}

// blockRule is what the blocked check reads: the store, the Blockers'
// Project's settings and the current time. It holds the Project's
// settings, not its CI Runs, so a check that needs a Blocker's CI result
// reads them only for that Blocker. Every caller asks it, so blocking is
// worked out one way.
type blockRule struct {
	r   store.Reader
	p   store.Project
	now time.Time
}

// blockRule is the blocked check for Project key's Tickets now.
func (c *Core) blockRule(r store.Reader, key string) blockRule {
	p, _ := r.Project(key)
	return blockRule{r: r, p: p, now: c.now()}
}

// blocked is computed: an unfinished Ticket with a Blocker, after
// following Replacements, that isn't Done or cleared. Finished work is
// never blocked.
func (b blockRule) blocked(t store.Ticket) bool {
	return len(b.openBlockers(t)) > 0
}

// openBlockers are the Blockers keeping an unfinished t blocked.
func (b blockRule) openBlockers(t store.Ticket) []BlockerView {
	if finished(t.State) {
		return nil
	}
	return slices.DeleteFunc(b.blockerViews(t), func(v BlockerView) bool { return v.State == store.Done || v.Cleared })
}

// blockerViews lists t's Blockers after following Replacements, each once.
func (b blockRule) blockerViews(t store.Ticket) []BlockerView {
	key := b.p.Key
	out := []BlockerView{}
	seen := map[int]bool{}
	for _, n := range t.BlockedBy {
		for _, m := range store.Follow(b.r, key, n) {
			if seen[m] {
				continue
			}
			seen[m] = true
			bt, _ := b.r.Ticket(key, m)
			v := BlockerView{ID: DisplayID(key, m), Title: bt.Title, State: bt.State, CIResult: b.gatedCIResult(bt)}
			v.Cleared = b.p.CI.UnblockOnPush && (v.CIResult == CIRunning || v.CIResult == CIPassed)
			if m != n {
				v.Via = DisplayID(key, n)
			}
			out = append(out, v)
		}
	}
	return out
}

// gatedCIResult is bt's CI result while the CI gate holds it, else "".
// Under Unblock on push a result running (which includes no runs
// reported, or a pending Re-run request) or passed clears bt; a failed
// result, or none once the pushed commit's runs are pruned, keeps it
// blocking. ciGated reads the Project's CI Runs only for a Ticket
// Accepted with a pushed commit under Merge + Push with CI Tracking on.
// A push no later than bt's last move back to Ready, as when it was sent
// back and accepted again but not yet pushed, isn't its work, so it has
// no CI result here and keeps blocking. Times are whole seconds, and a
// new push always comes after the work done since.
func (b blockRule) gatedCIResult(bt store.Ticket) CIResult {
	pushed, runs, ok := ciGated(b.r, b.p, bt, b.now)
	if !ok || !pushed.Pushed.After(backToReady(bt)) {
		return ""
	}
	return latestCIResult(pushed, runs, b.now)
}

// backToReady is when t last moved to Ready from another State, or zero
// if it never has; its creation doesn't count.
func backToReady(t store.Ticket) time.Time {
	for _, m := range slices.Backward(t.Moves) {
		if m.To == store.Ready && m.From != "" {
			return m.Time
		}
	}
	return time.Time{}
}

// blockedError rejects claiming a blocked Ticket, naming its Blockers,
// with the CI result of one the CI gate holds, whether or not Unblock on
// push is on.
func (b blockRule) blockedError(t store.Ticket, actor Actor) error {
	var names []string
	for _, v := range b.openBlockers(t) {
		s := fmt.Sprintf("%s (%s", v.ID, v.State)
		if v.State == store.Cancelled {
			s += ", with no Replacements"
		}
		if v.CIResult != "" {
			s += ", CI " + string(v.CIResult)
		}
		if v.Via != "" {
			s += ", replacing Cancelled " + v.Via
		}
		names = append(names, s+")")
	}
	allowed := slices.DeleteFunc(allowedActions(t.State, actor), func(a string) bool { return a == "claim" })
	msg := fmt.Sprintf("%s is blocked by %s, so claim is not allowed", DisplayID(b.p.Key, t.Number), strings.Join(names, ", "))
	if len(allowed) > 0 {
		msg += "; allowed from " + string(t.State) + ": " + strings.Join(allowed, ", ")
	}
	return &Error{Code: ActionNotAllowed, Message: msg, AllowedActions: allowed}
}

// waitsOn is what Ticket n waits on: its Blockers, followed through
// Replacements. A Cancelled Ticket waits on nothing.
func waitsOn(r store.Reader, key string, n int) []int {
	t, ok := r.Ticket(key, n)
	if !ok || t.State == store.Cancelled {
		return nil
	}
	var out []int
	for _, b := range t.BlockedBy {
		out = append(out, store.Follow(r, key, b)...)
	}
	return out
}

// waitPath finds a chain of waits from one of from to a Ticket for which
// target is true, or nil.
func waitPath(r store.Reader, key string, from []int, target func(int) bool) []int {
	visited := map[int]bool{}
	var visit func(n int, path []int) []int
	visit = func(n int, path []int) []int {
		path = append(slices.Clip(path), n)
		if target(n) {
			return path
		}
		if visited[n] {
			return nil
		}
		visited[n] = true
		for _, m := range waitsOn(r, key, n) {
			if p := visit(m, path); p != nil {
				return p
			}
		}
		return nil
	}
	for _, n := range from {
		if p := visit(n, nil); p != nil {
			return p
		}
	}
	return nil
}

func cycleError(key string, chain []int) error {
	ids := make([]string, len(chain))
	for i, n := range chain {
		ids[i] = DisplayID(key, n)
	}
	return &Error{Code: Cycle, Message: "that would form a cycle: " + strings.Join(ids, " → ") + " (each waits on the next)"}
}

// overlay is a Reader that sees one Ticket as it would be after a write,
// so a cycle can be looked for before anything is written.
type overlay struct {
	store.Reader
	key string
	t   store.Ticket
}

func (o overlay) Ticket(project string, n int) (store.Ticket, bool) {
	if project == o.key && n == o.t.Number {
		return o.t, true
	}
	return o.Reader.Ticket(project, n)
}

// ticketRefs resolves refs, op's field, to existing Tickets' numbers,
// each once.
func ticketRefs(r store.Reader, key, op, field string, refs []Ref) ([]int, error) {
	var out []int
	for _, ref := range refs {
		t, err := resolveTicket(r, key, op, field, ref)
		if err != nil {
			return nil, err
		}
		n := t.Number
		if !slices.Contains(out, n) {
			out = append(out, n)
		}
	}
	return out, nil
}

// updateTicketIn is updateTicket with the Reader, for writes that check
// other Tickets.
func (c *Core) updateTicketIn(key, op string, ref Ref, full bool, fn func(r store.Reader, t *store.Ticket) (bool, error)) (Written[TicketView], error) {
	var v Written[TicketView]
	err := c.store.Update(func(w store.Writer) error {
		if err := writable(w, key); err != nil {
			return err
		}
		t, err := resolveTicket(w, key, op, "ticket", ref)
		if err != nil {
			return err
		}
		// With full the before view isn't compared, so it isn't built.
		var before TicketView
		if !full {
			before = c.ticketView(w, key, t)
		}
		changed, err := fn(w, &t)
		if err != nil {
			return err
		}
		if changed {
			t.Updated = c.now()
			if err := w.PutTicket(key, t); err != nil {
				return err
			}
		}
		v, err = acknowledge(before, c.ticketView(w, key, t), full)
		return err
	})
	return v, err
}

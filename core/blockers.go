package core

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/tomoram/code-central/api"
	"github.com/tomoram/code-central/domain"
	"github.com/tomoram/code-central/store"
)

// Blockers (ticket 28). A Blocker is a many-to-many link between Tickets
// in one Project, across Specs, stored on the dependant as blocked_by.
// Blocking is computed on read, by blockRule: Done clears a Blocker, as
// does a Blocker inside a Spec once it is merged into the Spec Branch. A Cancelled Blocker is followed
// through its Replacements (store.Follow), so cancelling never rewrites
// another Ticket's file.

// blockerCleared reports whether b no longer blocks though it isn't Done.
func blockerCleared(b api.BlockerView) bool { return b.Cleared != nil && *b.Cleared }

// AddBlocker is add_blocker(ticket, blocker). A link already there
// writes nothing; one that would form a cycle is rejected with cycle.
func (c *Core) AddBlocker(_ context.Context, key string, in api.BlockerInput) (api.Written[api.TicketView], error) {
	return c.updateTicketIn(key, "add_blocker", in.Ticket, in.Full, func(r store.Reader, t *domain.Ticket) (bool, error) {
		blocker, err := resolveTicket(r, key, "add_blocker", "blocker", in.Blocker)
		if err != nil {
			return false, err
		}
		b := blocker.Number
		if b == t.Number {
			return false, &api.Error{Code: api.Cycle, Message: fmt.Sprintf("%s can't block itself", domain.DisplayID(key, b))}
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
func (c *Core) RemoveBlocker(_ context.Context, key string, in api.BlockerInput) (api.Written[api.TicketView], error) {
	return c.updateTicketIn(key, "remove_blocker", in.Ticket, in.Full, func(r store.Reader, t *domain.Ticket) (bool, error) {
		b, err := api.RefNumber(in.Blocker, key, "blocker")
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
			if bv.ID == domain.DisplayID(key, b) && bv.Via != "" {
				return false, api.InvalidField("blocker", "%s blocks %s only as a Replacement for Cancelled %s; remove %s instead",
					bv.ID, domain.DisplayID(key, t.Number), bv.Via, bv.Via)
			}
		}
		return false, api.InvalidField("blocker", "%s is not a Blocker of %s", domain.DisplayID(key, b), domain.DisplayID(key, t.Number))
	})
}

// blockRule is what the blocked check reads: the store, the Blockers'
// Project's settings and the current time. It holds the Project's
// settings, not its CI Runs, so a check that needs a Blocker's CI result
// reads them only for that Blocker. Every caller asks it, so blocking is
// worked out one way.
type blockRule struct {
	r   store.Reader
	p   domain.Project
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
func (b blockRule) blocked(t domain.Ticket) bool {
	return len(b.openBlockers(t)) > 0
}

// openBlockers are the Blockers keeping an unfinished t blocked.
func (b blockRule) openBlockers(t domain.Ticket) []api.BlockerView {
	if finished(t.State) {
		return nil
	}
	return slices.DeleteFunc(b.blockerViews(t), func(v api.BlockerView) bool { return v.State == domain.Done || blockerCleared(v) })
}

// blockerViews lists t's Blockers after following Replacements, each once.
func (b blockRule) blockerViews(t domain.Ticket) []api.BlockerView {
	key := b.p.Key
	out := []api.BlockerView{}
	seen := map[int]bool{}
	for _, n := range t.BlockedBy {
		for _, m := range store.Follow(b.r, key, n) {
			if seen[m] {
				continue
			}
			seen[m] = true
			bt, _ := b.r.Ticket(key, m)
			// Within a Spec, a Spec-landed Blocker clears once its work is
			// merged into the Spec Branch.
			cleared := bt.Spec == t.Spec && bt.State == domain.Accepted && specMergeCounts(bt) && specLanded(b.r, b.p, bt)
			v := api.BlockerView{ID: domain.DisplayID(key, m), Title: bt.Title, State: bt.State, Cleared: &cleared}
			if m != n {
				v.Via = domain.DisplayID(key, n)
			}
			out = append(out, v)
		}
	}
	return out
}

// backToReady is when t last moved to Ready from another State, or zero
// if it never has; its creation doesn't count. It is read from the head.
func backToReady(t domain.Ticket) time.Time { return t.Head.BackToReady }

// blockedError rejects claiming a blocked Ticket, naming its Blockers,

func (b blockRule) blockedError(t domain.Ticket, actor api.Actor) error {
	var names []string
	for _, v := range b.openBlockers(t) {
		s := fmt.Sprintf("%s (%s", v.ID, v.State)
		if v.State == domain.Cancelled {
			s += ", with no Replacements"
		}
		if v.Via != "" {
			s += ", replacing Cancelled " + v.Via
		}
		names = append(names, s+")")
	}
	allowed := slices.DeleteFunc(allowedActions(t.State, actor), func(a string) bool { return a == "claim" })
	msg := fmt.Sprintf("%s is blocked by %s, so claim is not allowed", domain.DisplayID(b.p.Key, t.Number), strings.Join(names, ", "))
	if len(allowed) > 0 {
		msg += "; allowed from " + string(t.State) + ": " + strings.Join(allowed, ", ")
	}
	return &api.Error{Code: api.ActionNotAllowed, Message: msg, AllowedActions: allowed}
}

// waitsOn is what Ticket n waits on: its Blockers, followed through
// Replacements. A Cancelled Ticket waits on nothing.
func waitsOn(r store.Reader, key string, n int) []int {
	t, ok := r.Ticket(key, n)
	if !ok || t.State == domain.Cancelled {
		return nil
	}
	var out []int
	for _, b := range t.BlockedBy {
		out = append(out, store.Follow(r, key, b)...)
	}
	return out
}

// waitingOn is the Tickets that wait on Ticket n, in number order. It
// finds the Cancelled Tickets whose Replacements lead to n by repeating
// the ReplacedBy query, then the Tickets any of them, or n, blocks; of
// those, waitsOn keeps the ones whose waits do reach n.
func waitingOn(r store.Reader, key string, n int) []int {
	through, seen := []int{n}, map[int]bool{n: true}
	for i := 0; i < len(through); i++ {
		for _, c := range r.FindTickets(key, store.TicketFilter{ReplacedBy: through[i], States: []domain.State{domain.Cancelled}}) {
			if !seen[c.Number] {
				through, seen[c.Number] = append(through, c.Number), true
			}
		}
	}
	var out []int
	for _, b := range through {
		for _, d := range r.FindTickets(key, store.TicketFilter{BlockedBy: b}) {
			if !slices.Contains(out, d.Number) && slices.Contains(waitsOn(r, key, d.Number), n) {
				out = append(out, d.Number)
			}
		}
	}
	slices.Sort(out)
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
		ids[i] = domain.DisplayID(key, n)
	}
	return &api.Error{Code: api.Cycle, Message: "that would form a cycle: " + strings.Join(ids, " → ") + " (each waits on the next)"}
}

// overlay is a Reader that sees one Ticket as it would be after a write,
// so a cycle can be looked for before anything is written.
type overlay struct {
	store.Reader
	key string
	t   domain.Ticket
}

func (o overlay) Ticket(project string, n int, parts ...store.Part) (domain.Ticket, bool) {
	if project == o.key && n == o.t.Number {
		return o.t, true
	}
	return o.Reader.Ticket(project, n, parts...)
}

// ticketRefs resolves refs, op's field, to existing Tickets' numbers,
// each once.
func ticketRefs(r store.Reader, key, op, field string, refs []api.Ref) ([]int, error) {
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
func (c *Core) updateTicketIn(key, op string, ref api.Ref, full bool, fn func(r store.Reader, t *domain.Ticket) (bool, error)) (api.Written[api.TicketView], error) {
	var v api.Written[api.TicketView]
	err := c.store.Update(func(w store.Writer) error {
		if err := writable(w, key); err != nil {
			return err
		}
		t, h, err := readForWrite(w, key, op, "ticket", ref, whole)
		if err != nil {
			return err
		}
		// With full the before view isn't compared, so it isn't built.
		var before api.TicketView
		if !full {
			before = c.ticketView(w, key, t)
		}
		changed, err := fn(w, &t)
		if err != nil {
			return err
		}
		if changed {
			t.Updated = c.now()
			if err := h.put(w, key, &t); err != nil {
				return err
			}
		}
		v, err = acknowledge(before, c.ticketView(w, key, t), full)
		return err
	})
	return v, err
}

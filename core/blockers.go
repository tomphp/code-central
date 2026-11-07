package core

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/tomoram/code-central/store"
)

// Blockers (ticket 28). A Blocker is a many-to-many link between Tickets
// in one Project, across Specs, stored on the dependant as blocked_by.
// Blocking is computed on read: only Done clears a Blocker, and a
// Cancelled Blocker is followed through its Replacements (store.Follow),
// so cancelling never rewrites another Ticket's file.

// BlockerInput is add_blocker's and remove_blocker's input: ticket waits
// on blocker.
type BlockerInput struct {
	Ticket  Ref `json:"ticket"`
	Blocker Ref `json:"blocker"`
}

// BlockerView is one Blocker as get_ticket shows it, after following
// Replacements. Via names the Cancelled Blocker it stands in for, if any.
type BlockerView struct {
	ID    string      `json:"id"`
	Title string      `json:"title"`
	State store.State `json:"state"`
	Via   string      `json:"via,omitempty"`
}

// AddBlocker is add_blocker(ticket, blocker). A link already there
// writes nothing; one that would form a cycle is rejected with cycle.
func (c *Core) AddBlocker(_ context.Context, key string, in BlockerInput) (TicketView, error) {
	return c.updateTicketIn(key, in.Ticket, func(r store.Reader, t *store.Ticket) (bool, error) {
		b, err := ticketRef(r, key, in.Blocker, "blocker")
		if err != nil {
			return false, err
		}
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
func (c *Core) RemoveBlocker(_ context.Context, key string, in BlockerInput) (TicketView, error) {
	return c.updateTicketIn(key, in.Ticket, func(r store.Reader, t *store.Ticket) (bool, error) {
		b, err := in.Blocker.number(key, "blocker")
		if err != nil {
			return false, err
		}
		if slices.Contains(t.BlockedBy, b) {
			t.BlockedBy = slices.DeleteFunc(t.BlockedBy, func(n int) bool { return n == b })
			return true, nil
		}
		for _, bv := range blockerViews(r, key, *t) {
			if bv.ID == DisplayID(key, b) && bv.Via != "" {
				return false, invalid("blocker", "%s blocks %s only as a Replacement for Cancelled %s; remove %s instead",
					bv.ID, DisplayID(key, t.Number), bv.Via, bv.Via)
			}
		}
		return false, invalid("blocker", "%s is not a Blocker of %s", DisplayID(key, b), DisplayID(key, t.Number))
	})
}

// blocked is computed: an unfinished Ticket with a Blocker, after
// following Replacements, that isn't Done. Finished work is never blocked.
func blocked(r store.Reader, key string, t store.Ticket) bool {
	return len(openBlockers(r, key, t)) > 0
}

// openBlockers are the Blockers keeping an unfinished t blocked.
func openBlockers(r store.Reader, key string, t store.Ticket) []BlockerView {
	if finished(t.State) {
		return nil
	}
	return slices.DeleteFunc(blockerViews(r, key, t), func(b BlockerView) bool { return b.State == store.Done })
}

// blockerViews lists t's Blockers after following Replacements, each once.
func blockerViews(r store.Reader, key string, t store.Ticket) []BlockerView {
	out := []BlockerView{}
	seen := map[int]bool{}
	for _, b := range t.BlockedBy {
		for _, m := range store.Follow(r, key, b) {
			if seen[m] {
				continue
			}
			seen[m] = true
			bt, _ := r.Ticket(key, m)
			v := BlockerView{ID: DisplayID(key, m), Title: bt.Title, State: bt.State}
			if m != b {
				v.Via = DisplayID(key, b)
			}
			out = append(out, v)
		}
	}
	return out
}

// blockedError rejects claiming a blocked Ticket, naming its Blockers.
func blockedError(r store.Reader, key string, t store.Ticket, actor Actor) error {
	var names []string
	for _, b := range openBlockers(r, key, t) {
		s := fmt.Sprintf("%s (%s", b.ID, b.State)
		if b.State == store.Cancelled {
			s += ", with no Replacements"
		}
		if b.Via != "" {
			s += ", replacing Cancelled " + b.Via
		}
		names = append(names, s+")")
	}
	allowed := slices.DeleteFunc(allowedActions(t.State, actor), func(a string) bool { return a == "claim" })
	msg := fmt.Sprintf("%s is blocked by %s, so claim is not allowed", DisplayID(key, t.Number), strings.Join(names, ", "))
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

// ticketRef resolves ref to an existing Ticket's number.
func ticketRef(r store.Reader, key string, ref Ref, field string) (int, error) {
	n, err := ref.number(key, field)
	if err != nil {
		return 0, err
	}
	if _, ok := r.Ticket(key, n); !ok {
		return 0, invalid(field, "there is no Ticket %s", DisplayID(key, n))
	}
	return n, nil
}

// ticketRefs resolves refs to existing Tickets' numbers, each once.
func ticketRefs(r store.Reader, key string, refs []Ref, field string) ([]int, error) {
	var out []int
	for _, ref := range refs {
		n, err := ticketRef(r, key, ref, field)
		if err != nil {
			return nil, err
		}
		if !slices.Contains(out, n) {
			out = append(out, n)
		}
	}
	return out, nil
}

// updateTicketIn is updateTicket with the Reader, for writes that check
// other Tickets.
func (c *Core) updateTicketIn(key string, ref Ref, fn func(r store.Reader, t *store.Ticket) (bool, error)) (TicketView, error) {
	var v TicketView
	err := c.store.Update(func(w store.Writer) error {
		if _, ok := w.Project(key); !ok {
			return unknownProject(key)
		}
		t, err := ticket(w, key, ref)
		if err != nil {
			return err
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
		v = ticketView(w, key, t)
		return nil
	})
	return v, err
}

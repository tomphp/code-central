package core

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/tomoram/code-central/api"
	"github.com/tomoram/code-central/domain"
	"github.com/tomoram/code-central/store"
)

// Retros (Spec code-central-646). A Retro is a Ticket labelled retro, on
// the Project's Retros Spec, a Long-lived Spec: the human opens one on
// finished work, and its agent reads the sessions of that work's Claims.

// RetroOfTag is the Tag naming the work a Retro looks back on.
const RetroOfTag = "retro-of"

// SubmitRetro is submit_retro(target, note?). It writes, as human whoever
// calls, a Ready Retro titled "Retro: <target title>" on the Retros Spec,
// whose description lists every Claim on the target to read.
func (c *Core) SubmitRetro(_ context.Context, key string, in api.SubmitRetroInput) (api.Written[api.TicketView], error) {
	// The target is checked before the Retros Spec is made, so a refused
	// Retro writes nothing.
	err := c.store.View(func(r store.Reader) error {
		if err := writable(r, key); err != nil {
			return err
		}
		_, err := resolveRetroTarget(r, key, in.Target)
		return err
	})
	if err != nil {
		return api.Written[api.TicketView]{}, err
	}
	spec, err := c.projectSpecNumber(key, retrosSpec)
	if err != nil {
		return api.Written[api.TicketView]{}, err
	}
	var v api.Written[api.TicketView]
	err = c.store.Update(func(w store.Writer) error {
		if err := writable(w, key); err != nil {
			return err
		}
		target, err := resolveRetroTarget(w, key, in.Target)
		if err != nil {
			return err
		}
		now := c.now()
		t := domain.Ticket{
			Number: w.NextNumber(key), Title: "Retro: " + target.title, State: domain.Ready, Spec: spec,
			Labels: []string{RetroLabel}, Tags: map[string]string{RetroOfTag: target.id},
			Created: now, Updated: now, Description: target.description(in.Note),
			Moves: submitMoves(false, now, "submit_retro"),
		}
		if err := whole.put(w, key, &t); err != nil {
			return err
		}
		v, err = acknowledgeCreate(c.ticketView(w, key, domain.Ticket{}), c.ticketView(w, key, t), in.Full)
		return err
	})
	return v, err
}

// retroTarget is the finished work a Retro looks back on.
type retroTarget struct {
	key, id, kind, title string
	// tickets are the target Ticket alone, or every Ticket of the target
	// Spec in claim order.
	tickets []domain.Ticket
}

// resolveRetroTarget reads the finished work ref names: a Done or
// Cancelled Ticket, or a Finished Spec that isn't Long-lived.
func resolveRetroTarget(r store.Reader, key string, ref api.Ref) (retroTarget, error) {
	n, err := refNumber(ref, key, "target")
	if err != nil {
		return retroTarget{}, err
	}
	id := DisplayID(key, n)
	if s, ok := r.Spec(key, n); ok {
		if slices.Contains(s.Labels, LongLivedLabel) {
			return retroTarget{}, invalid("target", "%s is a Long-lived Spec, which never finishes; start a Retro on each of its finished Tickets instead", id)
		}
		tickets := specTickets(r, key, n, store.Claims)
		if _, done := summarise(tickets); !done {
			return retroTarget{}, invalid("target", "Spec %s isn't Finished; a Retro looks back on finished work, so start it once every Ticket is Done or Cancelled", id)
		}
		return retroTarget{key: key, id: id, kind: "Spec", title: s.Title, tickets: tickets}, nil
	}
	if t, ok := r.Ticket(key, n, store.Claims); ok {
		if !finished(t.State) {
			return retroTarget{}, invalid("target", "%s is %s; a Retro looks back on finished work, so start it once the Ticket is Done or Cancelled", id, t.State)
		}
		return retroTarget{key: key, id: id, kind: "Ticket", title: t.Title, tickets: []domain.Ticket{t}}, nil
	}
	return retroTarget{}, invalid("target", "there is no Spec or Ticket %s", id)
}

// ticketRetroable is whether the UI offers the Retro button on t: once it
// is Done or Cancelled, as resolveRetroTarget requires, and while its
// Project p takes writes, which writable refuses. TicketView's retroable.
func ticketRetroable(p domain.Project, t domain.Ticket) bool {
	return finished(t.State) && p.Archived.IsZero()
}

// specRetroable is whether the UI offers the Retro button on s, whose
// Tickets are all finished when specFinished: once it is Finished and
// unless it is Long-lived, as resolveRetroTarget requires, a finished Map
// too, and while its Project p takes writes, which writable refuses.
// SpecDetail's retroable.
func specRetroable(p domain.Project, s domain.Spec, specFinished bool) bool {
	return specFinished && !slices.Contains(s.Labels, LongLivedLabel) && p.Archived.IsZero()
}

// description is a Retro's description: the target as a link, the
// human's note under its own heading when there is one, and the Claims to
// read.
func (rt retroTarget) description(note string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Retro on %s [%s](%s): %s.\n", rt.kind, rt.id, rt.id, rt.title)
	if note = strings.TrimSpace(note); note != "" {
		fmt.Fprintf(&b, "\n## Note\n\n%s\n", note)
	}
	b.WriteString("\n## Claims to read\n\n")
	claims := 0
	for _, t := range rt.tickets {
		for _, cl := range t.Claims {
			claims++
			fmt.Fprintf(&b, "- %s (%s), Claim %d: %s.\n", DisplayID(rt.key, t.Number), t.Title, cl.Number, claimToRead(cl))
		}
	}
	if claims == 0 {
		b.WriteString("None: no agent ever claimed this work, so there is no session to read.\n")
	}
	return b.String()
}

// claimToRead is one Claim's entry in a Retro's Claims to read: its
// session, its working folder and how it ended.
func claimToRead(cl domain.Claim) string {
	session := "no session to read"
	if cl.SessionID != "" {
		session = "session_id `" + cl.SessionID + "`"
	}
	dir := "no working_dir"
	if cl.WorkingDir != "" {
		dir = "working_dir `" + cl.WorkingDir + "`"
	}
	ending := "still live"
	if !cl.Live() {
		ending = "ended by " + cl.EndedBy
	}
	return session + ", " + dir + ", " + ending
}

// retrosSpecBody is the Retros Spec's body.
const retrosSpecBody = "This Spec holds the Retros the human opens on finished work. " +
	"Each reads the sessions of that work's Claims and proposes changes to the agents' environment; " +
	"the Tickets for the changes the human picks open here too.\n"

// retrosOf lists the Retros whose retro-of Tag names target, oldest first,
// or nil when there are none.
func retrosOf(r store.Reader, key, target string) []api.RetroView {
	var out []api.RetroView
	for _, t := range r.FindTickets(key, store.TicketFilter{Label: RetroLabel, Tag: &store.TagFilter{Key: RetroOfTag, Values: []string{target}}}) {
		out = append(out, api.RetroView{ID: DisplayID(key, t.Number), Title: t.Title, State: t.State})
	}
	return out
}

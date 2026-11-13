package core

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/tomoram/code-central/store"
)

// Auto-accept (Spec code-central-536, ADR 0037): the human lets finished
// work of chosen kinds skip In Review. The Project's setting is one
// switch per Auto-accept kind, which only the human sets, over HTTP.

// AutoAcceptLabel is the reserved Label that, on a Ticket or its Spec,
// turns every Auto-accept kind on for the Ticket. Only the human adds or
// removes it.
const AutoAcceptLabel = "auto-accept"

// AutoAcceptView is a Project's Auto-accept setting: on or off for
// every Auto-accept kind.
type AutoAcceptView map[store.AutoAcceptKind]bool

// autoAcceptView is p's Auto-accept setting with every kind present.
func autoAcceptView(p store.Project) AutoAcceptView {
	v := AutoAcceptView{}
	for _, k := range store.AutoAcceptKinds {
		v[k] = p.AutoAccepts(k)
	}
	return v
}

// SetAutoAcceptInput is the human's set_auto_accept(kinds, full?).
type SetAutoAcceptInput struct {
	// Kinds is on or off per Auto-accept kind; a kind left out keeps its
	// value.
	Kinds map[store.AutoAcceptKind]bool `json:"kinds"`
	// Full asks for the Project in place of the Acknowledgement.
	Full bool `json:"full,omitempty"`
}

// SetAutoAccept is the human's set_auto_accept: turns the Project's
// Auto-accept setting on or off per kind. It applies to finishes from
// now on, accepting nothing already In Review. Like the Project's other
// settings it adds no Move, Edit or Comment.
func (c *Core) SetAutoAccept(ctx context.Context, key string, in SetAutoAcceptInput) (Written[ProjectView], error) {
	if err := humanOnly(ctx, "set_auto_accept"); err != nil {
		return Written[ProjectView]{}, err
	}
	var unknown []string
	for k := range in.Kinds {
		if !k.Valid() {
			unknown = append(unknown, string(k))
		}
	}
	if len(unknown) > 0 {
		sort.Strings(unknown)
		return Written[ProjectView]{}, invalid("kinds", "unknown Auto-accept kind %s; the kinds are %s", strings.Join(unknown, ", "), joinKinds())
	}
	var before, after ProjectView
	err := c.store.Update(func(w store.Writer) error {
		if err := writable(w, key); err != nil {
			return err
		}
		p, _ := w.Project(key)
		before = c.projectView(p)
		was := slices.Clone(p.AutoAccept)
		for k, on := range in.Kinds {
			p.SetAutoAccept(k, on)
		}
		after = c.projectView(p)
		if slices.Equal(p.AutoAccept, was) {
			return nil
		}
		return w.PutProject(p)
	})
	if err != nil {
		return Written[ProjectView]{}, err
	}
	return acknowledgeProject(before, after, in.Full, c.now())
}

// autoAcceptKind is t's Auto-accept kind, worked out from what it
// carries: its Stage, the Spec merge mark, or Implementation results for
// every other Ticket. A Prototype, a Wayfinder-type Ticket and a Map's
// Ticket have none. sp is t's Spec.
func autoAcceptKind(sp store.Spec, t store.Ticket) (store.AutoAcceptKind, bool) {
	if slices.ContainsFunc(t.Labels, isWayfinderType) || isMap(sp.Labels) {
		return "", false
	}
	switch StageOf(t.Labels) {
	case "grilling":
		return store.AutoAcceptGrilling, true
	case "spec":
		return store.AutoAcceptSpec, true
	case "tickets":
		return store.AutoAcceptTickets, true
	}
	if t.SpecMergeTicket {
		return store.AutoAcceptMerge, true
	}
	return store.AutoAcceptImplementation, true
}

// autoAcceptCause is why t, In Review, is auto-accept eligible in
// Project p, with sp its Spec: the first cause that applies (the Label
// on t, the Label on sp, then p's setting for t's kind), or false when
// none does. A spec Stage is eligible only once a finish has given its
// Spec recommendation.
func autoAcceptCause(p store.Project, sp store.Spec, t store.Ticket) (store.AutoAcceptCause, bool) {
	kind, ok := autoAcceptKind(sp, t)
	switch {
	case !ok || kind == store.AutoAcceptSpec && t.Recommendation == "":
		return "", false
	case slices.Contains(t.Labels, AutoAcceptLabel):
		return store.CauseTicketLabel, true
	case slices.Contains(sp.Labels, AutoAcceptLabel):
		return store.CauseSpecLabel, true
	case p.AutoAccepts(kind):
		return store.CauseProject, true
	}
	return "", false
}

// autoAccept ends finish's write: when t, now In Review, is auto-accept
// eligible it carries out the human's accept in the same write, else it
// writes t as it is.
func (c *Core) autoAccept(ctx context.Context, w store.Writer, key string, t store.Ticket, now time.Time) error {
	p, _ := w.Project(key)
	sp, _ := w.Spec(key, t.Spec)
	if cause, ok := autoAcceptCause(p, sp, t); ok {
		im, ok, err := c.acceptOnBehalf(ctx, w, key, t, cause, now)
		if err != nil {
			return err
		}
		if ok {
			return putRecords(w, key, im)
		}
	}
	return w.PutTicket(key, t)
}

// acceptOnBehalf is the human's accept of t, In Review and auto-accept
// eligible for cause, as the records it rewrites and creates, t among
// them; it writes nothing itself. A spec Stage recommending implement is
// implemented, any other Ticket (a spec Stage recommending split too) is
// accepted. The accepting Move's Actor is human, with the cause. ok is
// false, with no records, when implement isn't allowed just now, so the
// spec Stage stays In Review for the human.
func (c *Core) acceptOnBehalf(ctx context.Context, r store.Reader, key string, t store.Ticket, cause store.AutoAcceptCause, now time.Time) (im store.Import, ok bool, err error) {
	human := WithActor(ctx, Human)
	if t.Recommendation == store.RecommendImplement {
		done, err := c.implementStep(human, r, key, &t, cause, now)
		var e *Error
		if errors.As(err, &e) && e.Code == ActionNotAllowed {
			return store.Import{}, false, nil
		}
		if err != nil {
			return store.Import{}, false, err
		}
		return done.records(t), true, nil
	}
	if err := acceptStep(human, &t, key, cause, now); err != nil {
		return store.Import{}, false, err
	}
	t.Updated = now
	return store.Import{Rewritten: []store.Ticket{t}}, true, nil
}

// sweepAutoAccept is the write that gives a Spec or a Ticket the
// auto-accept Label, given target, that record alone, already labelled:
// every Ticket the Label now covers that is In Review and eligible is
// accepted on the human's behalf in the same write.
func (c *Core) sweepAutoAccept(ctx context.Context, r store.Reader, key string, target store.Import, now time.Time) (store.Import, error) {
	p, _ := r.Project(key)
	var sp store.Spec
	var covered []store.Ticket
	im := target
	if len(target.RewrittenSpecs) == 1 {
		sp = target.RewrittenSpecs[0]
		for _, t := range r.Tickets(key) {
			if t.Spec == sp.Number {
				covered = append(covered, t)
			}
		}
	} else {
		// The labelled Ticket is written once, accepted or as it is.
		covered, im = target.Rewritten, store.Import{}
		sp, _ = r.Spec(key, covered[0].Spec)
	}
	for _, t := range covered {
		if t.State != store.InReview {
			continue
		}
		cause, ok := autoAcceptCause(p, sp, t)
		if !ok {
			continue
		}
		done, ok, err := c.acceptOnBehalf(ctx, r, key, t, cause, now)
		if err != nil {
			return store.Import{}, err
		}
		if ok {
			im.Tickets = append(im.Tickets, done.Tickets...)
			im.Rewritten = append(im.Rewritten, done.Rewritten...)
		}
	}
	if len(im.RewrittenSpecs) == 0 && len(im.Rewritten) == 0 {
		return target, nil
	}
	return im, nil
}

// putRecords makes im the update's one write: a lone rewritten Spec or
// Ticket by its own put, and anything more as one Import.
func putRecords(w store.Writer, key string, im store.Import) error {
	if len(im.Specs) == 0 && len(im.Tickets) == 0 {
		switch {
		case len(im.RewrittenSpecs) == 1 && len(im.Rewritten) == 0:
			return w.PutSpec(key, im.RewrittenSpecs[0])
		case len(im.RewrittenSpecs) == 0 && len(im.Rewritten) == 1:
			return w.PutTicket(key, im.Rewritten[0])
		}
	}
	return w.PutImport(key, im)
}

// joinKinds lists the Auto-accept kinds for an error message.
func joinKinds() string {
	names := make([]string, 0, len(store.AutoAcceptKinds))
	for _, k := range store.AutoAcceptKinds {
		names = append(names, fmt.Sprintf("%q", k))
	}
	return strings.Join(names, ", ")
}

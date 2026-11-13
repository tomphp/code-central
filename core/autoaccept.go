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
// Ticket have none.
func autoAcceptKind(r store.Reader, key string, t store.Ticket) (store.AutoAcceptKind, bool) {
	if slices.ContainsFunc(t.Labels, isWayfinderType) {
		return "", false
	}
	if sp, ok := r.Spec(key, t.Spec); ok && isMap(sp.Labels) {
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

// autoAcceptCause is why t, just finished, is auto-accept eligible: the
// first cause that applies, or false when none does. A spec Stage is
// eligible only once a finish has given its Spec recommendation.
func autoAcceptCause(r store.Reader, key string, t store.Ticket) (store.AutoAcceptCause, bool) {
	kind, ok := autoAcceptKind(r, key, t)
	if !ok || kind == store.AutoAcceptSpec && t.Recommendation == "" {
		return "", false
	}
	if p, ok := r.Project(key); ok && p.AutoAccepts(kind) {
		return store.CauseProject, true
	}
	return "", false
}

// autoAccept ends finish's write: when t, now In Review, is auto-accept
// eligible it carries out the human's accept in the same write, else it
// writes t as it is. A spec Stage recommending implement is implemented,
// unless implement isn't allowed just now, when it stays In Review; any
// other eligible Ticket, including a spec Stage recommending split, is
// accepted. The accepting Move's Actor is human, with the cause.
func (c *Core) autoAccept(ctx context.Context, w store.Writer, key string, t store.Ticket, now time.Time) error {
	cause, ok := autoAcceptCause(w, key, t)
	if !ok {
		return w.PutTicket(key, t)
	}
	human := WithActor(ctx, Human)
	if t.Recommendation == store.RecommendImplement {
		spec := t
		done, err := c.implementStep(human, w, key, &spec, cause, now)
		var e *Error
		if errors.As(err, &e) && e.Code == ActionNotAllowed {
			return w.PutTicket(key, t)
		}
		if err != nil {
			return err
		}
		return w.PutImport(key, done.records(spec))
	}
	if err := acceptStep(human, &t, key, cause, now); err != nil {
		return err
	}
	return w.PutTicket(key, t)
}

// joinKinds lists the Auto-accept kinds for an error message.
func joinKinds() string {
	names := make([]string, 0, len(store.AutoAcceptKinds))
	for _, k := range store.AutoAcceptKinds {
		names = append(names, fmt.Sprintf("%q", k))
	}
	return strings.Join(names, ", ")
}

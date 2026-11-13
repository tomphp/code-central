package core

import (
	"context"
	"fmt"
	"slices"
	"sort"
	"strings"

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

// joinKinds lists the Auto-accept kinds for an error message.
func joinKinds() string {
	names := make([]string, 0, len(store.AutoAcceptKinds))
	for _, k := range store.AutoAcceptKinds {
		names = append(names, fmt.Sprintf("%q", k))
	}
	return strings.Join(names, ", ")
}

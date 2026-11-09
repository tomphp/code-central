package core

import (
	"context"
	"maps"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/tomoram/code-central/store"
)

// EditView is one entry in a Spec's or Ticket's edit history. Previous
// holds the old value of each field the edit changed.
type EditView struct {
	Number   int               `json:"number"`
	Actor    string            `json:"actor"`
	Time     time.Time         `json:"time"`
	Previous map[string]string `json:"previous"`
}

// EditTicketInput is edit_ticket's input. A field left out is unchanged.
type EditTicketInput struct {
	Ticket      Ref     `json:"ticket"`
	Title       *string `json:"title,omitempty"`
	Description *string `json:"description,omitempty"`
	// Full asks for the Ticket in place of the Acknowledgement.
	Full bool `json:"full,omitempty"`
}

// EditTicket is edit_ticket(ticket, title?, description?, full?). An edit
// that changes something records the previous values in edits; one that
// changes nothing writes nothing.
func (c *Core) EditTicket(ctx context.Context, key string, in EditTicketInput) (Written[TicketView], error) {
	if in.Title != nil && strings.TrimSpace(*in.Title) == "" {
		return Written[TicketView]{}, invalid("title", "a Ticket needs a title")
	}
	return c.updateTicket(key, "edit_ticket", in.Ticket, in.Full, func(t *store.Ticket) (bool, error) {
		if err := definitionOwned(ctx, DisplayID(key, t.Number), t.Link); err != nil {
			return false, err
		}
		prev := map[string]string{}
		change(prev, "title", &t.Title, in.Title)
		change(prev, "description", &t.Description, in.Description)
		if len(prev) == 0 {
			return false, nil
		}
		t.Edits = append(t.Edits, store.Edit{Number: len(t.Edits) + 1, Actor: string(ActorOf(ctx)), Time: c.now(), Previous: prev})
		return true, nil
	})
}

// EditSpecInput is edit_spec's input. A field left out is unchanged; a
// body given replaces the old one whole.
type EditSpecInput struct {
	Spec  Ref     `json:"spec"`
	Title *string `json:"title,omitempty"`
	Body  *string `json:"body,omitempty"`
	// Full asks for the Spec in place of the Acknowledgement.
	Full bool `json:"full,omitempty"`
}

// EditSpec is edit_spec(spec, title?, body?, full?), recording edits as
// EditTicket does. The history sits in the Spec's own file.
func (c *Core) EditSpec(ctx context.Context, key string, in EditSpecInput) (Written[SpecView], error) {
	if in.Title != nil && strings.TrimSpace(*in.Title) == "" {
		return Written[SpecView]{}, invalid("title", "a Spec needs a title")
	}
	var v Written[SpecView]
	err := c.store.Update(func(w store.Writer) error {
		if err := writable(w, key); err != nil {
			return err
		}
		s, err := resolveSpec(w, key, "edit_spec", "spec", in.Spec)
		if err != nil {
			return err
		}
		n := s.Number
		if err := definitionOwned(ctx, DisplayID(key, n), s.Link); err != nil {
			return err
		}
		before := specView(key, s)
		prev := map[string]string{}
		change(prev, "title", &s.Title, in.Title)
		change(prev, "body", &s.Body, in.Body)
		if len(prev) > 0 {
			s.Updated = c.now()
			s.Edits = append(s.Edits, store.Edit{Number: len(s.Edits) + 1, Actor: string(ActorOf(ctx)), Time: s.Updated, Previous: prev})
			if err := w.PutSpec(key, s); err != nil {
				return err
			}
		}
		v, err = acknowledge(before, specView(key, s), in.Full)
		return err
	})
	return v, err
}

// change sets *field to *to when to is given and differs, recording the
// old value in prev under name.
func change(prev map[string]string, name string, field *string, to *string) {
	if to == nil || *to == *field {
		return
	}
	prev[name] = *field
	*field = *to
}

// TicketInput names one Ticket, for actions that need nothing else.
type TicketInput struct {
	Ticket Ref `json:"ticket"`
	// Full asks for the Ticket in place of the Acknowledgement.
	Full bool `json:"full,omitempty"`
}

// CommentInput is comment's input.
type CommentInput struct {
	Ticket Ref    `json:"ticket"`
	Body   string `json:"body"`
	// Full asks for the Ticket in place of the Acknowledgement.
	Full bool `json:"full,omitempty"`
}

// DeferInput is defer's input; the reason is required.
type DeferInput struct {
	Ticket Ref    `json:"ticket"`
	Reason string `json:"reason"`
	// Full asks for the Ticket in place of the Acknowledgement.
	Full bool `json:"full,omitempty"`
}

// Comment is comment(ticket, body). It works on any Ticket, including
// one another agent holds, and needs no Claim ID.
func (c *Core) Comment(ctx context.Context, key string, in CommentInput) (Written[TicketView], error) {
	if strings.TrimSpace(in.Body) == "" {
		return Written[TicketView]{}, invalid("body", "a comment needs a body")
	}
	return c.updateTicket(key, "comment", in.Ticket, in.Full, func(t *store.Ticket) (bool, error) {
		addComment(t, ActorOf(ctx), c.now(), 0, in.Body)
		return true, nil
	})
}

// Approve is approve(ticket): Draft to Ready.
func (c *Core) Approve(ctx context.Context, key string, in TicketInput) (Written[TicketView], error) {
	return c.updateTicket(key, "approve", in.Ticket, in.Full, func(t *store.Ticket) (bool, error) {
		return true, transition(ctx, t, key, "approve", "", c.now())
	})
}

// Defer is defer(ticket, reason): Ready to Draft, with the reason stored
// as a comment.
func (c *Core) Defer(ctx context.Context, key string, in DeferInput) (Written[TicketView], error) {
	if strings.TrimSpace(in.Reason) == "" {
		return Written[TicketView]{}, invalid("reason", "defer needs a reason")
	}
	return c.updateTicket(key, "defer", in.Ticket, in.Full, func(t *store.Ticket) (bool, error) {
		if err := transition(ctx, t, key, "defer", "", c.now()); err != nil {
			return false, err
		}
		addComment(t, ActorOf(ctx), c.now(), 0, in.Reason)
		return true, nil
	})
}

// addComment adds a numbered comment to t. claim is the number of the
// Claim a required note was made under, 0 for none.
func addComment(t *store.Ticket, a Actor, at time.Time, claim int, body string) {
	t.Comments = append(t.Comments, store.Comment{Number: len(t.Comments) + 1, Actor: string(a), Time: at, Claim: claim, Body: body})
}

// updateTicket loads the Ticket ref names, for operation op, under the
// write lock and applies fn. When fn reports a change, the Ticket's
// updated time is set and it is written. Its result is the write's
// Acknowledgement, or with full the Ticket.
func (c *Core) updateTicket(key, op string, ref Ref, full bool, fn func(t *store.Ticket) (bool, error)) (Written[TicketView], error) {
	return c.updateTicketIn(key, op, ref, full, func(_ store.Reader, t *store.Ticket) (bool, error) { return fn(t) })
}

// TargetView is a Spec's or Ticket's Labels and Tags, as the Label and
// Tag operations return them. Kind is "spec" or "ticket".
type TargetView struct {
	ID     string            `json:"id"`
	Kind   string            `json:"kind"`
	Labels []string          `json:"labels"`
	Tags   map[string]string `json:"tags"`
}

// LabelsInput is add_labels' and remove_labels' input. Each Label is
// added or removed by name; the rest of the list is left alone, so two
// writers don't overwrite each other.
type LabelsInput struct {
	Target Ref      `json:"target"`
	Labels []string `json:"labels"`
}

// AddLabels is add_labels(target, labels). A Label already present is
// kept once.
func (c *Core) AddLabels(_ context.Context, key string, in LabelsInput) (TargetView, error) {
	if err := checkLabels(in.Labels); err != nil {
		return TargetView{}, err
	}
	if err := checkReservedNames("labels", in.Labels); err != nil {
		return TargetView{}, err
	}
	return c.updateTarget(key, in.Target, func(onSpec bool, labels *[]string, _ map[string]string) error {
		if err := checkPlacement("labels", in.Labels, onSpec); err != nil {
			return err
		}
		for _, l := range in.Labels {
			if !slices.Contains(*labels, l) {
				*labels = append(*labels, l)
			}
		}
		// Only a new type Label is checked against the rest, so Labels
		// a Ticket carried before the rule can still be added to.
		if !onSpec && slices.ContainsFunc(in.Labels, isTypeLabel) {
			return checkOneType("labels", *labels)
		}
		return nil
	})
}

// RemoveLabels is remove_labels(target, labels). A Label not present is
// ignored.
func (c *Core) RemoveLabels(_ context.Context, key string, in LabelsInput) (TargetView, error) {
	if err := checkLabels(in.Labels); err != nil {
		return TargetView{}, err
	}
	return c.updateTarget(key, in.Target, func(_ bool, labels *[]string, _ map[string]string) error {
		*labels = slices.DeleteFunc(*labels, func(l string) bool { return slices.Contains(in.Labels, l) })
		return nil
	})
}

func checkLabels(labels []string) error {
	if len(labels) == 0 {
		return invalid("labels", "name at least one Label")
	}
	for _, l := range labels {
		if strings.TrimSpace(l) == "" || strings.ContainsAny(l, "\r\n") {
			return invalid("labels", "a Label is non-empty and single-line, not %q", l)
		}
	}
	return nil
}

// SetTagsInput is set_tags' input.
type SetTagsInput struct {
	Target Ref               `json:"target"`
	Tags   map[string]string `json:"tags"`
}

// RemoveTagsInput is remove_tags' input.
type RemoveTagsInput struct {
	Target Ref      `json:"target"`
	Keys   []string `json:"keys"`
}

var tagKey = regexp.MustCompile(`^[a-z0-9._-]+$`)

// SetTags is set_tags(target, {k: v}). It merges, overwriting only the
// keys it names.
func (c *Core) SetTags(_ context.Context, key string, in SetTagsInput) (TargetView, error) {
	if len(in.Tags) == 0 {
		return TargetView{}, invalid("tags", "name at least one Tag")
	}
	for k, v := range in.Tags {
		if !tagKey.MatchString(k) {
			return TargetView{}, invalid("tags", "Tag key %q must be lowercase letters, digits, '.', '_' and '-'", k)
		}
		if strings.TrimSpace(v) == "" || strings.ContainsAny(v, "\r\n") {
			return TargetView{}, invalid("tags", "Tag %q needs a non-empty, single-line value", k)
		}
	}
	return c.updateTarget(key, in.Target, func(_ bool, _ *[]string, tags map[string]string) error {
		maps.Copy(tags, in.Tags)
		return nil
	})
}

// RemoveTags is remove_tags(target, keys). A key not present is ignored.
func (c *Core) RemoveTags(_ context.Context, key string, in RemoveTagsInput) (TargetView, error) {
	if len(in.Keys) == 0 {
		return TargetView{}, invalid("keys", "name at least one Tag key")
	}
	for _, k := range in.Keys {
		if !tagKey.MatchString(k) {
			return TargetView{}, invalid("keys", "Tag key %q must be lowercase letters, digits, '.', '_' and '-'", k)
		}
	}
	return c.updateTarget(key, in.Target, func(_ bool, _ *[]string, tags map[string]string) error {
		for _, k := range in.Keys {
			delete(tags, k)
		}
		return nil
	})
}

// updateTarget loads the Spec or Ticket numbered by ref (they share one
// counter) and applies fn to its Labels and Tags. It is written only
// when they changed. Label and Tag changes aren't recorded in edits.
func (c *Core) updateTarget(key string, ref Ref, fn func(onSpec bool, labels *[]string, tags map[string]string) error) (TargetView, error) {
	var v TargetView
	err := c.store.Update(func(w store.Writer) error {
		if err := writable(w, key); err != nil {
			return err
		}
		n, err := ref.number(key, "target")
		if err != nil {
			return err
		}
		apply := func(onSpec bool, labels *[]string, tags *map[string]string) (bool, error) {
			oldL, oldT := slices.Clone(*labels), maps.Clone(*tags)
			if *tags == nil {
				*tags = map[string]string{}
			}
			if err := fn(onSpec, labels, *tags); err != nil {
				return false, err
			}
			return !slices.Equal(oldL, *labels) || !maps.Equal(oldT, *tags), nil
		}
		if s, ok := w.Spec(key, n); ok {
			changed, err := apply(true, &s.Labels, &s.Tags)
			if err != nil {
				return err
			}
			if changed {
				s.Updated = c.now()
				if err := w.PutSpec(key, s); err != nil {
					return err
				}
			}
			v = TargetView{ID: DisplayID(key, n), Kind: "spec", Labels: orEmpty(s.Labels), Tags: orEmptyMap(s.Tags)}
			return nil
		}
		if t, ok := w.Ticket(key, n); ok {
			changed, err := apply(false, &t.Labels, &t.Tags)
			if err != nil {
				return err
			}
			if changed {
				t.Updated = c.now()
				if err := w.PutTicket(key, t); err != nil {
					return err
				}
			}
			v = TargetView{ID: DisplayID(key, n), Kind: "ticket", Labels: orEmpty(t.Labels), Tags: orEmptyMap(t.Tags)}
			return nil
		}
		return notFound("there is no Spec or Ticket %s", DisplayID(key, n))
	})
	return v, err
}

func editViews(es []store.Edit) []EditView {
	out := []EditView{}
	for _, e := range es {
		out = append(out, EditView{Number: e.Number, Actor: e.Actor, Time: e.Time, Previous: e.Previous})
	}
	return out
}

// CommentView is one comment on a Ticket.
type CommentView struct {
	Number int       `json:"number"`
	Actor  string    `json:"actor"`
	Time   time.Time `json:"time"`
	Claim  int       `json:"claim,omitempty"` // the Claim a required note was made under
	Body   string    `json:"body"`
}

func commentViews(cs []store.Comment) []CommentView {
	out := []CommentView{}
	for _, c := range cs {
		out = append(out, CommentView{Number: c.Number, Actor: c.Actor, Time: c.Time, Claim: c.Claim, Body: c.Body})
	}
	return out
}

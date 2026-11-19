package core

import (
	"context"
	"maps"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/tomoram/code-central/api"
	"github.com/tomoram/code-central/domain"
	"github.com/tomoram/code-central/store"
)

// EditTicket is edit_ticket(ticket, title?, description?, full?). An edit
// that changes something records the previous values in edits; one that
// changes nothing writes nothing.
func (c *Core) EditTicket(ctx context.Context, key string, in api.EditTicketInput) (api.Written[api.TicketView], error) {
	if in.Title != nil && strings.TrimSpace(*in.Title) == "" {
		return api.Written[api.TicketView]{}, invalid("title", "a Ticket needs a title")
	}
	return c.updateTicket(key, "edit_ticket", in.Ticket, in.Full, func(t *domain.Ticket) (bool, error) {
		if err := definitionOwned(ctx, DisplayID(key, t.Number), t.Link); err != nil {
			return false, err
		}
		prev := map[string]string{}
		change(prev, "title", &t.Title, in.Title)
		change(prev, "description", &t.Description, in.Description)
		if len(prev) == 0 {
			return false, nil
		}
		t.Edits = append(t.Edits, domain.Edit{Number: len(t.Edits) + 1, Actor: string(api.ActorOf(ctx)), Time: c.now(), Previous: prev})
		return true, nil
	})
}

// ticketEditable is whether the UI offers Edit on t (the glossary's
// Edit): while it is Draft, its Link doesn't own its definition, which
// definitionOwned refuses, and its Project p takes writes, which writable
// refuses. TicketView's editable.
func ticketEditable(p domain.Project, t domain.Ticket) bool {
	return t.State == domain.Draft && !t.Link.Owned() && p.Archived.IsZero()
}

// specEditable is whether the UI offers Edit on s, whose Tickets are all
// finished when specFinished (the glossary's Edit): until it is Finished,
// never on a Map, while its Link doesn't own its definition, which
// definitionOwned refuses, and its Project p takes writes, which writable
// refuses. SpecDetail's editable.
func specEditable(p domain.Project, s domain.Spec, specFinished bool) bool {
	return !specFinished && !isMap(s.Labels) && !s.Link.Owned() && p.Archived.IsZero()
}

// EditSpec is edit_spec(spec, title?, body?, full?), recording edits as
// EditTicket does. The history sits in the Spec's own file.
func (c *Core) EditSpec(ctx context.Context, key string, in api.EditSpecInput) (api.Written[api.SpecView], error) {
	if in.Title != nil && strings.TrimSpace(*in.Title) == "" {
		return api.Written[api.SpecView]{}, invalid("title", "a Spec needs a title")
	}
	var v api.Written[api.SpecView]
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
			s.Edits = append(s.Edits, domain.Edit{Number: len(s.Edits) + 1, Actor: string(api.ActorOf(ctx)), Time: s.Updated, Previous: prev})
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

// Comment is comment(ticket, body). It works on any Ticket, including
// one another agent holds, and needs no Claim ID.
func (c *Core) Comment(ctx context.Context, key string, in api.CommentInput) (api.Written[api.TicketView], error) {
	if strings.TrimSpace(in.Body) == "" {
		return api.Written[api.TicketView]{}, invalid("body", "a comment needs a body")
	}
	return c.updateTicket(key, "comment", in.Ticket, in.Full, func(t *domain.Ticket) (bool, error) {
		addComment(t, api.ActorOf(ctx), c.now(), 0, in.Body)
		return true, nil
	})
}

// Approve is approve(ticket): Draft to Ready.
func (c *Core) Approve(ctx context.Context, key string, in api.TicketInput) (api.Written[api.TicketView], error) {
	return c.updateTicket(key, "approve", in.Ticket, in.Full, func(t *domain.Ticket) (bool, error) {
		return true, transition(ctx, t, key, "approve", "", c.now())
	})
}

// Defer is defer(ticket, reason): Ready to Draft, with the reason stored
// as a comment.
func (c *Core) Defer(ctx context.Context, key string, in api.DeferInput) (api.Written[api.TicketView], error) {
	if strings.TrimSpace(in.Reason) == "" {
		return api.Written[api.TicketView]{}, invalid("reason", "defer needs a reason")
	}
	return c.updateTicket(key, "defer", in.Ticket, in.Full, func(t *domain.Ticket) (bool, error) {
		if err := transition(ctx, t, key, "defer", "", c.now()); err != nil {
			return false, err
		}
		addComment(t, api.ActorOf(ctx), c.now(), 0, in.Reason)
		return true, nil
	})
}

// addComment adds a numbered comment to t. claim is the number of the
// Claim a required note was made under, 0 for none.
func addComment(t *domain.Ticket, a api.Actor, at time.Time, claim int, body string) {
	t.Comments = append(t.Comments, domain.Comment{Number: len(t.Comments) + 1, Actor: string(a), Time: at, Claim: claim, Body: body})
}

// updateTicket loads the Ticket ref names, for operation op, under the
// write lock and applies fn. When fn reports a change, the Ticket's
// updated time is set and it is written. Its result is the write's
// Acknowledgement, or with full the Ticket.
func (c *Core) updateTicket(key, op string, ref api.Ref, full bool, fn func(t *domain.Ticket) (bool, error)) (api.Written[api.TicketView], error) {
	return c.updateTicketIn(key, op, ref, full, func(_ store.Reader, t *domain.Ticket) (bool, error) { return fn(t) })
}

// AddLabels is add_labels(target, labels). A Label already present is
// kept once. Adding the auto-accept Label also accepts, in the same
// write, every Ticket it now covers that is In Review and eligible.
func (c *Core) AddLabels(ctx context.Context, key string, in api.LabelsInput) (api.TargetView, error) {
	if err := checkLabels(in.Labels); err != nil {
		return api.TargetView{}, err
	}
	if err := checkReservedNames("labels", in.Labels); err != nil {
		return api.TargetView{}, err
	}
	if err := checkHumanLabels(ctx, "labels", in.Labels); err != nil {
		return api.TargetView{}, err
	}
	sweep := false
	then := func(r store.Reader, target store.Import) (store.Import, error) {
		if !sweep {
			return target, nil
		}
		return c.sweepAutoAccept(ctx, r, key, target, c.now())
	}
	return c.updateTargetThen(key, in.Target, func(_ store.Reader, _ int, onSpec bool, labels *[]string, _ map[string]string) error {
		if err := checkPlacement("labels", in.Labels, onSpec); err != nil {
			return err
		}
		sweep = slices.Contains(in.Labels, domain.AutoAcceptLabel) && !slices.Contains(*labels, domain.AutoAcceptLabel)
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
	}, then)
}

// RemoveLabels is remove_labels(target, labels). A Label not present is
// ignored.
func (c *Core) RemoveLabels(ctx context.Context, key string, in api.LabelsInput) (api.TargetView, error) {
	if err := checkLabels(in.Labels); err != nil {
		return api.TargetView{}, err
	}
	if err := checkHumanLabels(ctx, "labels", in.Labels); err != nil {
		return api.TargetView{}, err
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

var tagKey = regexp.MustCompile(`^[a-z0-9._-]+$`)

// SetTags is set_tags(target, {k: v}). It merges, overwriting only the
// keys it names. A Spec's branch Tag is checked as checkBranchTagChange
// says.
func (c *Core) SetTags(_ context.Context, key string, in api.SetTagsInput) (api.TargetView, error) {
	if len(in.Tags) == 0 {
		return api.TargetView{}, invalid("tags", "name at least one Tag")
	}
	if err := checkTagSyntax(in.Tags); err != nil {
		return api.TargetView{}, err
	}
	return c.updateTargetIn(key, in.Target, func(r store.Reader, n int, onSpec bool, _ *[]string, tags map[string]string) error {
		if !onSpec {
			if err := checkTicketTags(r, key, in.Tags); err != nil {
				return err
			}
		}
		if name, ok := in.Tags[domain.BranchTag]; ok && onSpec {
			if err := checkBranchTagChange(r, key, "tags", n, tags, name); err != nil {
				return err
			}
		}
		maps.Copy(tags, in.Tags)
		return nil
	})
}

// checkTagSyntax checks each Tag's key and value.
func checkTagSyntax(tags map[string]string) error {
	for k, v := range tags {
		if !tagKey.MatchString(k) {
			return invalid("tags", "Tag key %q must be lowercase letters, digits, '.', '_' and '-'", k)
		}
		if strings.TrimSpace(v) == "" || strings.ContainsAny(v, "\r\n") {
			return invalid("tags", "Tag %q needs a non-empty, single-line value", k)
		}
	}
	return nil
}

// checkTicketTags checks what only a Ticket's Tags must meet: effort sets
// its Model Choice, so it is a valid effort level for the Project.
func checkTicketTags(r store.Reader, key string, tags map[string]string) error {
	if effort, ok := tags[EffortTag]; ok {
		return checkEffort(r, key, "tags", effort)
	}
	return nil
}

// RemoveTags is remove_tags(target, keys). A key not present is ignored.
// Removing a Spec's branch Tag is refused once it is locked, as
// checkBranchTagChange says.
func (c *Core) RemoveTags(_ context.Context, key string, in api.RemoveTagsInput) (api.TargetView, error) {
	if len(in.Keys) == 0 {
		return api.TargetView{}, invalid("keys", "name at least one Tag key")
	}
	for _, k := range in.Keys {
		if !tagKey.MatchString(k) {
			return api.TargetView{}, invalid("keys", "Tag key %q must be lowercase letters, digits, '.', '_' and '-'", k)
		}
	}
	return c.updateTargetIn(key, in.Target, func(r store.Reader, n int, onSpec bool, _ *[]string, tags map[string]string) error {
		if onSpec && slices.Contains(in.Keys, domain.BranchTag) {
			if err := checkBranchTagChange(r, key, "keys", n, tags, ""); err != nil {
				return err
			}
		}
		for _, k := range in.Keys {
			delete(tags, k)
		}
		return nil
	})
}

// updateTarget loads the Spec or Ticket numbered by ref (they share one
// counter) and applies fn to its Labels and Tags. It is written only
// when they changed. Label and Tag changes aren't recorded in edits.
func (c *Core) updateTarget(key string, ref api.Ref, fn func(onSpec bool, labels *[]string, tags map[string]string) error) (api.TargetView, error) {
	return c.updateTargetIn(key, ref, func(_ store.Reader, _ int, onSpec bool, labels *[]string, tags map[string]string) error {
		return fn(onSpec, labels, tags)
	})
}

// targetFn changes the Labels and Tags of the Spec or Ticket numbered n,
// read through r.
type targetFn func(r store.Reader, n int, onSpec bool, labels *[]string, tags map[string]string) error

// updateTargetIn is updateTarget with fn given the store and the
// target's number too.
func (c *Core) updateTargetIn(key string, ref api.Ref, fn targetFn) (api.TargetView, error) {
	return c.updateTargetThen(key, ref, fn, nil)
}

// updateTargetThen is updateTargetIn with then, when not nil, turning
// the changed target, given as an Import that rewrites it alone, into
// the records the write makes, so other records can change with it.
func (c *Core) updateTargetThen(key string, ref api.Ref, fn targetFn,
	then func(r store.Reader, target store.Import) (store.Import, error)) (api.TargetView, error) {
	var v api.TargetView
	err := c.store.Update(func(w store.Writer) error {
		if err := writable(w, key); err != nil {
			return err
		}
		n, err := refNumber(ref, key, "target")
		if err != nil {
			return err
		}
		apply := func(onSpec bool, labels *[]string, tags *map[string]string) (bool, error) {
			oldL, oldT := slices.Clone(*labels), maps.Clone(*tags)
			if *tags == nil {
				*tags = map[string]string{}
			}
			if err := fn(w, n, onSpec, labels, *tags); err != nil {
				return false, err
			}
			return !slices.Equal(oldL, *labels) || !maps.Equal(oldT, *tags), nil
		}
		put := func(target store.Import) error {
			if then != nil {
				var err error
				if target, err = then(w, target); err != nil {
					return err
				}
			}
			return putRecords(w, key, target)
		}
		if s, ok := w.Spec(key, n); ok {
			changed, err := apply(true, &s.Labels, &s.Tags)
			if err != nil {
				return err
			}
			if changed {
				s.Updated = c.now()
				if err := put(store.Import{RewrittenSpecs: []domain.Spec{s}}); err != nil {
					return err
				}
			}
			v = api.TargetView{ID: DisplayID(key, n), Kind: api.RecordSpec, Labels: orEmpty(s.Labels), Tags: orEmptyMap(s.Tags)}
			return nil
		}
		// An Import rewrites the Ticket whole, so it is read whole.
		if t, ok := w.Ticket(key, n, whole...); ok {
			changed, err := apply(false, &t.Labels, &t.Tags)
			if err != nil {
				return err
			}
			if changed {
				t.Updated = c.now()
				if err := put(store.Import{Rewritten: []domain.Ticket{t}}); err != nil {
					return err
				}
			}
			v = api.TargetView{ID: DisplayID(key, n), Kind: api.RecordTicket, Labels: orEmpty(t.Labels), Tags: orEmptyMap(t.Tags)}
			return nil
		}
		return notFound("there is no Spec or Ticket %s", DisplayID(key, n))
	})
	return v, err
}

func editViews(es []domain.Edit) []api.EditView {
	out := []api.EditView{}
	for _, e := range es {
		out = append(out, api.EditView{Number: e.Number, Actor: e.Actor, Time: e.Time, Previous: e.Previous})
	}
	return out
}

func commentViews(cs []domain.Comment) []api.CommentView {
	out := []api.CommentView{}
	for _, c := range cs {
		out = append(out, api.CommentView{Number: c.Number, Actor: c.Actor, Time: c.Time, Claim: c.Claim, Body: c.Body})
	}
	return out
}

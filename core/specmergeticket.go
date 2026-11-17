package core

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/tomoram/code-central/store"
)

// Spec merge Tickets (code-central-542): when a Spec Branch conflicts
// with the base, the Factory asks Code Central to open the Ticket that
// merges the base into it, which Code Central marks as a Spec merge
// Ticket. Only open_spec_merge sets the mark.

// OpenSpecMergeInput is the Factory's open_spec_merge(spec, conflicts,
// base_tip, spec_branch_tip, repositories?): the files the Spec Branch
// conflicts with the base in, and both branches' tips. Those are the
// Home Repository's; repositories gives each conflicting Repository's
// the same way, the Home Repository's too when it isn't given above
// (Spec code-central-848).
type OpenSpecMergeInput struct {
	Spec          Ref                 `json:"spec"`
	Conflicts     []string            `json:"conflicts"`
	BaseTip       string              `json:"base_tip"`
	SpecBranchTip string              `json:"spec_branch_tip"`
	Repositories  []SpecMergeConflict `json:"repositories,omitempty"`
}

// SpecMergeConflict is how the Spec Branch conflicts with the base in the
// Repository Name: the files, and both branches' tips there.
type SpecMergeConflict struct {
	Name          string   `json:"name"`
	Conflicts     []string `json:"conflicts"`
	BaseTip       string   `json:"base_tip"`
	SpecBranchTip string   `json:"spec_branch_tip"`
}

// SpecMergeOutcome says what open_spec_merge did.
type SpecMergeOutcome string

// The open_spec_merge outcomes.
const (
	// SpecMergeOpened is a new Ready Spec merge Ticket.
	SpecMergeOpened SpecMergeOutcome = "opened"
	// SpecMergeCommented is the conflict commented on the open one.
	SpecMergeCommented SpecMergeOutcome = "commented"
	// SpecMergeUnchanged is nothing written: the open one already says it.
	SpecMergeUnchanged SpecMergeOutcome = "unchanged"
)

// SpecMergeOutcomes are the open_spec_merge outcomes.
var SpecMergeOutcomes = []SpecMergeOutcome{SpecMergeOpened, SpecMergeCommented, SpecMergeUnchanged}

// OpenSpecMergeResult is open_spec_merge's result: the Spec merge Ticket
// and what was done to it.
type OpenSpecMergeResult struct {
	Ticket  string           `json:"ticket"`
	Outcome SpecMergeOutcome `json:"outcome"`
}

// SpecMergeTitle is the title of the Ticket that merges the base into the
// Spec Branch of spec, a display ID.
func SpecMergeTitle(spec string) string {
	return fmt.Sprintf("Merge the base into the Spec Branch of %s", spec)
}

// OpenSpecMerge is the Factory's open_spec_merge. It comments the
// conflict on the Spec's open Spec merge Ticket, or failing that on an
// open unmarked Ticket with its title (one opened before the mark
// existed), which it leaves unmarked; it writes nothing when that Ticket
// already says the same. With neither, it opens a Ready marked Ticket on
// the Spec. It refuses a Long-lived Spec, a Map and a Finished Spec.
func (c *Core) OpenSpecMerge(ctx context.Context, key string, in OpenSpecMergeInput) (OpenSpecMergeResult, error) {
	// The Home Repository's conflict above may be left out, given in
	// repositories instead.
	if len(in.Repositories) == 0 || len(in.Conflicts) > 0 || in.BaseTip != "" || in.SpecBranchTip != "" {
		if err := checkSpecMergeConflict("", in.Conflicts, in.BaseTip, in.SpecBranchTip); err != nil {
			return OpenSpecMergeResult{}, err
		}
	}
	var res OpenSpecMergeResult
	err := c.store.Update(func(w store.Writer) error {
		if err := writable(w, key); err != nil {
			return err
		}
		p, _ := w.Project(key)
		sp, err := resolveSpec(w, key, "open_spec_merge", "spec", in.Spec)
		if err != nil {
			return err
		}
		id := DisplayID(key, sp.Number)
		tickets := specTickets(w, key, sp.Number)
		if why := whyNoSpecMerge(sp, tickets); why != "" {
			return &Error{Code: ActionNotAllowed, Message: fmt.Sprintf("%s %s, so it takes no Spec merge Ticket", id, why)}
		}
		title, body := SpecMergeTitle(id), specMergeBody(id, p.Base(), in)
		if len(in.Repositories) > 0 {
			cs, err := specMergeConflicts(p, in)
			if err != nil {
				return err
			}
			body = specMergeBodyIn(p, id, cs)
		}
		now := c.now()
		if t, ok := openSpecMergeTicket(tickets, title); ok {
			res.Ticket = DisplayID(key, t.Number)
			if t.Description == body || slices.ContainsFunc(t.Comments, func(cm store.Comment) bool { return cm.Body == body }) {
				res.Outcome = SpecMergeUnchanged
				return nil
			}
			addComment(&t, ActorOf(ctx), now, 0, body)
			t.Updated = now
			res.Outcome = SpecMergeCommented
			return w.PutTicket(key, t)
		}
		t := store.Ticket{
			Number: w.NextNumber(key), Title: title, State: store.Ready, Spec: sp.Number,
			SpecMergeTicket: true, Created: now, Updated: now, Description: body,
		}
		res = OpenSpecMergeResult{Ticket: DisplayID(key, t.Number), Outcome: SpecMergeOpened}
		return w.PutTicket(key, t)
	})
	return res, err
}

// whyNoSpecMerge is why spec, with its tickets, takes no Spec merge
// Ticket, or "" when it does.
func whyNoSpecMerge(spec store.Spec, tickets []store.Ticket) string {
	switch {
	case slices.Contains(spec.Labels, LongLivedLabel):
		return "is Long-lived: its Tickets land one by one, with no Spec Branch"
	case isMap(spec.Labels):
		return "is a Map, with no Spec Branch"
	}
	if _, done := summarise(tickets); done {
		return "is Finished: every Ticket on it is Done or Cancelled"
	}
	return ""
}

// openSpecMergeTicket is the open Ticket among a Spec's tickets that
// takes its Spec merge comments: the first open marked one, else the
// first open unmarked one titled title.
func openSpecMergeTicket(tickets []store.Ticket, title string) (store.Ticket, bool) {
	var legacy *store.Ticket
	for i, t := range tickets {
		if finished(t.State) {
			continue
		}
		if t.SpecMergeTicket {
			return t, true
		}
		if legacy == nil && t.Title == title {
			legacy = &tickets[i]
		}
	}
	if legacy != nil {
		return *legacy, true
	}
	return store.Ticket{}, false
}

// specMergeBody is a Spec merge Ticket's description, and each comment it
// takes: the conflicting files and both tips.
func specMergeBody(spec, base string, in OpenSpecMergeInput) string {
	var b strings.Builder
	fmt.Fprintf(&b, "The Spec Branch spec/%s conflicts with the base, %s, in these files:\n\n", spec, base)
	for _, f := range in.Conflicts {
		fmt.Fprintf(&b, "- %s\n", f)
	}
	fmt.Fprintf(&b, "\nThe base's tip is %s and the Spec Branch's tip is %s. Merge the base into this branch and resolve the conflicts.\n", in.BaseTip, in.SpecBranchTip)
	return b.String()
}

// checkSpecMergeConflict refuses a conflict without its files or either
// tip, naming the field within at (repositories[i]), or at the top
// level for "".
func checkSpecMergeConflict(at string, conflicts []string, baseTip, specTip string) error {
	field := func(name string) string {
		if at == "" {
			return name
		}
		return at + "." + name
	}
	switch {
	case len(conflicts) == 0:
		return invalid(field("conflicts"), "open_spec_merge needs the files the Spec Branch conflicts with the base in")
	case strings.TrimSpace(baseTip) == "":
		return invalid(field("base_tip"), "open_spec_merge needs the base's tip")
	case strings.TrimSpace(specTip) == "":
		return invalid(field("spec_branch_tip"), "open_spec_merge needs the Spec Branch's tip")
	}
	return nil
}

// specMergeConflicts is every conflict in gives for Project p, in p's
// landing order: the Home Repository's in either form, and each other
// Repository's. It refuses a name that isn't one of p's Repositories, a
// name given twice, an entry without its files or either tip, and the
// Home Repository given in both forms.
func specMergeConflicts(p store.Project, in OpenSpecMergeInput) ([]SpecMergeConflict, error) {
	var out []SpecMergeConflict
	if len(in.Conflicts) > 0 {
		out = append(out, SpecMergeConflict{Name: p.Key, Conflicts: in.Conflicts, BaseTip: in.BaseTip, SpecBranchTip: in.SpecBranchTip})
	}
	names := repositoryNames(p.RepositoryList())
	for i, c := range in.Repositories {
		at := fmt.Sprintf("repositories[%d]", i)
		c.Name = strings.TrimSpace(c.Name)
		switch {
		case !slices.Contains(names, c.Name):
			return nil, notARepository(p, at+".name", c.Name)
		case c.Name == p.Key && len(in.Conflicts) > 0:
			return nil, invalid("repositories", "%s is the Home Repository, given both in conflicts and in repositories: give it in one form or the other", p.Key)
		case slices.ContainsFunc(out, func(o SpecMergeConflict) bool { return o.Name == c.Name }):
			return nil, invalid(at+".name", "%s is listed twice: give each Repository's conflict once", c.Name)
		}
		if err := checkSpecMergeConflict(at, c.Conflicts, c.BaseTip, c.SpecBranchTip); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	inLandingOrder(p, out, func(c SpecMergeConflict) string { return c.Name })
	return out, nil
}

// specMergeBodyIn is specMergeBody for a Spec Branch conflicting in each
// of cs's Repositories, p's: one Spec merge Ticket names each, with its
// base, both tips and its files (ADR 0049).
func specMergeBodyIn(p store.Project, spec string, cs []SpecMergeConflict) string {
	var b strings.Builder
	fmt.Fprintf(&b, "The Spec Branch spec/%s conflicts with the base in these Repositories:\n\n", spec)
	for _, c := range cs {
		r, _ := repositoryField(p, c.Name)
		fmt.Fprintf(&b, "- %s: merging spec/%s at %s into %s at %s conflicts in these files:\n", c.Name, spec, c.SpecBranchTip, baseOfRepository(p, r), c.BaseTip)
		for _, f := range c.Conflicts {
			fmt.Fprintf(&b, "  - %s\n", f)
		}
	}
	b.WriteString("\nIn each, merge the base into this branch and resolve the conflicts.\n")
	return b.String()
}

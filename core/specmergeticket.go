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

// Spec merge Tickets (code-central-542): when a Spec Branch conflicts
// with the base, the Factory asks Code Central to open the Ticket that
// merges the base into it, which Code Central marks as a Spec merge
// Ticket. Only open_spec_merge sets the mark.

// specMergeTitle is the Spec merge Ticket's title for spec, a display
// ID, under the policies of its conflicting Repositories: the rebase
// title with Rebase in force in any, else the merge title.
func specMergeTitle(spec string, rebase bool) string {
	if rebase {
		return api.SpecMergeRebaseTitle(spec)
	}
	return api.SpecMergeTitle(spec)
}

// OpenSpecMerge is the Factory's open_spec_merge. It comments the
// conflict on the Spec's open Spec merge Ticket, or failing that on an
// open unmarked Ticket with its title (one opened before the mark
// existed), which it leaves unmarked; it writes nothing when that Ticket
// already says the same. With neither, it opens a Ready marked Ticket on
// the Spec. It refuses a Long-lived Spec, a Map and a Finished Spec.
func (c *Core) OpenSpecMerge(ctx context.Context, key string, in api.OpenSpecMergeInput) (api.OpenSpecMergeResult, error) {
	// The Home Repository's conflict above may be left out, given in
	// repositories instead.
	if len(in.Repositories) == 0 || specMergeHomeGiven(in) {
		if err := checkSpecMergeConflict("", in.Conflicts, in.BaseTip, in.SpecBranchTip); err != nil {
			return api.OpenSpecMergeResult{}, err
		}
	}
	var res api.OpenSpecMergeResult
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
			return &api.Error{Code: api.ActionNotAllowed, Message: fmt.Sprintf("%s %s, so it takes no Spec merge Ticket", id, why)}
		}
		// The title and body follow the Landing Policy of the conflicting
		// Repositories when the Ticket is opened (ADR 0067): to rebase
		// with Rebase in force in any, else to merge.
		var rebase bool
		var body string
		branch := SpecBranch(id, sp.Tags)
		if len(in.Repositories) == 0 {
			rebase = landingPolicyIn(p, p.Key).RebaseInForce()
			body = specMergeBody(branch, p.Base(), in, rebase)
		} else {
			cs, err := specMergeConflicts(p, in)
			if err != nil {
				return err
			}
			rebase = slices.ContainsFunc(cs, func(c api.SpecMergeConflict) bool { return landingPolicyIn(p, c.Name).RebaseInForce() })
			body = specMergeBodyPerRepository(p, branch, cs, rebase)
		}
		title := specMergeTitle(id, rebase)
		now := c.now()
		if open, ok := openSpecMergeTicket(tickets, id); ok {
			h := held{store.Comments}
			t, _ := w.Ticket(key, open.Number, h...)
			res.Ticket = DisplayID(key, t.Number)
			if t.Description == body || slices.ContainsFunc(t.Comments, func(cm domain.Comment) bool { return cm.Body == body }) {
				res.Outcome = api.SpecMergeUnchanged
				return nil
			}
			addComment(&t, api.ActorOf(ctx), now, 0, body)
			t.Updated = now
			res.Outcome = api.SpecMergeCommented
			return h.put(w, key, &t)
		}
		t := domain.Ticket{
			Number: w.NextNumber(key), Title: title, State: domain.Ready, Spec: sp.Number,
			SpecMergeTicket: true, Created: now, Updated: now, Description: body,
		}
		res = api.OpenSpecMergeResult{Ticket: DisplayID(key, t.Number), Outcome: api.SpecMergeOpened}
		return whole.put(w, key, &t)
	})
	return res, err
}

// whyNoSpecMerge is why spec, with its tickets, takes no Spec merge
// Ticket, or "" when it does.
func whyNoSpecMerge(spec domain.Spec, tickets []domain.Ticket) string {
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
// first open unmarked one with either Spec merge title for spec, a
// display ID.
func openSpecMergeTicket(tickets []domain.Ticket, spec string) (domain.Ticket, bool) {
	var legacy *domain.Ticket
	for i, t := range tickets {
		if finished(t.State) {
			continue
		}
		if t.SpecMergeTicket {
			return t, true
		}
		if legacy == nil && api.IsSpecMergeTitle(spec, t.Title) {
			legacy = &tickets[i]
		}
	}
	if legacy != nil {
		return *legacy, true
	}
	return domain.Ticket{}, false
}

// specMergeBody is a Spec merge Ticket's description, and each comment it
// takes, for the Spec Branch named branch: the conflicting files and both
// tips, then what to do, which is to rebase this branch onto the base
// with rebase, else to merge the base into it.
func specMergeBody(branch, base string, in api.OpenSpecMergeInput, rebase bool) string {
	var b strings.Builder
	fmt.Fprintf(&b, "The Spec Branch %s conflicts with the base, %s, in these files:\n\n", branch, base)
	for _, f := range in.Conflicts {
		fmt.Fprintf(&b, "- %s\n", f)
	}
	do := "Merge the base into this branch"
	if rebase {
		do = "Rebase this branch onto the base"
	}
	fmt.Fprintf(&b, "\nThe base's tip is %s and the Spec Branch's tip is %s. %s and resolve the conflicts.\n", in.BaseTip, in.SpecBranchTip, do)
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
func specMergeConflicts(p domain.Project, in api.OpenSpecMergeInput) ([]api.SpecMergeConflict, error) {
	var out []api.SpecMergeConflict
	if specMergeHomeGiven(in) {
		out = append(out, api.SpecMergeConflict{Name: p.Key, Conflicts: in.Conflicts, BaseTip: in.BaseTip, SpecBranchTip: in.SpecBranchTip})
	}
	names := repositoryNames(p.RepositoryList())
	for i, c := range in.Repositories {
		at := fmt.Sprintf("repositories[%d]", i)
		c.Name = strings.TrimSpace(c.Name)
		switch {
		case !slices.Contains(names, c.Name):
			return nil, notARepository(p, at+".name", c.Name)
		case c.Name == p.Key && specMergeHomeGiven(in):
			return nil, invalid("repositories", "%s is the Home Repository, given both in conflicts and in repositories: give it in one form or the other", p.Key)
		case slices.ContainsFunc(out, func(o api.SpecMergeConflict) bool { return o.Name == c.Name }):
			return nil, invalid(at+".name", "%s is listed twice: give each Repository's conflict once", c.Name)
		}
		if err := checkSpecMergeConflict(at, c.Conflicts, c.BaseTip, c.SpecBranchTip); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	inLandingOrder(p, out, func(c api.SpecMergeConflict) string { return c.Name })
	return out, nil
}

// specMergeHomeGiven reports whether in gives the Home Repository's conflict in
// its top-level fields, rather than in repositories.
func specMergeHomeGiven(in api.OpenSpecMergeInput) bool {
	return len(in.Conflicts) > 0 || in.BaseTip != "" || in.SpecBranchTip != ""
}

// specMergeBodyPerRepository is specMergeBody for the Spec Branch named
// branch conflicting in each of cs's Repositories, p's: one Spec merge
// Ticket names each, with its base, both tips and its files (ADR 0049).
// Each of cs names one of p's Repositories, as specMergeConflicts
// checked. With rebase it says to rebase this branch onto its base in
// each, else to merge each base into it.
func specMergeBodyPerRepository(p domain.Project, branch string, cs []api.SpecMergeConflict, rebase bool) string {
	var b strings.Builder
	fmt.Fprintf(&b, "The Spec Branch %s conflicts with the base in these Repositories:\n\n", branch)
	for _, c := range cs {
		r, _ := repositoryField(p, c.Name)
		fmt.Fprintf(&b, "- %s: merging %s at %s into %s at %s conflicts in these files:\n", c.Name, branch, c.SpecBranchTip, baseOfRepository(p, r), c.BaseTip)
		for _, f := range c.Conflicts {
			fmt.Fprintf(&b, "  - %s\n", f)
		}
	}
	if rebase {
		b.WriteString("\nIn each, rebase this branch onto its base and resolve the conflicts.\n")
	} else {
		b.WriteString("\nIn each, merge the base into this branch and resolve the conflicts.\n")
	}
	return b.String()
}

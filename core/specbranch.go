package core

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/tomoram/code-central/api"
	"github.com/tomoram/code-central/domain"
	"github.com/tomoram/code-central/store"
)

// BranchTag is the reserved Spec Tag whose value names the Spec's Spec
// Branch in place of spec/<spec id> (Spec code-central-1089). On a
// Ticket it means nothing and isn't checked.
const BranchTag = "branch"

// specBranchPrefix starts every default Spec Branch name, so no branch
// Tag may start with it.
const specBranchPrefix = "spec/"

// SpecBranch is the Spec Branch of the Spec id, a display ID, carrying
// tags: its branch Tag's value when it has one, else spec/<id>. It is
// the one place the name is made.
func SpecBranch(id string, tags map[string]string) string {
	if name := tags[BranchTag]; name != "" {
		return name
	}
	return specBranchPrefix + id
}

// checkBranchTagName checks name as a branch Tag in Project p, all but
// its uniqueness: a valid branch name that isn't a default Spec Branch
// name, any Repository's Base or a Ticket branch's name. field names the
// input refused.
func checkBranchTagName(p domain.Project, field, name string) error {
	if err := checkBranchName(name); err != nil {
		return invalid(field, "a branch Tag names the Spec Branch, so it must be a valid branch name: %s", err)
	}
	// In any case, since git's refs are files on a case-insensitive disk.
	if strings.HasPrefix(strings.ToLower(name), specBranchPrefix) {
		return invalid(field, "branch %q starts with %s, which is kept for the Spec Branches of Specs with no branch Tag", name, specBranchPrefix)
	}
	for _, r := range p.RepositoryList() {
		if strings.EqualFold(baseOfRepository(p, r), name) {
			return invalid(field, "branch %q is the Base of the Repository %s, so it can't name a Spec Branch", name, r.Name)
		}
	}
	if looksLikeTicketID(p.Key, name) {
		return invalid(field, "branch %q has the shape of a Ticket ID, %s-<n>, which names Ticket branches", name, p.Key)
	}
	return nil
}

// looksLikeTicketID reports whether name has the shape of a Ticket ID of
// Project key, in any case.
func looksLikeTicketID(key, name string) bool {
	return regexp.MustCompile(`(?i)^` + regexp.QuoteMeta(key) + `-[0-9]+$`).MatchString(name)
}

// chooseAnotherName ends checkBranchTagFree's refusal of a name the
// human gave.
const chooseAnotherName = "or choose another name"

// checkBranchTagFree refuses name as Spec n's branch Tag when another
// Spec of Project key that isn't Finished already has it, saying to
// change that Spec's Tag first, then orElse, such as "or choose another
// name". n is 0 for a Spec not yet made.
func checkBranchTagFree(r store.Reader, key, field string, n int, name, orElse string) error {
	for _, s := range r.FindSpecs(key, store.SpecFilter{Tag: &store.TagFilter{Key: BranchTag, Values: []string{name}}}) {
		if s.Number == n {
			continue
		}
		if _, done := summarise(specTickets(r, key, s.Number)); done {
			continue
		}
		id := DisplayID(key, s.Number)
		return invalid(field, "branch %q is already the Spec Branch of %s, %q, which isn't Finished: change or remove the branch Tag on %s first, %s",
			name, id, s.Title, id, orElse)
	}
	return nil
}

// checkBranchTagChange checks changing Spec n's branch Tag, in tags, to
// name, or removing it for "": it is refused once any implementation
// Ticket of the Spec has ever been claimed, as the Spec Branch exists by
// then under its name, and a new name passes checkBranchTagName and
// checkBranchTagFree. Setting the value it has is no change, so isn't
// refused. It runs inside the store update that writes the Tag, so two
// writes can't both take one name.
func checkBranchTagChange(r store.Reader, key, field string, n int, tags map[string]string, name string) error {
	if tags[BranchTag] == name {
		return nil
	}
	if err := checkBranchTagLock(r, key, n, tags); err != nil {
		return err
	}
	if name == "" {
		return nil
	}
	p, _ := r.Project(key)
	if err := checkBranchTagName(p, field, name); err != nil {
		return err
	}
	return checkBranchTagFree(r, key, field, n, name, chooseAnotherName)
}

// checkBranchTagLock refuses any change to Spec n's branch Tag, in tags,
// once one of its implementation Tickets (any but a Stage Ticket) has
// ever been claimed.
func checkBranchTagLock(r store.Reader, key string, n int, tags map[string]string) error {
	for _, t := range specTickets(r, key, n, store.Claims) {
		if StageOf(t.Labels) == "" && len(t.Claims) > 0 {
			id := DisplayID(key, n)
			return &api.Error{Code: api.ActionNotAllowed, Message: fmt.Sprintf(
				"the Spec Branch of %s already exists as %s, since %s has been claimed, so its branch Tag can't change or be removed",
				id, SpecBranch(id, tags), DisplayID(key, t.Number))}
		}
	}
	return nil
}

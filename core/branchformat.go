package core

import (
	"errors"
	"regexp"
	"strings"

	"github.com/tomoram/code-central/api"
	"github.com/tomoram/code-central/domain"
	"github.com/tomoram/code-central/store"
)

// An Integration's Branch format (Spec code-central-1089) names the Spec
// Branch of a Requirement To Requirement makes of one of its items, with
// {id} for the item's ID, such as feat/{id}. Empty, it names none.

// branchFormatID is the placeholder a Branch format fills with an item's
// ID.
const branchFormatID = "{id}"

// sampleItemID fills a Branch format to check it when it is saved.
const sampleItemID = "4711"

// branchFormatPlaceholder matches any {...} placeholder in a Branch
// format.
var branchFormatPlaceholder = regexp.MustCompile(`\{[^{}]*\}`)

// fillBranchFormat is format filled in with the ID of the item linkID
// names: what follows its last #, as in acme/web#12 or contoso#4711.
func fillBranchFormat(format, linkID string) string {
	return strings.ReplaceAll(format, branchFormatID, linkID[strings.LastIndex(linkID, "#")+1:])
}

// checkBranchFormat checks format as a Branch format in Project p, field
// naming the input: empty, or containing {id}, no other placeholder, and
// filled with a sample ID naming a branch a branch Tag could have, apart
// from uniqueness.
func checkBranchFormat(p domain.Project, field, format string) error {
	if format == "" {
		return nil
	}
	if !strings.Contains(format, branchFormatID) {
		return api.InvalidField(field, "a Branch format must contain %s, which To Requirement fills with the item's ID, such as feat/%s", branchFormatID, branchFormatID)
	}
	for _, m := range branchFormatPlaceholder.FindAllString(format, -1) {
		if m != branchFormatID {
			return api.InvalidField(field, "a Branch format can only fill in %s, not %s", branchFormatID, m)
		}
	}
	if strings.ContainsAny(strings.ReplaceAll(format, branchFormatID, ""), "{}") {
		return api.InvalidField(field, "a Branch format can only use { and } around %s", branchFormatID)
	}
	return checkBranchTagName(p, field, fillBranchFormat(format, sampleItemID))
}

// requirementBranch is the branch Tag To Requirement gives the
// Requirement it makes of t, an Incoming Issue, in key: its Integration's
// Branch format filled with its item's ID, or "" for none. It is refused
// when that name can't be the Requirement's branch Tag.
func requirementBranch(r store.Reader, key string, t domain.Ticket) (string, error) {
	format := inboxOf(r, key, t.Link.Integration).branchFormat
	if format == "" {
		return "", nil
	}
	name := fillBranchFormat(format, t.Link.ID)
	p, _ := r.Project(key)
	if err := checkBranchTagName(p, "ticket", name); err != nil {
		return "", toRequirementBranchRefused(err, "change the Branch format")
	}
	if err := checkBranchTagFree(r, key, "ticket", 0, name, "or change the Branch format"); err != nil {
		return "", toRequirementBranchRefused(err, "")
	}
	return name, nil
}

// toRequirementBranchRefused is err, a refused branch Tag, as To
// Requirement's refusal, with fix, if any, added.
func toRequirementBranchRefused(err error, fix string) error {
	var e *api.Error
	if !errors.As(err, &e) {
		return err
	}
	msg := "To Requirement would give the Requirement the branch Tag its Branch format names, but " + e.Message
	if fix != "" {
		msg += ": " + fix
	}
	return &api.Error{Code: api.ActionNotAllowed, Message: msg}
}

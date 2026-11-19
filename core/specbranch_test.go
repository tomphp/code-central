package core_test

import (
	"net/url"
	"slices"
	"strings"
	"testing"

	"github.com/tomoram/code-central/api"
	"github.com/tomoram/code-central/core"
	"github.com/tomoram/code-central/domain"
)

// The branch Tag (Spec code-central-1089), through core's public
// operations.

// branchWorld is acme with a Requirement Spec acme-1, its grilling Stage
// Ticket acme-2 and implementation Ticket acme-3, both Ready, and a plain
// Spec acme-4 with a Ready Ticket acme-5.
func branchWorld(t *testing.T) fixture {
	t.Helper()
	f := newFixture(t)
	f.project(t, "acme")
	f.spec(t, "acme", "Offline mode")
	f.ticket(t, "acme", api.CreateTicketInput{Spec: "acme-1", Title: "grilling", State: domain.Ready, Labels: []string{core.StageLabel(domain.StageGrilling)}})
	f.ticket(t, "acme", api.CreateTicketInput{Spec: "acme-1", Title: "First", State: domain.Ready})
	f.spec(t, "acme", "Dark mode")
	f.ticket(t, "acme", api.CreateTicketInput{Spec: "acme-4", Title: "Toggle", State: domain.Ready})
	return f
}

func (f fixture) setBranch(target, name string) error {
	_, err := f.core.SetTags(ctx, "acme", api.SetTagsInput{Target: api.Ref(target), Tags: map[string]string{core.BranchTag: name}})
	return err
}

func (f fixture) removeBranch(target string) error {
	_, err := f.core.RemoveTags(ctx, "acme", api.RemoveTagsInput{Target: api.Ref(target), Keys: []string{core.BranchTag}})
	return err
}

func wantSays(t *testing.T, err error, parts ...string) {
	t.Helper()
	for _, p := range parts {
		if err == nil || !strings.Contains(err.Error(), p) {
			t.Errorf("err = %v, want it to say %q", err, p)
		}
	}
}

func TestATicketsSpecRefNamesItsSpecBranchByTheBranchTagElseSpecSlashID(t *testing.T) {
	f := branchWorld(t)
	if got := f.getTicket(t, "acme-3").Spec.Branch; got != "spec/acme-1" {
		t.Errorf("with no branch Tag, Spec Branch = %q, want spec/acme-1", got)
	}
	if err := f.setBranch("acme-1", "feat/4711"); err != nil {
		t.Fatal(err)
	}
	if got := f.getTicket(t, "acme-3").Spec.Branch; got != "feat/4711" {
		t.Errorf("with a branch Tag, Spec Branch = %q, want feat/4711", got)
	}
	if got := f.getTicket(t, "acme-5").Spec.Branch; got != "spec/acme-4" {
		t.Errorf("another Spec's Ticket names %q, want spec/acme-4", got)
	}
}

func TestABranchTagThatCantNameASpecBranchIsRefused(t *testing.T) {
	f := branchWorld(t)
	f.addRepository(t, api.AddRepositoryInput{Name: "api", CloneURL: "git@github.com:acme/api.git", Base: "develop"})
	for name, says := range map[string]string{
		"feat 4711":   "valid branch name",
		"feat..4711":  "valid branch name",
		"spec/4711":   "starts with spec/",
		"Spec/4711":   "starts with spec/",
		"main":        "Base of the Repository acme",
		"develop":     "Base of the Repository api",
		"Develop":     "Base of the Repository api",
		"acme-12":     "shape of a Ticket ID",
		"ACME-3":      "shape of a Ticket ID",
		"feat/acme-1": "",
	} {
		err := f.setBranch("acme-1", name)
		if says == "" {
			if err != nil {
				t.Errorf("%q refused: %v", name, err)
			}
			continue
		}
		wantCode(t, err, api.Invalid, "tags")
		wantSays(t, err, says)
	}
	if sp := f.specRead(t, "acme-1"); sp.Tags[core.BranchTag] != "feat/acme-1" {
		t.Errorf("tags %v, want only the good name kept", sp.Tags)
	}
}

func TestABranchTagAnotherUnfinishedSpecHasIsRefusedNamingThatSpec(t *testing.T) {
	f := branchWorld(t)
	if err := f.setBranch("acme-1", "feat/4711"); err != nil {
		t.Fatal(err)
	}

	err := f.setBranch("acme-4", "feat/4711")
	wantCode(t, err, api.Invalid, "tags")
	wantSays(t, err, "acme-1", "Offline mode")

	// Once acme-1 is Finished, its name is free again.
	f.moveTo(t, 2, domain.Done)
	f.moveTo(t, 3, domain.Done)
	if err := f.setBranch("acme-4", "feat/4711"); err != nil {
		t.Errorf("a Finished Spec's branch Tag blocked reuse: %v", err)
	}
}

func TestATicketsBranchTagIsNotChecked(t *testing.T) {
	f := branchWorld(t)
	if err := f.setBranch("acme-3", "spec/anything"); err != nil {
		t.Errorf("a Ticket's branch Tag was checked: %v", err)
	}
}

func TestTheBranchTagIsLockedOnceAnImplementationTicketHasBeenClaimed(t *testing.T) {
	f := branchWorld(t)
	f.claim(t, api.ClaimInput{Ticket: "acme-2"}) // the grilling Stage
	if err := f.setBranch("acme-1", "feat/4711"); err != nil {
		t.Fatalf("a claimed Stage Ticket locked the Tag: %v", err)
	}
	f.moveTo(t, 2, domain.Done)
	f.claim(t, api.ClaimInput{Ticket: "acme-3"})

	err := f.setBranch("acme-1", "feat/4712")
	wantCode(t, err, api.ActionNotAllowed, "")
	wantSays(t, err, "already exists as feat/4711")
	err = f.removeBranch("acme-1")
	wantCode(t, err, api.ActionNotAllowed, "")
	wantSays(t, err, "already exists as feat/4711")
	if err := f.setBranch("acme-1", "feat/4711"); err != nil {
		t.Errorf("setting the same value was refused: %v", err)
	}
	if got := f.getTicket(t, "acme-3").Spec.Branch; got != "feat/4711" {
		t.Errorf("Spec Branch = %q, want feat/4711 kept", got)
	}
}

func TestATagOnASpecClaimedWithoutOneIsLockedToo(t *testing.T) {
	f := branchWorld(t)
	f.claim(t, api.ClaimInput{Ticket: "acme-5"})

	err := f.setBranch("acme-4", "feat/4711")
	wantCode(t, err, api.ActionNotAllowed, "")
	wantSays(t, err, "already exists as spec/acme-4")
	if err := f.removeBranch("acme-4"); err != nil {
		t.Errorf("removing a branch Tag the Spec hasn't got was refused: %v", err)
	}
}

func TestFindSpecsWithAnEmptyTagValueFindsEverySpecWithThatTag(t *testing.T) {
	f := branchWorld(t)
	if err := f.setBranch("acme-4", "feat/4711"); err != nil {
		t.Fatal(err)
	}
	var in api.FindSpecsInput
	if err := in.DecodeQuery(url.Values{"tag": {"branch="}}); err != nil {
		t.Fatal(err)
	}
	rows, err := f.core.FindSpecs(ctx, "acme", in)
	if err != nil {
		t.Fatal(err)
	}
	var ids []string
	for _, r := range rows {
		ids = append(ids, r.ID)
	}
	if !slices.Equal(ids, []string{"acme-4"}) {
		t.Errorf("tag=branch= found %v, want acme-4 alone", ids)
	}
}

func TestASpecMergeTicketNamesTheTagsSpecBranch(t *testing.T) {
	f := branchWorld(t)
	if err := f.setBranch("acme-4", "feat/4711"); err != nil {
		t.Fatal(err)
	}
	res, err := f.core.OpenSpecMerge(ctx, "acme", api.OpenSpecMergeInput{Spec: "acme-4", Conflicts: []string{"README"}, BaseTip: "aaa111", SpecBranchTip: "bbb222"})
	if err != nil {
		t.Fatal(err)
	}
	if d := f.getTicket(t, res.Ticket).Description; !strings.HasPrefix(d, "The Spec Branch feat/4711 conflicts with the base") {
		t.Errorf("description %q, want it to name feat/4711", d)
	}
}

func TestAnImportChecksItsSpecsBranchTags(t *testing.T) {
	f := branchWorld(t)
	if err := f.setBranch("acme-1", "feat/4711"); err != nil {
		t.Fatal(err)
	}
	spec := func(source, branch string) api.ImportSpec {
		return api.ImportSpec{Source: source, Title: source, Tags: map[string]string{core.BranchTag: branch}}
	}
	for _, tc := range []struct {
		specs []api.ImportSpec
		field string
		says  string
	}{
		{[]api.ImportSpec{spec("a", "spec/a")}, "specs[0].tags.branch", "starts with spec/"},
		{[]api.ImportSpec{spec("a", "feat/1"), spec("b", "feat/4711")}, "specs[1].tags.branch", "acme-1"},
		{[]api.ImportSpec{spec("a", "feat/1"), spec("b", "feat/1")}, "specs[1].tags.branch", "specs[0]"},
	} {
		_, err := f.core.Import(ctx, "acme", api.ImportInput{Payload: api.ImportPayload{Specs: tc.specs}})
		wantCode(t, err, api.Invalid, tc.field)
		wantSays(t, err, tc.says)
	}
	if _, err := f.core.Import(ctx, "acme", api.ImportInput{Payload: api.ImportPayload{Specs: []api.ImportSpec{spec("a", "feat/1")}}}); err != nil {
		t.Errorf("a good branch Tag was refused: %v", err)
	}
}

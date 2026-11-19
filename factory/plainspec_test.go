package factory_test

import (
	"context"
	"strings"
	"testing"

	"github.com/tomoram/code-central/api"
	"github.com/tomoram/code-central/domain"

	"github.com/tomoram/code-central/factory/factorytest"
)

// Land per Spec covers a plain Spec, one with no Stages, as it covers a
// Requirement (ADR 0031); a Long-lived Spec's Tickets land one by one.

func TestAPlainSpecIsMergedAndPushedOnceAndItsTicketsLandTogether(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t)
	remote := h.AddRemote("origin")
	mergeAndPush(h, "origin")
	h.MakePlainSpec()
	a, _ := accepted(h, "First", "one.txt", "1\n")
	b, _ := accepted(h, "Second", "two.txt", "2\n")

	h.Start()
	h.WantState(a, domain.Done)
	h.WantState(b, domain.Done)

	merge := h.Git("rev-parse", "main")
	if got := h.Git("log", "-1", "--format=%s", merge); got != "Merge spec "+h.SpecID()+": Work" {
		t.Errorf("merge subject %q", got)
	}
	if got := remote.Git("rev-parse", "main"); got != merge {
		t.Errorf("the remote's main is %s, want %s", got, merge)
	}
	for _, id := range []string{a, b} {
		if m := h.Ticket(id).Moves; m[len(m)-1].Commit != merge {
			t.Errorf("%s landed at %q, want the merge %s", id, m[len(m)-1].Commit, merge)
		}
	}
	if got := h.Git("log", "--first-parent", "--format=%s", "main"); strings.Count(got, "Merge spec ") != 1 || strings.Contains(got, "Merge "+a) {
		t.Errorf("main's first-parent history %q, want the one Spec merge and no Ticket merge of its own", got)
	}
}

func TestATicketAddedAfterAPlainSpecLandedLandsWithASecondBatch(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t)
	h.MakePlainSpec()
	a, _ := accepted(h, "First", "one.txt", "1\n")
	r := h.Start()
	h.WantState(a, domain.Done)
	first := h.Git("rev-parse", "main")

	late, _ := accepted(h, "Late", "late.txt", "late\n")
	r.Poll()
	h.WantState(late, domain.Done)
	second := h.Git("rev-parse", "main")
	if second == first {
		t.Fatal("main didn't move for the late Ticket")
	}
	if got := h.Git("log", "-1", "--format=%s", second); got != "Merge spec "+h.SpecID()+": Work" {
		t.Errorf("second batch merge subject %q", got)
	}
	if m := h.Ticket(late).Moves; m[len(m)-1].Commit != second {
		t.Errorf("late Ticket landed at %q, want %s", m[len(m)-1].Commit, second)
	}
}

func TestLabellingASpecLongLivedWhileItsTicketsWaitLandsThemTogetherAtOnce(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t)
	h.MakePlainSpec()
	a, b := accepted(h, "First", "one.txt", "1\n")
	_ = b
	c, _ := accepted(h, "Second", "two.txt", "2\n")
	release := holdTicketsStage(t, h)
	defer release()
	r := h.Start()
	r.WantOutput(c + ": merged into spec/" + h.SpecID())

	if _, err := h.Human().AddLabels(context.Background(), h.Project, api.LabelsInput{Target: api.Ref(h.SpecID()), Labels: []string{domain.LongLivedLabel}}); err != nil {
		t.Fatal(err)
	}
	r.Poll()
	h.WantState(a, domain.Done)
	h.WantState(c, domain.Done)
	merge := h.Git("rev-parse", "main")
	if got := h.Git("log", "-1", "--format=%s", merge); got != "Merge spec "+h.SpecID()+": Work" {
		t.Errorf("merge subject %q", got)
	}
}

func TestALongLivedSpecsTicketsLandOneByOne(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t)
	a, _ := accepted(h, "First", "one.txt", "1\n")
	b, _ := accepted(h, "Second", "two.txt", "2\n")

	h.Start()
	h.WantState(a, domain.Done)
	h.WantState(b, domain.Done)
	if got := h.Git("branch", "--list", "spec/*"); got != "" {
		t.Errorf("a Spec Branch was made for a Long-lived Spec: %q", got)
	}
	if got := h.Git("log", "-1", "--format=%s", "main"); !strings.HasPrefix(got, "Merge "+b) {
		t.Errorf("main's tip is %q, want a plain merge of %s", got, b)
	}
}

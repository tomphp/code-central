package core_test

import (
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"testing"
	"time"

	"github.com/tomoram/code-central/clock"
	"github.com/tomoram/code-central/core"
	"github.com/tomoram/code-central/ids"
	"github.com/tomoram/code-central/store"
)

// A Project's CI state (Spec code-central-568) through core's public
// operations: every reader sees the same CI for Tickets that share a
// pushed commit, and a response on the Project covers them all.

// Tickets sharing a pushed commit read its runs whichever Ticket they
// were tracked against (code-central-418): the same runs, result and
// CI item on get_ticket, on find_tickets rows and in Needs Me.
func TestTicketsSharingAPushedCommitReadTheSameCI(t *testing.T) {
	f := itemWorld(t)
	f.ciRun(t, 1, "ci.yml", "abc123", "acme-5", store.CICompleted, store.CIFailure)
	f.ciRun(t, 2, "lint.yml", "abc123", "acme-8", store.CICompleted, store.CISuccess)
	for _, id := range []string{"acme-5", "acme-8"} {
		tk := f.getTicket(t, id)
		if tk.CIResult != core.CIFailed || tk.CICommit != "abc123" || len(tk.CIRuns) != 2 {
			t.Errorf("%s: result %q from %q with %d runs, want failed at abc123 with both runs", id, tk.CIResult, tk.CICommit, len(tk.CIRuns))
		}
		if tk.CIItem == nil || tk.CIItem.Commit != "abc123" || !slices.Equal(listed(tk.CIItem), []string{"acme-5", "acme-8"}) {
			t.Errorf("%s: item %+v, want abc123's listing both", id, tk.CIItem)
		}
	}
	rows, err := f.core.FindTickets(ctx, "acme", core.FindTicketsInput{State: []store.State{store.Accepted}})
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range rows {
		if row.CIResult != core.CIFailed {
			t.Errorf("%s's row: result %q, want failed", row.ID, row.CIResult)
		}
	}
	if row := f.theItem(t, core.ReasonCIFailed); row.CIResult != core.CIFailed || row.CIFailure == nil || !slices.Equal(row.CIFailure.Workflows, []string{"ci.yml"}) {
		t.Fatalf("item row %+v, want failed in ci.yml", row)
	}
}

// A response stored on the Project for a commit covers every Ticket
// waiting on it (code-central-432), whichever Ticket it was named on.
func TestAResponseOnTheProjectCoversEveryTicketOnTheCommit(t *testing.T) {
	f := itemWorld(t)
	f.ciRun(t, 1, "ci.yml", "abc123", "acme-5", store.CICompleted, store.CIFailure)
	if _, err := f.core.RerunCI(human, "acme", core.TicketInput{Ticket: "acme-5"}); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"acme-5", "acme-8"} {
		if tk := f.getTicket(t, id); tk.CIResult != core.CIRunning || tk.NeedsMeReason != "" || tk.CIItem != nil {
			t.Errorf("%s: result %q, reason %q, item %+v, want running on the commit's Re-run request", id, tk.CIResult, tk.NeedsMeReason, tk.CIItem)
		}
	}
	w, err := f.core.GetCIWatch(ctx, "acme")
	if err != nil {
		t.Fatal(err)
	}
	if len(w.Reruns) != 1 || w.Reruns[0].Commit != "abc123" || w.Reruns[0].ID != 1 || !w.Reruns[0].Requested.Equal(f.clock.Now()) {
		t.Fatalf("reruns %+v, want run 1 on abc123 asked for once, now", w.Reruns)
	}
	if len(w.Waiting) != 1 || w.Waiting[0].Commit != "abc123" {
		t.Fatalf("waiting %+v, want abc123 watched again", w.Waiting)
	}
}

// A Land anyway or Re-run request the human gave before CI items, stored
// on one Ticket's pushed commit, keeps its effect once the store is
// migrated: it moves onto the Project for its commit, so it now covers
// every Ticket waiting on that commit.
func TestAResponseStoredOnAPushedCommitBeforeCIItemsKeepsItsEffectOnceMigrated(t *testing.T) {
	dir := t.TempDir()
	s, err := store.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	clk := clock.NewFake(t0)
	f := itemWorldOn(t, fixture{core: core.New(s, clk, &ids.Sequence{}), clock: clk, store: s})
	f.moveTo(t, 6, store.Accepted)
	f.pushed(t, "acme-6", "def456")
	f.ciRun(t, 1, "ci.yml", "abc123", "acme-5", store.CICompleted, store.CIFailure)
	f.ciRun(t, 2, "ci.yml", "def456", "acme-6", store.CICompleted, store.CIFailure)
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	at := f.clock.Now().Format(time.RFC3339)
	asFormat36(t, dir)
	addToPushedCommit(t, dir, 5, "    land_anyway: "+at+"\n")
	addToPushedCommit(t, dir, 6, "    rerun: "+at+"\n    reruns:\n      - id: 2\n        attempt: 1\n")

	if s, err = store.Open(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	if m := s.Migrated(); m == nil || m.From != 36 {
		t.Fatalf("migrated %+v, want from format 36", m)
	}
	f = fixture{core: core.New(s, f.clock, &ids.Sequence{}), clock: f.clock, store: s}
	if rows := f.needsMe(t); len(rows) != 0 {
		t.Fatalf("needs me %+v, want nothing waiting", rows)
	}
	for _, id := range []string{"acme-5", "acme-8"} {
		if tk := f.getTicket(t, id); !tk.LandAnyway || tk.NeedsMeReason != "" || tk.CIItem != nil {
			t.Errorf("%s: land anyway %v, reason %q, item %+v, want abc123 landed anyway", id, tk.LandAnyway, tk.NeedsMeReason, tk.CIItem)
		}
	}
	if tk := f.getTicket(t, "acme-6"); tk.CIResult != core.CIRunning || tk.NeedsMeReason != "" {
		t.Fatalf("acme-6: result %q, reason %q, want running on its Re-run request", tk.CIResult, tk.NeedsMeReason)
	}
	w, err := f.core.GetCIWatch(ctx, "acme")
	if err != nil {
		t.Fatal(err)
	}
	if len(w.Reruns) != 1 || w.Reruns[0].Ticket != "acme-6" || w.Reruns[0].Commit != "def456" || w.Reruns[0].ID != 2 {
		t.Fatalf("reruns %+v, want def456's run 2 for acme-6", w.Reruns)
	}
}

// asFormat36 marks the store in dir as format 36, the last before the
// legacy CI responses moved; its files are otherwise the same.
func asFormat36(t *testing.T, dir string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, "store.yaml"), []byte("format: 36\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

// addToPushedCommit adds lines to acme Ticket n's only pushed commit, as
// a format-36 store held a response there.
func addToPushedCommit(t *testing.T, dir string, n int, lines string) {
	t.Helper()
	path := filepath.Join(dir, "projects", "acme", "tickets", strconv.Itoa(n)+".yaml")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	pushed := regexp.MustCompile(`(?m)^    pushed: .*\n`)
	if len(pushed.FindAllIndex(data, -1)) != 1 {
		t.Fatalf("acme-%d has no single pushed commit:\n%s", n, data)
	}
	data = pushed.ReplaceAllFunc(data, func(line []byte) []byte { return append(slices.Clone(line), lines...) })
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
}

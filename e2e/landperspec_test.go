//go:build e2e

package e2e

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A two-Ticket Spec with a Blocker, under Land per Spec (on by default under
// Merge): the second Ticket starts once the first is accepted, and both land
// together in one merge of the Spec Branch.
func TestATwoTicketSpecLandsTogetherThroughItsSpecBranch(t *testing.T) {
	w := startWorld(t, twoTicketSpec+"\n\nSay hello, then goodbye.")
	inRepo := w.inRepo

	for _, id := range []string{"acme-2", "acme-3", "acme-4"} {
		w.waitFor(id, "In Review")
		w.accept(id)
		w.waitFor(id, "Done")
	}

	// The first Ticket's branch is cut from the Spec Branch, which is cut from the base.
	first := w.waitFor("acme-5", "In Review")
	if got := inRepo("merge-base", "spec/acme-1", "acme-5"); got != w.base {
		t.Errorf("spec/acme-1 and acme-5 meet at %s, want the base %s", got, w.base)
	}
	if blocked := w.ticket("acme-6"); blocked.State == "In Progress" || blocked.State == "In Review" {
		t.Fatalf("acme-6 started while its Blocker was In Review: %s", blocked.State)
	}

	// Accepting it merges it into the Spec Branch, not the base, and clears
	// the Blocker: the second Ticket starts.
	w.accept("acme-5")
	second := w.waitFor("acme-6", "In Review")
	if got := inRepo("rev-parse", "main"); got != w.base {
		t.Fatalf("main moved to %s before the Spec was ready", got)
	}
	if got := w.ticket("acme-5").State; got != "Accepted" {
		t.Errorf("acme-5 is %s, want Accepted while it waits for its Spec", got)
	}
	inRepo("merge-base", "--is-ancestor", first.Commit, "spec/acme-1") // fails the test if not
	if got := inRepo("merge-base", "spec/acme-1", "acme-6"); got == w.base {
		t.Errorf("acme-6 wasn't cut from the Spec Branch's tip, which has acme-5")
	}

	// Accepting the last one makes the Spec ready: one merge lands both.
	w.accept("acme-6")
	d5, d6 := w.waitFor("acme-5", "Done"), w.waitFor("acme-6", "Done")
	merge := inRepo("rev-parse", "main")
	if got := inRepo("log", "-1", "--format=%s", merge); got != "Merge spec acme-1: "+twoTicketSpec {
		t.Errorf("main's tip is %q", got)
	}
	for _, d := range []ticketView{d5, d6} {
		if m := d.Moves[len(d.Moves)-1]; m.Commit != merge {
			t.Errorf("%s landed at %q, want the Spec's merge %s", d.ID, m.Commit, merge)
		}
	}
	for _, f := range []string{"hello.txt", "goodbye.txt"} {
		if _, err := os.Stat(filepath.Join(w.repo, f)); err != nil {
			t.Errorf("%s isn't on main: %v", f, err)
		}
	}
	inRepo("merge-base", "--is-ancestor", second.Commit, "main")
	if out := w.factoryOut.String(); !strings.Contains(out, "acme-1: merged spec/acme-1 into main") {
		t.Errorf("the Factory didn't report the Spec merge:\n%s", out)
	}
	w.stop(t)
}

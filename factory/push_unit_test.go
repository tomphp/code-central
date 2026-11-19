package factory

import (
	"context"
	"strings"
	"testing"
)

// The push helper's force flag (ADR 0067): only the Spec Branch's and a
// Ticket's branch's pushes set it, and it is refused for a base, which
// is never force-pushed, whoever asks.

// pushedElsewhere puts the remote origin's branch, and the fake's
// remote-tracking ref of it, at a commit of its own, as a push the
// Factory has since fetched, and returns it.
func pushedElsewhere(w *landWorld, branch string) string {
	theirs := w.repo.commitOn("elsewhere-"+branch, "Pushed from elsewhere")
	w.repo.mu.Lock()
	defer w.repo.mu.Unlock()
	w.repo.remotes["origin"].refs[branch] = theirs
	w.repo.refs["refs/remotes/origin/"+branch] = theirs
	return theirs
}

func TestForceNeverPushesABaseButReplacesAnotherBranchWithLease(t *testing.T) {
	t.Parallel()
	w := newLandWorld(t)
	w.repo.addRemote("origin")
	ctx := context.Background()
	home := w.l.repos.home

	theirs := pushedElsewhere(w, "main")
	base := w.repo.commitOn("main", "Local work")
	pushed, why := w.l.push(ctx, home, "origin", "main", base, true)
	if pushed || !strings.Contains(why, "never force-pushes a base") {
		t.Errorf("forcing main pushed %v, why %q; want it refused", pushed, why)
	}
	if got := w.repo.remoteTip("origin", "main"); got != theirs || w.repo.pushes != 0 {
		t.Errorf("the remote's main is at %s after %d pushes, want it left at %s", got, w.repo.pushes, theirs)
	}

	pushedElsewhere(w, "spec/acme-1")
	spec := w.repo.commitOn("spec/acme-1", "Rebased Spec work")
	if pushed, why := w.l.push(ctx, home, "origin", "spec/acme-1", spec, true); !pushed || why != "" {
		t.Fatalf("forcing the Spec Branch pushed %v, why %q; want it pushed with lease", pushed, why)
	}
	if got := w.repo.remoteTip("origin", "spec/acme-1"); got != spec {
		t.Errorf("the remote's Spec Branch is at %s, want the forced %s", got, spec)
	}
}

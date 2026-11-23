package core

import (
	"slices"

	"github.com/tomoram/code-central/domain"
	"github.com/tomoram/code-central/store"
)

// wakingStates are the States a move into wakes the Factories: Ready
// (claimable again), Answered (to resume), Accepted (to land), and Done
// and Cancelled (which may clear another Ticket's last Blocker).
var wakingStates = []domain.State{domain.Ready, domain.Answered, domain.Accepted, domain.Done, domain.Cancelled}

// wakesTheFactory says whether writing Ticket t, over the record r holds
// before the write, is a change the Factory's poll acts on: t moves into
// one of the wakingStates (a new Ticket counts as moving into its
// State), a live Claim on it ends by a move its holder didn't make, or
// its Blockers shrink.
func wakesTheFactory(r store.Reader, key string, t domain.Ticket) bool {
	was, found := r.Ticket(key, t.Number)
	if !found {
		return slices.Contains(wakingStates, t.State)
	}
	movedIn := was.State != t.State && slices.Contains(wakingStates, t.State)
	return movedIn || claimWithdrawn(was, t) || blockersShrank(was, t)
}

// blockersShrank says whether t has lost a Blocker was had, which may
// leave it claimable.
func blockersShrank(was, t domain.Ticket) bool {
	return slices.ContainsFunc(was.BlockedBy, func(n int) bool { return !slices.Contains(t.BlockedBy, n) })
}

// claimWithdrawn says whether the Claim live on was has ended on t by an
// action other than its holder's own, such as cancel, release or a move
// by hand: the Factory may be running its turn, and has a slot back.
func claimWithdrawn(was, t domain.Ticket) bool {
	live, ok := liveClaim(was)
	if !ok {
		return false
	}
	for _, c := range t.Claims {
		if c.Number == live.Number {
			return !c.Live() && !isHolderAction(c.EndedBy)
		}
	}
	return false
}

// projectWakesTheFactory says whether writing Project p, over the record
// r holds before the write, changes a setting the Factory reads: the
// base, Landing, CI, Models, Stage Guides, Auto-accept or Archived. The
// Push remote the Factory reports, its Reported Models and UI-only
// settings such as Focus Kinds don't count.
func projectWakesTheFactory(r store.Reader, p domain.Project) bool {
	was, found := r.Project(p.Key)
	if !found {
		return false
	}
	ci := func(c domain.CI) domain.CI {
		c.PushRemoteURL, c.NoPushRemote, c.AutoFix = "", false, domain.AutoFix{}
		return c
	}
	return was.Base() != p.Base() || was.Landing != p.Landing || !ci(was.CI).Equal(ci(p.CI)) ||
		!was.Models.Equal(p.Models) || was.StageGuides != p.StageGuides ||
		!slices.Equal(was.AutoAccept, p.AutoAccept) || !was.Archived.Equal(p.Archived)
}

// specWakesTheFactory says whether writing Spec s, over the record r
// holds before the write, turns a Map's guided Tag on: the Factory then
// starts its next decision.
func specWakesTheFactory(r store.Reader, key string, s domain.Spec) bool {
	guided := func(s domain.Spec) bool { return isMap(s.Labels) && s.Tags[domain.GuidedTag] == domain.GuidedOn }
	was, found := r.Spec(key, s.Number)
	return guided(s) && !(found && guided(was))
}

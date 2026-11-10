package core

import (
	"context"
	"fmt"
	"slices"
	"time"

	"github.com/tomoram/code-central/store"
)

// CI failed (code-central-261): an Accepted Ticket whose CI result is
// failed waits on the human in Needs Me until they choose one of the
// failure options. Land anyway, the human's land_anyway, accepts the
// failed CI result, and the Factory lands the Ticket on its next poll.

// The failure options, each named for its operation.
const (
	// OptionLandAnyway is land_anyway, offered only under Merge: under
	// Branch the human's merge decides.
	OptionLandAnyway = "land_anyway"
)

// CIFailureSummary is what failed on a CI failed Ticket's pushed commit:
// the commit and its failed runs' workflow file names.
type CIFailureSummary struct {
	Commit    string   `json:"commit"`
	Workflows []string `json:"workflows"`
}

// ciFailure is t's failure while it is CI failed: an Accepted Ticket
// whose latest pushed commit's CI result is failed and that the human
// hasn't dealt with. since is when its first failed run finished.
func ciFailure(r store.Reader, key string, t store.Ticket) (s CIFailureSummary, since time.Time, ok bool) {
	if t.State != store.Accepted || len(t.PushedCommits) == 0 {
		return CIFailureSummary{}, time.Time{}, false
	}
	latest := t.PushedCommits[len(t.PushedCommits)-1]
	if !latest.LandAnyway.IsZero() {
		return CIFailureSummary{}, time.Time{}, false
	}
	mine, _ := ticketRuns(r, key, t)
	if ciResultOf(latest.Commit, mine) != CIFailed {
		return CIFailureSummary{}, time.Time{}, false
	}
	s = CIFailureSummary{Commit: latest.Commit, Workflows: []string{}}
	for _, run := range countedRuns(latest.Commit, mine) {
		if run.Status != store.CICompleted || !run.Conclusion.Failed() {
			continue
		}
		s.Workflows = append(s.Workflows, run.Workflow)
		if at := run.Finished; !at.IsZero() && (since.IsZero() || at.Before(since)) {
			since = at
		}
	}
	slices.Sort(s.Workflows)
	s.Workflows = slices.Compact(s.Workflows)
	if since.IsZero() {
		since = latest.Pushed
	}
	return s, since, true
}

// ciFailureOptions are the failure options the human may choose for t
// while it is CI failed, nil otherwise: Land anyway under Merge, none yet
// under Branch, where the human's merge decides.
func ciFailureOptions(r store.Reader, key string, t store.Ticket) []string {
	if _, _, ok := ciFailure(r, key, t); !ok {
		return nil
	}
	p, _ := r.Project(key)
	if p.Landing.Method == store.LandingBranch {
		return []string{}
	}
	return []string{OptionLandAnyway}
}

// landAnyway reports whether the human chose Land anyway for t's latest
// pushed commit.
func landAnyway(t store.Ticket) bool {
	n := len(t.PushedCommits)
	return n > 0 && !t.PushedCommits[n-1].LandAnyway.IsZero()
}

// LandAnyway is the human's land_anyway(ticket): it accepts the failed CI
// result of an Accepted Ticket's latest pushed commit, recorded on that
// commit, so the Factory lands the Ticket on its next poll. A newer
// pushed commit has its own CI result, which it doesn't cover. Choosing
// it again changes nothing.
func (c *Core) LandAnyway(ctx context.Context, key string, in TicketInput) (Written[TicketView], error) {
	if err := humanOnly(ctx, "land_anyway"); err != nil {
		return Written[TicketView]{}, err
	}
	return c.updateTicketIn(key, "land_anyway", in.Ticket, in.Full, func(r store.Reader, t *store.Ticket) (bool, error) {
		if landAnyway(*t) && t.State == store.Accepted {
			return false, nil
		}
		if !slices.Contains(ciFailureOptions(r, key, *t), OptionLandAnyway) {
			return false, &Error{Code: ActionNotAllowed, Message: whyNotLandAnyway(r, key, *t)}
		}
		t.PushedCommits[len(t.PushedCommits)-1].LandAnyway = c.now()
		return true, nil
	})
}

// whyNotLandAnyway says why t isn't offered Land anyway.
func whyNotLandAnyway(r store.Reader, key string, t store.Ticket) string {
	id := DisplayID(key, t.Number)
	if t.State != store.Accepted {
		return fmt.Sprintf("%s is %s: only an Accepted Ticket whose CI failed lands anyway", id, t.State)
	}
	if _, _, ok := ciFailure(r, key, t); !ok {
		return fmt.Sprintf("%s has no failed CI result to land anyway", id)
	}
	return fmt.Sprintf("%s's Project lands by Branch, so your merge decides whether it lands, whatever its CI result", id)
}

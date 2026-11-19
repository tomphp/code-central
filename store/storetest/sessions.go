package storetest

import (
	"testing"
	"time"

	"github.com/tomoram/code-central/domain"
	"github.com/tomoram/code-central/store"
)

// ClaimBySession (Spec code-central-1032) finds a Claim by its session ID
// across Projects, for transcripts and usage.

func runClaimBySession(t *testing.T, f Factory) {
	t.Run("a claim is found by its session ID, the newest started across projects", func(t *testing.T) {
		s := f.New(t)
		for _, key := range []string{"acme", "beta"} {
			put(t, s, func(w store.Writer) error {
				return w.PutProject(domain.Project{Key: key, DisplayName: key, Created: t0, Mode: "committed"})
			})
		}
		claim := func(n int, session string, started time.Time) domain.Claim {
			return domain.Claim{Number: n, ID: "C" + session, SessionID: session, Started: started, Ended: started.Add(time.Minute), EndedBy: "finish"}
		}
		for _, x := range []struct {
			project string
			ticket  domain.Ticket
		}{
			// s1 claimed acme-1, then beta-2; s2 claimed acme-1 and
			// acme-3 at the same moment; s3 claimed beta-4, still live.
			{"acme", domain.Ticket{Number: 1, Claims: []domain.Claim{claim(1, "s1", t0), claim(2, "s2", t0.Add(time.Hour))}}},
			{"beta", domain.Ticket{Number: 2, Claims: []domain.Claim{claim(1, "s1", t0.Add(2*time.Hour))}}},
			{"acme", domain.Ticket{Number: 3, Claims: []domain.Claim{claim(1, "s2", t0.Add(time.Hour))}}},
			{"beta", domain.Ticket{Number: 4, Claims: []domain.Claim{{Number: 1, ID: "L", SessionID: "s3", Started: t0}}}},
			// A Claim with no session ID is never found.
			{"beta", domain.Ticket{Number: 5, Claims: []domain.Claim{{Number: 1, ID: "N", Started: t0}}}},
		} {
			x.ticket.Title, x.ticket.State, x.ticket.Spec, x.ticket.Created, x.ticket.Updated = "T", domain.Done, 1, t0, t0
			put(t, s, func(w store.Writer) error { return w.PutTicket(x.project, x.ticket, store.Claims) })
		}
		read(t, s, func(r store.Reader) {
			for _, c := range []struct {
				session string
				want    store.SessionClaim
			}{
				{"s1", store.SessionClaim{Project: "beta", Ticket: 2, Claim: claim(1, "s1", t0.Add(2*time.Hour))}},
				// On a tie, the first in Project, Ticket and Claim order.
				{"s2", store.SessionClaim{Project: "acme", Ticket: 1, Claim: claim(2, "s2", t0.Add(time.Hour))}},
				{"s3", store.SessionClaim{Project: "beta", Ticket: 4, Claim: domain.Claim{Number: 1, ID: "L", SessionID: "s3", Started: t0}}},
			} {
				got, ok := r.ClaimBySession(c.session)
				same := got.Project == c.want.Project && got.Ticket == c.want.Ticket && got.Claim.Number == c.want.Claim.Number &&
					got.Claim.SessionID == c.want.Claim.SessionID && got.Claim.Started.Equal(c.want.Claim.Started) && got.Claim.Ended.Equal(c.want.Claim.Ended)
				if !ok || !same {
					t.Errorf("%s: found %+v, %v; want %+v", c.session, got, ok, c.want)
				}
			}
			for _, session := range []string{"s4", ""} {
				if got, ok := r.ClaimBySession(session); ok {
					t.Errorf("session %q found %+v", session, got)
				}
			}
		})
	})
}

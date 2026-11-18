package storetest

import (
	"fmt"
	"maps"
	"slices"
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

// TicketCommits (Spec code-central-1032) lists every pushed and landed
// commit of a Project's Tickets, for the CI watch.

func runTicketCommits(t *testing.T, f Factory) {
	t.Run("ticket commits are every pushed and landed commit, in ticket order", func(t *testing.T) {
		s := f.New(t)
		seedProject(t, s)
		read(t, s, func(r store.Reader) {
			if got := r.TicketCommits("acme"); len(got) != 0 {
				t.Errorf("a project with no Tickets has commits %+v", got)
			}
		})
		for _, x := range []domain.Ticket{
			// 3 pushed in the Home Repository and in web, and landed in
			// both; the Home Repository's landing is its Move's commit,
			// its entry among the Repositories saying it again.
			{Number: 3, Spec: 1, State: domain.Done,
				PushedCommits: []domain.PushedCommit{
					{Commit: "h1", Branch: "acme-3", Pushed: t0},
					{Repository: "web", Commit: "w1", Branch: "acme-3", Pushed: t0},
					{Commit: "h2", Pushed: t0.Add(time.Hour)},
				},
				Repositories: []domain.ChangedRepository{
					{Name: "acme", Branch: "acme-3", Commit: "h2", Landed: &domain.Landed{Commit: "hl", Time: t0}},
					{Name: "web", Branch: "acme-3", Commit: "w1", Landed: &domain.Landed{Commit: "wl", Time: t0}},
					{Name: "api", Branch: "acme-3", Commit: "a1"},
				},
				Moves: []domain.Move{
					{From: domain.Ready, To: domain.InProgress, Actor: "agent", Time: t0},
					{From: domain.Accepted, To: domain.Done, Actor: "human", Time: t0, Commit: "hl"},
				},
			},
			// 2 has no commits, and 1 shares 3's first.
			{Number: 2, Spec: 1, State: domain.Ready},
			{Number: 1, Spec: 4, State: domain.Accepted, PushedCommits: []domain.PushedCommit{{Commit: "h1", Pushed: t0}}},
		} {
			x.Title, x.Created, x.Updated = "T", t0, t0
			put(t, s, func(w store.Writer) error { return w.PutTicket("acme", x, store.Moves) })
		}
		read(t, s, func(r store.Reader) {
			want := []store.TicketCommit{
				{Ticket: 1, Spec: 4, Commit: "h1", Kind: store.CommitPushed},
				{Ticket: 3, Spec: 1, Commit: "h1", Kind: store.CommitPushed},
				{Ticket: 3, Spec: 1, Repository: "web", Commit: "w1", Kind: store.CommitPushed},
				{Ticket: 3, Spec: 1, Commit: "h2", Kind: store.CommitPushed},
				{Ticket: 3, Spec: 1, Commit: "hl", Kind: store.CommitLanded},
				{Ticket: 3, Spec: 1, Repository: "web", Commit: "wl", Kind: store.CommitLanded},
			}
			if got := r.TicketCommits("acme"); !slices.Equal(got, want) {
				t.Errorf("commits\n%+v\nwant\n%+v", got, want)
			}
			if got := r.TicketCommits("other"); len(got) != 0 {
				t.Errorf("an unknown project has commits %+v", got)
			}
		})
	})
}

// Records (Spec code-central-1032) reads a Project's records whole, every
// History part with each Ticket, for Export and Store Move.

func runRecords(t *testing.T, f Factory) {
	t.Run("records are a project's specs and then its tickets whole, in number order", func(t *testing.T) {
		s := f.New(t)
		seedProject(t, s)
		for _, n := range []int{4, 1} {
			put(t, s, func(w store.Writer) error {
				return w.PutSpec("acme", domain.Spec{Number: n, Title: "S", Created: t0, Updated: t0})
			})
		}
		for _, n := range []int{3, 2} {
			x := historyTicket()
			x.Number = n
			put(t, s, func(w store.Writer) error { return w.PutTicket("acme", x, store.History...) })
		}
		var got []string
		read(t, s, func(r store.Reader) {
			for rec := range r.Records("acme") {
				switch {
				case rec.Spec != nil && rec.Ticket == nil:
					got = append(got, fmt.Sprint("spec ", rec.Spec.Number))
				case rec.Ticket != nil && rec.Spec == nil:
					got = append(got, fmt.Sprint("ticket ", rec.Ticket.Number))
					if parts, want := partsOf(*rec.Ticket), partsOf(historyTicket()); !maps.Equal(parts, want) {
						t.Errorf("ticket %d has parts %v, want %v", rec.Ticket.Number, parts, want)
					}
					if !headEqual(rec.Ticket.Head, historyHead()) {
						t.Errorf("ticket %d has head %s, want %s", rec.Ticket.Number, showHead(rec.Ticket.Head), showHead(historyHead()))
					}
				default:
					t.Errorf("a record is neither one Spec nor one Ticket: %+v", rec)
				}
			}
			for range r.Records("other") {
				t.Error("an unknown project has records")
			}
			// A read may stop part way.
			for range r.Records("acme") {
				break
			}
		})
		if want := []string{"spec 1", "spec 4", "ticket 2", "ticket 3"}; !slices.Equal(got, want) {
			t.Errorf("records %v, want %v", got, want)
		}
	})
}

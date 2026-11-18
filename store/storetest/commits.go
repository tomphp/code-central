package storetest

import (
	"slices"
	"testing"
	"time"

	"github.com/tomoram/code-central/domain"
	"github.com/tomoram/code-central/store"
)

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

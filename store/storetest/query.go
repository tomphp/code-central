package storetest

import (
	"maps"
	"slices"
	"testing"
	"time"

	"github.com/tomoram/code-central/domain"
	"github.com/tomoram/code-central/store"
)

// The Reader's queries (Spec code-central-1032): FindTickets and
// FindSpecs pick records by one filter each, and StateCounts counts the
// Tickets in each State.

// numbers are the numbers of tickets, in their order.
func numbers(tickets []domain.Ticket) []int {
	out := []int{}
	for _, t := range tickets {
		out = append(out, t.Number)
	}
	return out
}

func runQueries(t *testing.T, f Factory) {
	t.Run("tickets are found by Spec, in number or claim order", func(t *testing.T) {
		s := f.New(t)
		seedProject(t, s)
		for _, n := range []int{1, 2} {
			put(t, s, func(w store.Writer) error {
				return w.PutSpec("acme", domain.Spec{Number: n, Title: "S", Created: t0, Updated: t0})
			})
		}
		// 5 was created first, and 3 and 4 at the same time.
		for _, x := range []domain.Ticket{
			{Number: 3, Spec: 1, Created: t0.Add(time.Hour)},
			{Number: 4, Spec: 1, Created: t0.Add(time.Hour)},
			{Number: 5, Spec: 1, Created: t0},
			{Number: 6, Spec: 2, Created: t0},
		} {
			x.Title, x.State, x.Updated = "T", domain.Ready, t0
			put(t, s, func(w store.Writer) error { return w.PutTicket("acme", x) })
		}
		read(t, s, func(r store.Reader) {
			for _, c := range []struct {
				name string
				f    store.TicketFilter
				want []int
			}{
				{"one Spec", store.TicketFilter{Spec: []int{1}}, []int{3, 4, 5}},
				{"several Specs", store.TicketFilter{Spec: []int{1, 2}}, []int{3, 4, 5, 6}},
				{"a Spec with none", store.TicketFilter{Spec: []int{7}}, []int{}},
				{"every Ticket", store.TicketFilter{}, []int{3, 4, 5, 6}},
				{"claim order", store.TicketFilter{Spec: []int{1}, Order: store.ByClaim}, []int{5, 3, 4}},
				{"claim order of every Ticket", store.TicketFilter{Order: store.ByClaim}, []int{5, 6, 3, 4}},
			} {
				if got := numbers(r.FindTickets("acme", c.f)); !slices.Equal(got, c.want) {
					t.Errorf("%s: found %v, want %v", c.name, got, c.want)
				}
			}
			if got := r.FindTickets("other", store.TicketFilter{}); len(got) != 0 {
				t.Errorf("an unknown project has Tickets %v", numbers(got))
			}
		})
	})

	t.Run("tickets are found by each filter field, the fields combined", func(t *testing.T) {
		s := f.New(t)
		seedProject(t, s)
		put(t, s, func(w store.Writer) error {
			return w.PutSpec("acme", domain.Spec{Number: 1, Title: "S", Created: t0, Updated: t0})
		})
		pushed := func(repository, commit, branch string) domain.PushedCommit {
			return domain.PushedCommit{Repository: repository, Commit: commit, Branch: branch, Pushed: t0}
		}
		for _, x := range []domain.Ticket{
			{Number: 2, State: domain.Ready, Labels: []string{"ui", "auth"}, Tags: map[string]string{"source": "gh-1"}},
			{Number: 3, State: domain.InReview, Labels: []string{"ui"}, Tags: map[string]string{"source": "gh-2", "area": "web"},
				Link: domain.Link{Integration: "github-issues", ID: "12"}},
			{Number: 4, State: domain.Accepted, Link: domain.Link{Integration: "github-issues", ID: "13", Detached: true},
				PushedCommits: []domain.PushedCommit{pushed("", "aaa", "acme-4"), pushed("", "bbb", "acme-4")}},
			{Number: 5, State: domain.Accepted, Link: domain.Link{Integration: "azure-boards", ID: "12"},
				PushedCommits: []domain.PushedCommit{pushed("api", "aaa", "acme-5")}, BlockedBy: []int{2, 3}},
			{Number: 6, State: domain.Cancelled, Replacements: []int{7, 8}, BlockedBy: []int{3}},
			{Number: 7, State: domain.Done, Labels: []string{"auth"}, Replacements: []int{8}},
			{Number: 8, State: domain.Draft},
		} {
			x.Title, x.Spec, x.Created, x.Updated = "T", 1, t0, t0
			put(t, s, func(w store.Writer) error { return w.PutTicket("acme", x) })
		}
		read(t, s, func(r store.Reader) {
			for _, c := range []struct {
				name string
				f    store.TicketFilter
				want []int
			}{
				{"one State", store.TicketFilter{States: []domain.State{domain.Accepted}}, []int{4, 5}},
				{"several States", store.TicketFilter{States: []domain.State{domain.Ready, domain.Done, domain.Draft}}, []int{2, 7, 8}},
				{"a State none is in", store.TicketFilter{States: []domain.State{domain.Failed}}, []int{}},
				{"Label", store.TicketFilter{Label: "auth"}, []int{2, 7}},
				{"a Label none carries", store.TicketFilter{Label: "api"}, []int{}},
				{"Tag with one value", store.TicketFilter{Tag: &store.TagFilter{Key: "source", Values: []string{"gh-2"}}}, []int{3}},
				{"Tag with several values", store.TicketFilter{Tag: &store.TagFilter{Key: "source", Values: []string{"gh-1", "gh-2", "gh-9"}}}, []int{2, 3}},
				{"Tag with any value", store.TicketFilter{Tag: &store.TagFilter{Key: "area"}}, []int{3}},
				{"Tag of another key's value", store.TicketFilter{Tag: &store.TagFilter{Key: "area", Values: []string{"gh-1"}}}, []int{}},
				{"Link to an item", store.TicketFilter{Link: &store.LinkFilter{Integration: "github-issues", ID: "12"}}, []int{3}},
				{"Link to a detached item", store.TicketFilter{Link: &store.LinkFilter{Integration: "github-issues", ID: "13"}}, []int{4}},
				{"Link to any item", store.TicketFilter{Link: &store.LinkFilter{Integration: "github-issues"}}, []int{3, 4}},
				{"Link to another Integration's item", store.TicketFilter{Link: &store.LinkFilter{Integration: "azure-boards", ID: "13"}}, []int{}},
				{"PushedCommit in the Home Repository", store.TicketFilter{PushedCommit: &store.PushedCommitFilter{Commit: "aaa"}}, []int{4}},
				{"PushedCommit, not the latest", store.TicketFilter{PushedCommit: &store.PushedCommitFilter{Commit: "bbb"}}, []int{4}},
				{"PushedCommit in another Repository", store.TicketFilter{PushedCommit: &store.PushedCommitFilter{Repository: "api", Commit: "aaa"}}, []int{5}},
				{"PushedCommit in a Repository it wasn't pushed in", store.TicketFilter{PushedCommit: &store.PushedCommitFilter{Repository: "api", Commit: "bbb"}}, []int{}},
				{"PushedBranch in the Home Repository", store.TicketFilter{PushedBranch: &store.PushedBranchFilter{Branch: "acme-4"}}, []int{4}},
				{"PushedBranch in another Repository", store.TicketFilter{PushedBranch: &store.PushedBranchFilter{Repository: "api", Branch: "acme-5"}}, []int{5}},
				{"PushedBranch in a Repository it wasn't pushed in", store.TicketFilter{PushedBranch: &store.PushedBranchFilter{Branch: "acme-5"}}, []int{}},
				{"HasPushes", store.TicketFilter{HasPushes: true}, []int{4, 5}},
				{"BlockedBy", store.TicketFilter{BlockedBy: 3}, []int{5, 6}},
				{"BlockedBy a Ticket blocking none", store.TicketFilter{BlockedBy: 8}, []int{}},
				{"ReplacedBy", store.TicketFilter{ReplacedBy: 8}, []int{6, 7}},
				{"ReplacedBy a Ticket replacing none", store.TicketFilter{ReplacedBy: 2}, []int{}},
				{"fields combined", store.TicketFilter{States: []domain.State{domain.Accepted, domain.Ready}, Spec: []int{1}, HasPushes: true, BlockedBy: 2}, []int{5}},
				{"fields that pick none together", store.TicketFilter{Label: "ui", HasPushes: true}, []int{}},
			} {
				if got := numbers(r.FindTickets("acme", c.f)); !slices.Equal(got, c.want) {
					t.Errorf("%s: found %v, want %v", c.name, got, c.want)
				}
			}
		})
	})

	t.Run("found tickets carry only the History parts the filter names, and their heads", func(t *testing.T) {
		s := f.New(t)
		seedProject(t, s)
		put(t, s, func(w store.Writer) error { return w.PutTicket("acme", historyTicket(), store.History...) })
		read(t, s, func(r store.Reader) {
			heads := r.FindTickets("acme", store.TicketFilter{})
			if len(heads) != 1 || !maps.Equal(partsOf(heads[0]), partsOf(domain.Ticket{})) || !headEqual(heads[0].Head, historyHead()) {
				t.Fatalf("a head read %+v, want no History and its head", heads)
			}
			found := r.FindTickets("acme", store.TicketFilter{Parts: []store.Part{store.Moves}})
			if len(found) != 1 || !maps.Equal(partsOf(found[0]), only(store.Moves, 6)) || !headEqual(found[0].Head, historyHead()) {
				t.Fatalf("a read of Moves %+v, want only Moves and its head", found)
			}
			whole := r.FindTickets("acme", store.TicketFilter{Parts: store.History})
			if len(whole) != 1 || !maps.Equal(partsOf(whole[0]), partsOf(historyTicket())) {
				t.Fatalf("a whole read has parts %v", partsOf(whole[0]))
			}
		})
	})

	t.Run("specs are found by Label, Tag and Link, in number order", func(t *testing.T) {
		s := f.New(t)
		seedProject(t, s)
		for _, x := range []domain.Spec{
			{Number: 3, Labels: []string{"map"}, Tags: map[string]string{"source": "gh-1"}, Link: domain.Link{Integration: "github-issues", ID: "12"}},
			{Number: 1, Labels: []string{"map", "auto-accept"}, Tags: map[string]string{"source": "gh-2"}},
			{Number: 2, Link: domain.Link{Integration: "github-issues", ID: "13", Detached: true}},
			{Number: 4, Tags: map[string]string{"area": "web"}, Link: domain.Link{Integration: "azure-boards", ID: "12"}},
		} {
			x.Title, x.Created, x.Updated = "S", t0, t0
			put(t, s, func(w store.Writer) error { return w.PutSpec("acme", x) })
		}
		specNumbers := func(specs []domain.Spec) []int {
			out := []int{}
			for _, sp := range specs {
				out = append(out, sp.Number)
			}
			return out
		}
		read(t, s, func(r store.Reader) {
			for _, c := range []struct {
				name string
				f    store.SpecFilter
				want []int
			}{
				{"every Spec", store.SpecFilter{}, []int{1, 2, 3, 4}},
				{"Label", store.SpecFilter{Label: "map"}, []int{1, 3}},
				{"a Label none carries", store.SpecFilter{Label: "ui"}, []int{}},
				{"Tag with one value", store.SpecFilter{Tag: &store.TagFilter{Key: "source", Values: []string{"gh-1"}}}, []int{3}},
				{"Tag with several values", store.SpecFilter{Tag: &store.TagFilter{Key: "source", Values: []string{"gh-1", "gh-2"}}}, []int{1, 3}},
				{"Tag with any value", store.SpecFilter{Tag: &store.TagFilter{Key: "area"}}, []int{4}},
				{"Link to an item", store.SpecFilter{Link: &store.LinkFilter{Integration: "github-issues", ID: "12"}}, []int{3}},
				{"Link to any item, detached or not", store.SpecFilter{Link: &store.LinkFilter{Integration: "github-issues"}}, []int{2, 3}},
				{"fields combined", store.SpecFilter{Label: "map", Link: &store.LinkFilter{Integration: "github-issues"}}, []int{3}},
			} {
				if got := specNumbers(r.FindSpecs("acme", c.f)); !slices.Equal(got, c.want) {
					t.Errorf("%s: found %v, want %v", c.name, got, c.want)
				}
			}
			if got := r.FindSpecs("other", store.SpecFilter{}); len(got) != 0 {
				t.Errorf("an unknown project has Specs %v", specNumbers(got))
			}
		})
	})

	t.Run("state counts count the tickets in each State", func(t *testing.T) {
		s := f.New(t)
		seedProject(t, s)
		read(t, s, func(r store.Reader) {
			if got := r.StateCounts("acme"); len(got) != 0 {
				t.Errorf("a project with no Tickets counts %v", got)
			}
		})
		for n, state := range []domain.State{domain.Ready, domain.Ready, domain.InReview, domain.Done, domain.Ready} {
			put(t, s, func(w store.Writer) error {
				return w.PutTicket("acme", domain.Ticket{Number: n + 1, Title: "T", State: state, Spec: 1, Created: t0, Updated: t0})
			})
		}
		// A Ticket moving leaves its old State's count.
		put(t, s, func(w store.Writer) error {
			return w.PutTicket("acme", domain.Ticket{Number: 3, Title: "T", State: domain.Accepted, Spec: 1, Created: t0, Updated: t0})
		})
		read(t, s, func(r store.Reader) {
			want := map[domain.State]int{domain.Ready: 3, domain.Accepted: 1, domain.Done: 1}
			if got := r.StateCounts("acme"); !maps.Equal(got, want) {
				t.Errorf("counts %v, want %v", got, want)
			}
			if got := r.StateCounts("other"); len(got) != 0 {
				t.Errorf("an unknown project counts %v", got)
			}
		})
	})
}

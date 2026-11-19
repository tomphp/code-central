package storetest

import (
	"fmt"
	"maps"
	"slices"
	"testing"

	"github.com/tomoram/code-central/domain"
	"github.com/tomoram/code-central/store"
)

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

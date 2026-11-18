package storetest

import (
	"github.com/tomoram/code-central/domain"
	"reflect"
	"testing"

	"github.com/tomoram/code-central/store"
)

// runLinks checks that a Spec's and a Ticket's Link read back, with the
// keys of the comments written for it, and that an unlinked item reads
// back with no Link.
func runLinks(t *testing.T, f Factory) {
	t.Run("links read back after they are written", func(t *testing.T) {
		s := f.New(t)
		seedProject(t, s)
		specLink := domain.Link{Integration: "github", ID: "acme/web#12", URL: "https://github.com/acme/web/issues/12", ExternalState: domain.ExternalOpen, Outbound: true, Written: []string{"context", "grilling"}}
		ticketLink := domain.Link{Integration: "github", ID: "acme/web#13", ExternalState: domain.ExternalClosed, Detached: true}
		put(t, s, func(w store.Writer) error {
			return w.PutSpec("acme", domain.Spec{Number: 1, Title: "S", Link: specLink, Created: t0, Updated: t0})
		})
		put(t, s, func(w store.Writer) error {
			return w.PutTicket("acme", domain.Ticket{Number: 2, Title: "T", State: domain.Draft, Spec: 1, Link: ticketLink, Created: t0, Updated: t0})
		})
		put(t, s, func(w store.Writer) error {
			return w.PutTicket("acme", domain.Ticket{Number: 3, Title: "U", State: domain.Draft, Spec: 1, Created: t0, Updated: t0})
		})
		check := func(s store.Store) {
			read(t, s, func(r store.Reader) {
				if sp, _ := r.Spec("acme", 1); !reflect.DeepEqual(sp.Link, specLink) {
					t.Fatalf("spec link: got %+v", sp.Link)
				}
				if tk, _ := r.Ticket("acme", 2); !reflect.DeepEqual(tk.Link, ticketLink) {
					t.Fatalf("ticket link: got %+v", tk.Link)
				}
				if tk, _ := r.Ticket("acme", 3); tk.Link.Linked() {
					t.Fatalf("unlinked ticket has a link: %+v", tk.Link)
				}
			})
		}
		check(s)
		if f.Reopen != nil {
			check(f.Reopen(t, s))
		}
	})
}

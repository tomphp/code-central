package storetest

import (
	"testing"

	"github.com/tomoram/code-central/store"
)

// runLinks checks that a Spec's and a Ticket's Link read back, and that
// an unlinked item reads back with no Link.
func runLinks(t *testing.T, f Factory) {
	t.Run("links read back after they are written", func(t *testing.T) {
		s := f.New(t)
		seedProject(t, s)
		specLink := store.Link{Integration: "github", ID: "acme/web#12", URL: "https://github.com/acme/web/issues/12", ExternalState: store.ExternalOpen}
		ticketLink := store.Link{Integration: "github", ID: "acme/web#13", ExternalState: store.ExternalClosed, Detached: true}
		put(t, s, func(w store.Writer) error {
			return w.PutSpec("acme", store.Spec{Number: 1, Title: "S", Link: specLink, Created: t0, Updated: t0})
		})
		put(t, s, func(w store.Writer) error {
			return w.PutTicket("acme", store.Ticket{Number: 2, Title: "T", State: store.Draft, Spec: 1, Link: ticketLink, Created: t0, Updated: t0})
		})
		put(t, s, func(w store.Writer) error {
			return w.PutTicket("acme", store.Ticket{Number: 3, Title: "U", State: store.Draft, Spec: 1, Created: t0, Updated: t0})
		})
		check := func(s store.Store) {
			read(t, s, func(r store.Reader) {
				if sp, _ := r.Spec("acme", 1); sp.Link != specLink {
					t.Fatalf("spec link: got %+v", sp.Link)
				}
				if tk, _ := r.Ticket("acme", 2); tk.Link != ticketLink {
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

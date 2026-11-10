package storetest

import (
	"testing"

	"github.com/tomoram/code-central/store"
)

// runImport checks Import, the one bulk write: every record in it lands
// together, or none does.
func runImport(t *testing.T, f Factory) {
	t.Run("an import writes every record in it at once", func(t *testing.T) {
		s := f.New(t)
		seedProject(t, s)
		put(t, s, func(w store.Writer) error {
			return w.PutSpec("acme", store.Spec{Number: 1, Title: "Existing", Created: t0, Updated: t0})
		})
		put(t, s, func(w store.Writer) error { return w.PutImport("acme", ImportBatch()) })
		check := func(s store.Store) {
			read(t, s, func(r store.Reader) {
				if n := len(r.Specs("acme")); n != 3 {
					t.Fatalf("%d specs, want 3", n)
				}
				if n := len(r.Tickets("acme")); n != 3 {
					t.Fatalf("%d tickets, want 3", n)
				}
				sp, _ := r.Spec("acme", 2)
				if sp.Title != "Login" || sp.Body != "Users log in.\n" || sp.Tags["source"] != ".scratch/login/spec.md" {
					t.Fatalf("spec: %+v", sp)
				}
				tk, _ := r.Ticket("acme", 3)
				if tk.State != store.Done || len(tk.Moves) != 1 || tk.Moves[0] != ImportBatch().Tickets[0].Moves[0] {
					t.Fatalf("ticket 3: %+v", tk)
				}
				if tk, _ := r.Ticket("acme", 6); len(tk.Comments) != 1 || len(tk.Replacements) != 1 || tk.Replacements[0] != 4 {
					t.Fatalf("ticket 6: %+v", tk)
				}
				if n := r.NextNumber("acme"); n != 7 {
					t.Fatalf("NextNumber = %d, want 7", n)
				}
			})
		}
		check(s)
		if f.Reopen != nil {
			check(f.Reopen(t, s))
		}
	})

	t.Run("an import is the update's one write", func(t *testing.T) {
		s := f.New(t)
		seedProject(t, s)
		err := s.Update(func(w store.Writer) error {
			if err := w.PutSpec("acme", store.Spec{Number: 1, Title: "S", Created: t0, Updated: t0}); err != nil {
				return err
			}
			return w.PutImport("acme", ImportBatch())
		})
		if err == nil {
			t.Fatal("an import after another write in one update was allowed")
		}
		read(t, s, func(r store.Reader) {
			if len(r.Tickets("acme")) != 0 {
				t.Fatal("the refused import wrote tickets")
			}
		})
	})

	t.Run("an import that would overwrite a record writes nothing", func(t *testing.T) {
		s := f.New(t)
		seedProject(t, s)
		put(t, s, func(w store.Writer) error {
			return w.PutTicket("acme", store.Ticket{Number: 4, Title: "Old", State: store.Draft, Spec: 1, Created: t0, Updated: t0})
		})
		if err := s.Update(func(w store.Writer) error { return w.PutImport("acme", ImportBatch()) }); err == nil {
			t.Fatal("an import over an existing number was allowed")
		}
		check := func(s store.Store) {
			read(t, s, func(r store.Reader) {
				if len(r.Specs("acme")) != 0 || len(r.Tickets("acme")) != 1 {
					t.Fatalf("partial import: %d specs, %d tickets", len(r.Specs("acme")), len(r.Tickets("acme")))
				}
				if tk, _ := r.Ticket("acme", 4); tk.Title != "Old" {
					t.Fatalf("existing ticket overwritten: %+v", tk)
				}
			})
		}
		check(s)
		if f.Reopen != nil {
			check(f.Reopen(t, s))
		}
	})

	t.Run("an import rewrites the existing Tickets it names along with its new records", func(t *testing.T) {
		s := f.New(t)
		seedProject(t, s)
		put(t, s, func(w store.Writer) error {
			return w.PutTicket("acme", store.Ticket{Number: 1, Title: "Old", State: store.Draft, Spec: 9, Created: t0, Updated: t0})
		})
		im := ImportBatch()
		im.Rewritten = []store.Ticket{{Number: 1, Title: "Old", State: store.Cancelled, Spec: 9, Replacements: []int{3}, Created: t0, Updated: t0}}
		put(t, s, func(w store.Writer) error { return w.PutImport("acme", im) })
		check := func(s store.Store) {
			read(t, s, func(r store.Reader) {
				if tk, _ := r.Ticket("acme", 1); tk.State != store.Cancelled || len(tk.Replacements) != 1 {
					t.Fatalf("ticket 1: %+v, want it rewritten", tk)
				}
				if n := len(r.Tickets("acme")); n != 4 {
					t.Fatalf("%d tickets, want 4", n)
				}
			})
		}
		check(s)
		if f.Reopen != nil {
			check(f.Reopen(t, s))
		}
	})

	t.Run("an import that would rewrite a missing Ticket writes nothing", func(t *testing.T) {
		s := f.New(t)
		seedProject(t, s)
		im := ImportBatch()
		im.Rewritten = []store.Ticket{{Number: 1, Title: "Missing", State: store.Draft, Spec: 9, Created: t0, Updated: t0}}
		if err := s.Update(func(w store.Writer) error { return w.PutImport("acme", im) }); err == nil {
			t.Fatal("an import rewriting a missing Ticket was allowed")
		}
		read(t, s, func(r store.Reader) {
			if len(r.Specs("acme")) != 0 || len(r.Tickets("acme")) != 0 {
				t.Fatal("the refused import wrote records")
			}
		})
	})

	t.Run("an import into an unknown project writes nothing", func(t *testing.T) {
		s := f.New(t)
		seedProject(t, s)
		if err := s.Update(func(w store.Writer) error { return w.PutImport("nope", ImportBatch()) }); err == nil {
			t.Fatal("an import into an unknown project succeeded")
		}
	})
}

// ImportBatch is a small Import into Project acme: Specs 2 and 5, and
// Tickets 3 to 6 across them. The file store's crash tests use it too.
func ImportBatch() store.Import {
	return store.Import{
		Specs: []store.Spec{
			{Number: 2, Title: "Login", Tags: map[string]string{"source": ".scratch/login/spec.md"}, Created: t0, Updated: t0, Body: "Users log in.\n"},
			{Number: 5, Title: "Billing", Created: t0, Updated: t0},
		},
		Tickets: []store.Ticket{
			{Number: 3, Title: "Form", State: store.Done, Spec: 2, Created: t0, Updated: t0,
				Moves: []store.Move{{To: store.Done, Actor: "agent", Time: t0, Via: "import"}}},
			{Number: 4, Title: "API", State: store.Ready, Spec: 2, BlockedBy: []int{3}, Created: t0, Updated: t0,
				Moves: []store.Move{{To: store.Ready, Actor: "agent", Time: t0, Via: "import"}}},
			{Number: 6, Title: "Invoices", State: store.Cancelled, Spec: 5, Replacements: []int{4}, Created: t0, Updated: t0,
				Comments: []store.Comment{{Number: 1, Actor: "agent", Time: t0, Body: "wontfix\n"}}},
		},
	}
}

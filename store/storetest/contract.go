// Package storetest holds the contract suite every store.Store
// implementation must pass. Run it from each implementation's tests with
// a factory that returns a fresh, empty store.
package storetest

import (
	"slices"
	"testing"
	"time"

	"github.com/tomoram/code-central/store"
)

// Factory returns a fresh, empty store. Reopen, when non-nil, closes the
// given store and opens it again from wherever it persists, so the suite
// can check that writes survive a restart.
type Factory struct {
	New    func(t *testing.T) store.Store
	Reopen func(t *testing.T, s store.Store) store.Store
}

var t0 = time.Date(2026, 11, 7, 9, 30, 0, 0, time.UTC)

// Run runs the contract suite.
func Run(t *testing.T, f Factory) {
	t.Run("a project reads back after it is written", func(t *testing.T) {
		s := f.New(t)
		put(t, s, func(w store.Writer) error {
			return w.PutProject(store.Project{Key: "acme", DisplayName: "Acme", Created: t0, Mode: "committed"})
		})
		read(t, s, func(r store.Reader) {
			p, ok := r.Project("acme")
			if !ok {
				t.Fatal("project not found")
			}
			if p.DisplayName != "Acme" || !p.Created.Equal(t0) || p.Mode != "committed" {
				t.Fatalf("got %+v", p)
			}
			if _, ok := r.Project("other"); ok {
				t.Fatal("unknown project found")
			}
			if got := len(r.Projects()); got != 1 {
				t.Fatalf("Projects() has %d entries, want 1", got)
			}
		})
	})

	t.Run("specs and tickets read back after they are written", func(t *testing.T) {
		s := f.New(t)
		seedProject(t, s)
		spec := store.Spec{Number: 1, Title: "Login", Labels: []string{"auth"}, Tags: map[string]string{"area": "web"}, Created: t0, Updated: t0, Body: "# Login\n\nUsers log in.\n"}
		ticket := store.Ticket{Number: 2, Title: "Form", State: store.Ready, Spec: 1, Labels: []string{"ui"}, Created: t0, Updated: t0.Add(time.Minute), Description: "Build the form.\n"}
		put(t, s, func(w store.Writer) error { return w.PutSpec("acme", spec) })
		put(t, s, func(w store.Writer) error { return w.PutTicket("acme", ticket) })
		check := func(s store.Store) {
			read(t, s, func(r store.Reader) {
				gs, ok := r.Spec("acme", 1)
				if !ok || gs.Title != "Login" || gs.Body != spec.Body || gs.Labels[0] != "auth" || gs.Tags["area"] != "web" || !gs.Created.Equal(t0) {
					t.Fatalf("spec: got %+v", gs)
				}
				gt, ok := r.Ticket("acme", 2)
				if !ok || gt.Title != "Form" || gt.State != store.Ready || gt.Spec != 1 || gt.Description != ticket.Description || !gt.Updated.Equal(ticket.Updated) || gt.Labels[0] != "ui" {
					t.Fatalf("ticket: got %+v", gt)
				}
				if len(r.Specs("acme")) != 1 || len(r.Tickets("acme")) != 1 {
					t.Fatal("wrong list lengths")
				}
				if _, ok := r.Ticket("acme", 1); ok {
					t.Fatal("spec number found as a ticket")
				}
			})
		}
		check(s)
		if f.Reopen != nil {
			check(f.Reopen(t, s))
		}
	})

	t.Run("a ticket's claims, comments and moves read back after they are written", func(t *testing.T) {
		s := f.New(t)
		seedProject(t, s)
		tk := store.Ticket{
			Number: 1, Title: "T", State: store.InReview, Spec: 1, Created: t0, Updated: t0,
			Claims: []store.Claim{
				{Number: 1, ID: "ABC", SessionID: "sess-1", Started: t0, Ended: t0.Add(time.Hour), EndedBy: "finish"},
				{Number: 2, ID: "DEF", Started: t0.Add(2 * time.Hour)},
			},
			Comments: []store.Comment{{Number: 1, Actor: "agent", Time: t0.Add(time.Hour), Claim: 1, Body: "Did it.\n"}},
			Moves:    []store.Move{{From: store.Ready, To: store.InProgress, Actor: "agent", Time: t0, Claim: 1}},
		}
		put(t, s, func(w store.Writer) error { return w.PutTicket("acme", tk) })
		check := func(s store.Store) {
			read(t, s, func(r store.Reader) {
				got, _ := r.Ticket("acme", 1)
				if len(got.Claims) != 2 || got.Claims[0] != tk.Claims[0] || got.Claims[1] != tk.Claims[1] {
					t.Fatalf("claims: got %+v", got.Claims)
				}
				if len(got.Comments) != 1 || got.Comments[0] != tk.Comments[0] {
					t.Fatalf("comments: got %+v", got.Comments)
				}
				if len(got.Moves) != 1 || got.Moves[0] != tk.Moves[0] {
					t.Fatalf("moves: got %+v", got.Moves)
				}
			})
		}
		check(s)
		if f.Reopen != nil {
			check(f.Reopen(t, s))
		}
	})

	t.Run("a ticket's rounds read back after they are written, and reads are copies", func(t *testing.T) {
		s := f.New(t)
		seedProject(t, s)
		tk := store.Ticket{
			Number: 1, Title: "T", State: store.NeedsAnswer, Spec: 1, Created: t0, Updated: t0,
			Rounds: []store.Round{{Number: 1, Claim: 1, SessionID: "sess-1", Posted: t0, Questions: []store.Question{
				{Number: 1, Title: "Which DB?", Body: "Postgres or SQLite?\n", Recommended: "SQLite\n", Status: store.QuestionAnswered, Answer: "SQLite\n", Closed: t0.Add(time.Minute)},
				{Number: 2, Title: "Name?", Body: "What name?", Status: store.QuestionOpen},
				{Number: 3, Title: "Skip?", Body: "Skip it?", Status: store.QuestionDismissed, Closed: t0.Add(time.Hour)},
			}}},
		}
		put(t, s, func(w store.Writer) error { return w.PutTicket("acme", tk) })
		check := func(s store.Store) {
			read(t, s, func(r store.Reader) {
				got, _ := r.Ticket("acme", 1)
				if len(got.Rounds) != 1 {
					t.Fatalf("rounds: got %+v", got.Rounds)
				}
				g, w := got.Rounds[0], tk.Rounds[0]
				if g.Number != w.Number || g.Claim != w.Claim || g.SessionID != w.SessionID || !g.Posted.Equal(w.Posted) || !slices.Equal(g.Questions, w.Questions) {
					t.Fatalf("round: got %+v, want %+v", g, w)
				}
				got.Rounds[0].Questions[1].Status = store.QuestionAnswered
			})
			read(t, s, func(r store.Reader) {
				if again, _ := r.Ticket("acme", 1); again.Rounds[0].Questions[1].Status != store.QuestionOpen {
					t.Fatal("changing a read Ticket's Questions changed the store")
				}
			})
		}
		check(s)
		if f.Reopen != nil {
			check(f.Reopen(t, s))
		}
	})

	t.Run("comments and edits read back after they are written", func(t *testing.T) {
		s := f.New(t)
		seedProject(t, s)
		spec := store.Spec{Number: 1, Title: "Login v2", Created: t0, Updated: t0, Body: "New body.\n",
			Edits: []store.Edit{{Number: 1, Actor: "agent", Time: t0, Previous: map[string]string{"title": "Login", "body": "# Old\n\nOld body.\n"}}}}
		ticket := store.Ticket{Number: 2, Title: "Form", State: store.Draft, Spec: 1, Created: t0, Updated: t0,
			Comments: []store.Comment{{Number: 1, Actor: "human", Time: t0, Body: "Looks good.\nShip it.\n"}},
			Edits:    []store.Edit{{Number: 1, Actor: "agent", Time: t0, Previous: map[string]string{"description": ""}}}}
		put(t, s, func(w store.Writer) error { return w.PutSpec("acme", spec) })
		put(t, s, func(w store.Writer) error { return w.PutTicket("acme", ticket) })
		check := func(s store.Store) {
			read(t, s, func(r store.Reader) {
				gs, _ := r.Spec("acme", 1)
				if gs.Body != spec.Body || len(gs.Edits) != 1 || gs.Edits[0].Previous["body"] != "# Old\n\nOld body.\n" ||
					gs.Edits[0].Previous["title"] != "Login" || gs.Edits[0].Actor != "agent" || !gs.Edits[0].Time.Equal(t0) {
					t.Fatalf("spec: got %+v", gs)
				}
				gt, _ := r.Ticket("acme", 2)
				if len(gt.Comments) != 1 || gt.Comments[0] != ticket.Comments[0] {
					t.Fatalf("comments: got %+v", gt.Comments)
				}
				prev, ok := gt.Edits[0].Previous["description"]
				if len(gt.Edits) != 1 || gt.Edits[0].Number != 1 || !ok || prev != "" {
					t.Fatalf("edits: got %+v", gt.Edits)
				}
			})
		}
		check(s)
		if f.Reopen != nil {
			check(f.Reopen(t, s))
		}
	})

	t.Run("a ticket write replaces the previous version", func(t *testing.T) {
		s := f.New(t)
		seedProject(t, s)
		tk := store.Ticket{Number: 1, Title: "One", State: store.Draft, Spec: 1, Created: t0, Updated: t0}
		put(t, s, func(w store.Writer) error { return w.PutTicket("acme", tk) })
		tk.Title = "Two"
		put(t, s, func(w store.Writer) error { return w.PutTicket("acme", tk) })
		read(t, s, func(r store.Reader) {
			if got, _ := r.Ticket("acme", 1); got.Title != "Two" {
				t.Fatalf("title %q", got.Title)
			}
		})
	})

	t.Run("records read back are copies", func(t *testing.T) {
		s := f.New(t)
		seedProject(t, s)
		put(t, s, func(w store.Writer) error {
			return w.PutTicket("acme", store.Ticket{Number: 1, Title: "T", State: store.Draft, Spec: 1, Labels: []string{"a"}, Created: t0, Updated: t0})
		})
		read(t, s, func(r store.Reader) {
			got, _ := r.Ticket("acme", 1)
			got.Labels[0] = "changed"
		})
		read(t, s, func(r store.Reader) {
			if got, _ := r.Ticket("acme", 1); got.Labels[0] != "a" {
				t.Fatal("mutating a read changed the store")
			}
		})
	})

	t.Run("the ID counter is the highest number in the project plus one", func(t *testing.T) {
		s := f.New(t)
		seedProject(t, s)
		read(t, s, func(r store.Reader) {
			if n := r.NextNumber("acme"); n != 1 {
				t.Fatalf("empty project: NextNumber = %d, want 1", n)
			}
		})
		put(t, s, func(w store.Writer) error {
			return w.PutSpec("acme", store.Spec{Number: 4, Title: "S", Created: t0, Updated: t0})
		})
		put(t, s, func(w store.Writer) error {
			return w.PutTicket("acme", store.Ticket{Number: 2, Title: "T", State: store.Draft, Spec: 4, Created: t0, Updated: t0})
		})
		check := func(s store.Store) {
			read(t, s, func(r store.Reader) {
				if n := r.NextNumber("acme"); n != 5 {
					t.Fatalf("NextNumber = %d, want 5", n)
				}
			})
		}
		check(s)
		if f.Reopen != nil {
			check(f.Reopen(t, s))
		}
	})

	t.Run("an update writes at most one record", func(t *testing.T) {
		s := f.New(t)
		seedProject(t, s)
		err := s.Update(func(w store.Writer) error {
			if err := w.PutSpec("acme", store.Spec{Number: 1, Title: "S", Created: t0, Updated: t0}); err != nil {
				return err
			}
			return w.PutSpec("acme", store.Spec{Number: 2, Title: "S", Created: t0, Updated: t0})
		})
		if err == nil {
			t.Fatal("second write in one update was allowed")
		}
	})

	t.Run("a failed update changes nothing", func(t *testing.T) {
		s := f.New(t)
		seedProject(t, s)
		err := s.Update(func(w store.Writer) error {
			return w.PutTicket("nope", store.Ticket{Number: 1, Title: "T", State: store.Draft, Spec: 1, Created: t0, Updated: t0})
		})
		if err == nil {
			t.Fatal("write to an unknown project succeeded")
		}
	})
}

func seedProject(t *testing.T, s store.Store) {
	t.Helper()
	put(t, s, func(w store.Writer) error {
		return w.PutProject(store.Project{Key: "acme", DisplayName: "acme", Created: t0, Mode: "committed"})
	})
}

func put(t *testing.T, s store.Store, fn func(store.Writer) error) {
	t.Helper()
	if err := s.Update(fn); err != nil {
		t.Fatalf("update: %v", err)
	}
}

func read(t *testing.T, s store.Store, fn func(store.Reader)) {
	t.Helper()
	if err := s.View(func(r store.Reader) error { fn(r); return nil }); err != nil {
		t.Fatalf("view: %v", err)
	}
}

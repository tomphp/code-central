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

// Ticket History by part (Spec code-central-1032): a read returns only
// the parts it names, a write keeps the parts it doesn't name, and every
// write keeps the Ticket's derived head fields.

// historyTicket is a Ticket with every History part filled in: claimed
// twice, its first Claim ended by a Failure Report and back to Ready, and
// its second live after a Round.
func historyTicket() domain.Ticket {
	return domain.Ticket{
		Number: 1, Title: "T", State: domain.InProgress, Spec: 1, Created: t0, Updated: t0.Add(4 * time.Hour),
		Claims: []domain.Claim{
			{Number: 1, ID: "ABC", Started: t0, Ended: t0.Add(time.Hour), EndedBy: "report_failure"},
			{Number: 2, ID: "DEF", Started: t0.Add(2 * time.Hour)},
		},
		Rounds: []domain.Round{
			{Number: 1, Claim: 2, Posted: t0.Add(2 * time.Hour), Questions: []domain.Question{{Number: 1, Title: "Q", Body: "Q?", Status: domain.QuestionAnswered, Answer: "A", Closed: t0.Add(3 * time.Hour)}}},
		},
		FailureReports: []domain.FailureReport{{Number: 1, Claim: 1, Actor: "agent", Time: t0.Add(time.Hour), WhatBroke: "Broke.\n"}},
		Comments:       []domain.Comment{{Number: 1, Actor: "human", Time: t0, Body: "Go.\n"}},
		Moves: []domain.Move{
			{From: domain.Ready, To: domain.InProgress, Actor: "agent", Time: t0, Claim: 1},
			{From: domain.InProgress, To: domain.Failed, Actor: "agent", Time: t0.Add(time.Hour), Claim: 1},
			{From: domain.Failed, To: domain.Ready, Actor: "human", Time: t0.Add(90 * time.Minute)},
			{From: domain.Ready, To: domain.InProgress, Actor: "agent", Time: t0.Add(2 * time.Hour), Claim: 2},
			{From: domain.InProgress, To: domain.NeedsAnswer, Actor: "agent", Time: t0.Add(150 * time.Minute), Claim: 2},
			{From: domain.NeedsAnswer, To: domain.InProgress, Actor: "human", Time: t0.Add(3 * time.Hour), Claim: 2},
		},
		Edits: []domain.Edit{
			{Number: 1, Actor: "agent", Time: t0, Previous: map[string]string{"title": "Old"}},
			{Number: 2, Actor: "human", Time: t0.Add(30 * time.Minute), Previous: map[string]string{"external_state": "open"}},
			{Number: 3, Actor: "agent", Time: t0.Add(4 * time.Hour), Previous: map[string]string{"title": "T0"}},
		},
	}
}

// historyHead is historyTicket's derived head fields.
func historyHead() domain.TicketHead {
	tk := historyTicket()
	return domain.TicketHead{
		Entered: t0.Add(3 * time.Hour), LiveClaim: &tk.Claims[1], LatestRound: &tk.Rounds[0],
		LatestFailureReport: &tk.FailureReports[0], BackToReady: t0.Add(90 * time.Minute),
		ExternalStateChanged: t0.Add(30 * time.Minute),
	}
}

// partsOf counts each History part t carries.
func partsOf(t domain.Ticket) map[store.Part]int {
	return map[store.Part]int{
		store.Claims: len(t.Claims), store.Rounds: len(t.Rounds), store.Comments: len(t.Comments),
		store.Moves: len(t.Moves), store.Edits: len(t.Edits), store.FailureReports: len(t.FailureReports),
	}
}

// only is the counts of a Ticket carrying n of part and nothing else.
func only(part store.Part, n int) map[store.Part]int {
	out := partsOf(domain.Ticket{})
	out[part] = n
	return out
}

func runHistory(t *testing.T, f Factory) {
	whole := partsOf(historyTicket())

	t.Run("a ticket read carries only the History parts it names", func(t *testing.T) {
		s := f.New(t)
		seedProject(t, s)
		put(t, s, func(w store.Writer) error { return w.PutTicket("acme", historyTicket(), store.History...) })
		check := func(s store.Store) {
			read(t, s, func(r store.Reader) {
				for _, part := range store.History {
					got, _ := r.Ticket("acme", 1, part)
					if want := only(part, whole[part]); !maps.Equal(partsOf(got), want) {
						t.Errorf("read naming %s: got %v, want %v", part, partsOf(got), want)
					}
				}
				head, _ := r.Ticket("acme", 1)
				if head.Title != "T" || head.State != domain.InProgress {
					t.Fatalf("head: got %+v", head)
				}
				if got := partsOf(head); !maps.Equal(got, partsOf(domain.Ticket{})) {
					t.Errorf("read naming no part: got %v", got)
				}
				listed := r.FindTickets("acme", store.TicketFilter{Parts: []store.Part{store.Moves}})
				if len(listed) != 1 || !maps.Equal(partsOf(listed[0]), only(store.Moves, whole[store.Moves])) {
					t.Errorf("FindTickets naming moves: got %+v", listed)
				}
				all, _ := r.Ticket("acme", 1, store.History...)
				if got := partsOf(all); !maps.Equal(got, whole) {
					t.Errorf("read naming every part: got %v, want %v", got, whole)
				}
			})
		}
		check(s)
		if f.Reopen != nil {
			check(f.Reopen(t, s))
		}
	})

	t.Run("a write after a narrow read keeps the parts it doesn't name", func(t *testing.T) {
		s := f.New(t)
		seedProject(t, s)
		put(t, s, func(w store.Writer) error { return w.PutTicket("acme", historyTicket(), store.History...) })
		put(t, s, func(w store.Writer) error {
			tk, _ := w.Ticket("acme", 1, store.Comments)
			tk.Title = "Retitled"
			tk.Comments = append(tk.Comments, domain.Comment{Number: 2, Actor: "agent", Time: t0.Add(5 * time.Hour), Body: "More.\n"})
			return w.PutTicket("acme", tk, store.Comments)
		})
		put(t, s, func(w store.Writer) error {
			tk, _ := w.Ticket("acme", 1)
			tk.Description = "Head only.\n"
			return w.PutTicket("acme", tk)
		})
		check := func(s store.Store) {
			read(t, s, func(r store.Reader) {
				got, _ := r.Ticket("acme", 1, store.History...)
				want := maps.Clone(whole)
				want[store.Comments] = 2
				if got.Title != "Retitled" || got.Description != "Head only.\n" || !maps.Equal(partsOf(got), want) {
					t.Fatalf("got %q %q with %v, want %v", got.Title, got.Description, partsOf(got), want)
				}
				if got.Comments[1].Body != "More.\n" || !slices.Equal(got.Moves, historyTicket().Moves) {
					t.Fatalf("history: got %+v", got)
				}
			})
		}
		check(s)
		if f.Reopen != nil {
			check(f.Reopen(t, s))
		}
	})

	t.Run("a new ticket keeps only the History parts its write names", func(t *testing.T) {
		s := f.New(t)
		seedProject(t, s)
		put(t, s, func(w store.Writer) error { return w.PutTicket("acme", historyTicket(), store.Moves) })
		read(t, s, func(r store.Reader) {
			got, _ := r.Ticket("acme", 1, store.History...)
			if want := only(store.Moves, whole[store.Moves]); !maps.Equal(partsOf(got), want) {
				t.Fatalf("got %v, want %v", partsOf(got), want)
			}
		})
	})

	t.Run("a ticket's derived head fields come from its whole History", func(t *testing.T) {
		s := f.New(t)
		seedProject(t, s)
		tk := historyTicket()
		// What a writer sets in the head is ignored.
		tk.Head = domain.TicketHead{Entered: t0.Add(time.Minute), LiveClaim: &tk.Claims[0]}
		put(t, s, func(w store.Writer) error { return w.PutTicket("acme", tk, store.History...) })
		want := historyHead()
		s = checkHead(t, f, s, "after a whole write", want)

		// A head-only write keeps the fields its History gives.
		put(t, s, func(w store.Writer) error {
			got, _ := w.Ticket("acme", 1)
			got.Title = "Head only"
			return w.PutTicket("acme", got)
		})
		s = checkHead(t, f, s, "after a head-only write", want)

		// A move to In Review ending the Claim, by a write naming only the
		// parts it changes.
		at := t0.Add(5 * time.Hour)
		put(t, s, func(w store.Writer) error {
			got, _ := w.Ticket("acme", 1, store.Claims, store.Moves)
			got.State = domain.InReview
			got.Claims[1].Ended, got.Claims[1].EndedBy = at, "finish"
			got.Moves = append(got.Moves, domain.Move{From: domain.InProgress, To: domain.InReview, Actor: "agent", Time: at, Claim: 2})
			return w.PutTicket("acme", got, store.Claims, store.Moves)
		})
		want.Entered, want.LiveClaim = at, nil
		s = checkHead(t, f, s, "after finishing", want)

		// A second Round and Failure Report, and a move back to Ready.
		round := domain.Round{Number: 2, Claim: 2, Posted: at, Questions: []domain.Question{{Number: 1, Title: "R", Body: "R?", Status: domain.QuestionOpen}}}
		report := domain.FailureReport{Number: 2, Claim: 2, Actor: "agent", Time: at, WhatBroke: "Again.\n"}
		put(t, s, func(w store.Writer) error {
			got, _ := w.Ticket("acme", 1, store.Moves, store.Rounds, store.FailureReports)
			got.State = domain.Ready
			got.Moves = append(got.Moves, domain.Move{From: domain.InReview, To: domain.Ready, Actor: "human", Time: at.Add(time.Hour)})
			got.Rounds = append(got.Rounds, round)
			got.FailureReports = append(got.FailureReports, report)
			return w.PutTicket("acme", got, store.Moves, store.Rounds, store.FailureReports)
		})
		want.Entered, want.BackToReady = at.Add(time.Hour), at.Add(time.Hour)
		want.LatestRound, want.LatestFailureReport = &round, &report
		checkHead(t, f, s, "after a move back to Ready", want)
	})

	t.Run("a new ticket with no History has an empty head", func(t *testing.T) {
		s := f.New(t)
		seedProject(t, s)
		put(t, s, func(w store.Writer) error {
			return w.PutTicket("acme", domain.Ticket{Number: 1, Title: "T", State: domain.Ready, Spec: 1, Created: t0, Updated: t0})
		})
		checkHead(t, f, s, "a new Ticket", domain.TicketHead{})
	})

	t.Run("an import writes each ticket whole with its head", func(t *testing.T) {
		s := f.New(t)
		seedProject(t, s)
		tk := historyTicket()
		put(t, s, func(w store.Writer) error {
			return w.PutImport("acme", store.Import{Specs: []domain.Spec{{Number: 2, Title: "S", Created: t0, Updated: t0}}, Tickets: []domain.Ticket{tk}})
		})
		read(t, s, func(r store.Reader) {
			if got, _ := r.Ticket("acme", 1, store.History...); !maps.Equal(partsOf(got), whole) {
				t.Fatalf("imported: got %v, want %v", partsOf(got), whole)
			}
		})
		s = checkHead(t, f, s, "after an import", historyHead())

		// A rewrite in an import is whole too: its History is what it holds.
		tk.Comments, tk.State = nil, domain.Ready
		tk.Claims[1].Ended = t0.Add(5 * time.Hour)
		put(t, s, func(w store.Writer) error { return w.PutImport("acme", store.Import{Rewritten: []domain.Ticket{tk}}) })
		read(t, s, func(r store.Reader) {
			if got, _ := r.Ticket("acme", 1, store.History...); len(got.Comments) != 0 {
				t.Fatalf("rewritten: got %v", partsOf(got))
			}
		})
		want := historyHead()
		want.Entered, want.LiveClaim = t0.Add(90*time.Minute), nil
		checkHead(t, f, s, "after an import's rewrite", want)
	})
}

// checkHead checks Ticket acme-1's derived head fields, read with no
// History both alone and from Tickets, and again after a reopen. It
// returns s, reopened if the Kind can.
func checkHead(t *testing.T, f Factory, s store.Store, when string, want domain.TicketHead) store.Store {
	t.Helper()
	check := func(s store.Store) {
		t.Helper()
		read(t, s, func(r store.Reader) {
			got, _ := r.Ticket("acme", 1)
			if !headEqual(got.Head, want) {
				t.Errorf("%s: head %s, want %s", when, showHead(got.Head), showHead(want))
			}
			if listed := r.FindTickets("acme", store.TicketFilter{}); len(listed) != 1 || !headEqual(listed[0].Head, want) {
				t.Errorf("%s: FindTickets' head differs from Ticket's", when)
			}
		})
	}
	check(s)
	if f.Reopen != nil {
		s = f.Reopen(t, s)
		check(s)
	}
	return s
}

func headEqual(a, b domain.TicketHead) bool {
	return a.Entered.Equal(b.Entered) && a.BackToReady.Equal(b.BackToReady) && a.ExternalStateChanged.Equal(b.ExternalStateChanged) &&
		ptrEqual(a.LiveClaim, b.LiveClaim, func(x, y domain.Claim) bool { return x == y }) &&
		ptrEqual(a.LatestFailureReport, b.LatestFailureReport, func(x, y domain.FailureReport) bool { return x == y }) &&
		ptrEqual(a.LatestRound, b.LatestRound, func(x, y domain.Round) bool {
			return x.Number == y.Number && x.Claim == y.Claim && x.Posted.Equal(y.Posted) && slices.Equal(x.Questions, y.Questions)
		})
}

// ptrEqual says whether x and y are both nil, or point to equal values.
func ptrEqual[T any](x, y *T, eq func(T, T) bool) bool {
	if x == nil || y == nil {
		return x == y
	}
	return eq(*x, *y)
}

func showHead(h domain.TicketHead) string {
	s := fmt.Sprintf("entered %s, back to Ready %s, external state changed %s",
		h.Entered.Format(time.RFC3339), h.BackToReady.Format(time.RFC3339), h.ExternalStateChanged.Format(time.RFC3339))
	if h.LiveClaim != nil {
		s += ", live Claim " + h.LiveClaim.ID
	}
	if h.LatestRound != nil {
		s += fmt.Sprintf(", Round %d", h.LatestRound.Number)
	}
	if h.LatestFailureReport != nil {
		s += fmt.Sprintf(", Failure Report %d", h.LatestFailureReport.Number)
	}
	return s
}

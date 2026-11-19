package core

import (
	"context"

	"github.com/tomoram/code-central/domain"
	"github.com/tomoram/code-central/store"
)

// SpecQuestions is get_spec_questions' result: the roll-up of a Spec's
// Questions. Questions sit only on Tickets, so it lists each of the Spec's
// Tickets that has Rounds, in claim order, with its Rounds.
type SpecQuestions struct {
	Spec    string            `json:"spec"`
	Tickets []TicketQuestions `json:"tickets"`
}

// TicketQuestions is one Ticket's Rounds in a Spec's roll-up.
type TicketQuestions struct {
	ID     string       `json:"id"`
	Title  string       `json:"title"`
	State  domain.State `json:"state"`
	Rounds []RoundView  `json:"rounds"`
}

// SpecInput names one Spec, for reads that need nothing else.
type SpecInput struct {
	Spec Ref `json:"spec" path:"n" desc:"The Spec." required:"true"`
}

// GetSpecQuestions is the human's get_spec_questions(spec), for the UI.
func (c *Core) GetSpecQuestions(_ context.Context, key string, ref Ref) (SpecQuestions, error) {
	var v SpecQuestions
	err := c.store.View(func(r store.Reader) error {
		if _, ok := r.Project(key); !ok {
			return unknownProject(key)
		}
		s, err := resolveSpec(r, key, "get_spec_questions", "spec", ref)
		if err != nil {
			return err
		}
		n := s.Number
		v = SpecQuestions{Spec: DisplayID(key, n), Tickets: []TicketQuestions{}}
		for _, t := range specTickets(r, key, n) {
			if t.Head.LatestRound == nil {
				continue
			}
			t, _ = r.Ticket(key, t.Number, store.Rounds)
			v.Tickets = append(v.Tickets, TicketQuestions{
				ID: DisplayID(key, t.Number), Title: t.Title, State: t.State, Rounds: roundViews(t.Rounds),
			})
		}
		return nil
	})
	return v, err
}

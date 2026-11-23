package core

import (
	"context"

	"github.com/tomoram/code-central/api"
	"github.com/tomoram/code-central/core/foundation"
	"github.com/tomoram/code-central/domain"
	"github.com/tomoram/code-central/store"
)

// GetSpecQuestions is the human's get_spec_questions(spec), for the UI.
func (c *Core) GetSpecQuestions(_ context.Context, key string, ref api.Ref) (api.SpecQuestions, error) {
	var v api.SpecQuestions
	err := c.store.View(func(r store.Reader) error {
		if _, ok := r.Project(key); !ok {
			return api.UnknownProjectError(key)
		}
		s, err := foundation.ResolveSpec(r, key, "get_spec_questions", "spec", ref)
		if err != nil {
			return err
		}
		n := s.Number
		v = api.SpecQuestions{Spec: domain.DisplayID(key, n), Tickets: []api.TicketQuestions{}}
		for _, t := range specTickets(r, key, n) {
			if t.Head.LatestRound == nil {
				continue
			}
			t, _ = r.Ticket(key, t.Number, store.Rounds)
			v.Tickets = append(v.Tickets, api.TicketQuestions{
				ID: domain.DisplayID(key, t.Number), Title: t.Title, State: t.State, Rounds: roundViews(t.Rounds),
			})
		}
		return nil
	})
	return v, err
}

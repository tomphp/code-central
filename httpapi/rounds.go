package httpapi

import (
	"net/http"

	"github.com/tomoram/code-central/core"
	"github.com/tomoram/code-central/store"
)

// Rounds and Questions (ticket 29). Question numbers run across their
// Ticket, so the path names the Ticket and the body the Question.
func init() {
	for name, r := range map[string]struct {
		pattern string
		h       handlerFunc
	}{
		"post_round": {"POST /api/p/{key}/tickets/{n}/post_round", func(c *core.Core, r *http.Request) (any, error) {
			in, err := pathBody(r, func(in *core.PostRoundInput) *core.Ref { return &in.Ticket })
			if err != nil {
				return nil, err
			}
			return c.PostRound(r.Context(), r.PathValue("key"), in)
		}},
		"pick_up_answers": {"POST /api/p/{key}/tickets/{n}/pick_up_answers", func(c *core.Core, r *http.Request) (any, error) {
			in, err := pathBody(r, func(in *core.PickUpAnswersInput) *core.Ref { return &in.Ticket })
			if err != nil {
				return nil, err
			}
			return c.PickUpAnswers(r.Context(), r.PathValue("key"), in)
		}},
		"answer": {"POST /api/p/{key}/tickets/{n}/answer", func(c *core.Core, r *http.Request) (any, error) {
			in, err := pathBody(r, func(in *core.AnswerInput) *core.Ref { return &in.Ticket })
			if err != nil {
				return nil, err
			}
			return c.Answer(r.Context(), r.PathValue("key"), in)
		}},
		"accept_recommended": {"POST /api/p/{key}/tickets/{n}/accept_recommended", func(c *core.Core, r *http.Request) (any, error) {
			in, err := pathBody(r, func(in *core.AcceptRecommendedInput) *core.Ref { return &in.Ticket })
			if err != nil {
				return nil, err
			}
			return c.AcceptRecommended(r.Context(), r.PathValue("key"), in)
		}},
		"dismiss_question": {"POST /api/p/{key}/tickets/{n}/dismiss_question", func(c *core.Core, r *http.Request) (any, error) {
			in, err := pathBody(r, func(in *core.DismissQuestionInput) *core.Ref { return &in.Ticket })
			if err != nil {
				return nil, err
			}
			return c.DismissQuestion(r.Context(), r.PathValue("key"), in)
		}},
		"get_spec_questions": {"GET /api/p/{key}/specs/{n}/questions", func(c *core.Core, r *http.Request) (any, error) {
			if err := onlyParams(r.URL.Query()); err != nil {
				return nil, err
			}
			return c.GetSpecQuestions(r.Context(), r.PathValue("key"), core.Ref(r.PathValue("n")))
		}},
	} {
		Routes[name], handlers[name] = r.pattern, r.h
	}
}

// WaitPath is the long-poll behind `code-central wait`. It is not a core
// operation, so it sits outside Routes.
const WaitPath = "GET /api/p/{key}/tickets/{n}/wait"

// wait holds for up to core.WaitTimeout and returns the Ticket's State,
// at once if it has already left ?from=.
func wait(c *core.Core, r *http.Request) (any, error) {
	q := r.URL.Query()
	if err := onlyParams(q, "from"); err != nil {
		return nil, err
	}
	return c.Wait(r.Context(), r.PathValue("key"), core.Ref(r.PathValue("n")), store.State(q.Get("from")))
}

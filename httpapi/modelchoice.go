package httpapi

import (
	"net/http"

	"github.com/tomoram/code-central/core"
)

// The Model Choice (Spec code-central-301): the Factory reads it before
// every resume and reports what each turn was started with.
func init() {
	Routes["get_model_choice"] = "GET /api/p/{key}/tickets/{n}/model-choice"
	Routes["report_model_choice"] = "POST /api/p/{key}/tickets/{n}/report_model_choice"

	handlers["get_model_choice"] = func(c *core.Core, r *http.Request) (any, error) {
		return c.GetModelChoice(r.Context(), r.PathValue("key"), core.GetModelChoiceInput{Ticket: core.Ref(r.PathValue("n"))})
	}
	handlers["report_model_choice"] = func(c *core.Core, r *http.Request) (any, error) {
		in, err := pathBody(r, func(in *core.ReportModelChoiceInput) *core.Ref { return &in.Ticket })
		if err != nil {
			return nil, err
		}
		return c.ReportModelChoice(r.Context(), r.PathValue("key"), in)
	}
}

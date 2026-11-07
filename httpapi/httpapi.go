// Package httpapi is the HTTP front door: a thin translator from requests
// onto core's operations. GET reads resources; every write is a POST to a
// named action with a strictly decoded JSON body.
package httpapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/tomoram/code-central/core"
	"github.com/tomoram/code-central/store"
)

// Routes maps every core operation to its HTTP method and path pattern
// (net/http ServeMux syntax). A test checks it against core.Operations.
var Routes = map[string]string{
	"create_project": "POST /api/projects/create_project",
	"get_project":    "GET /api/projects/{key}",
	"create_spec":    "POST /api/p/{key}/create_spec",
	"create_ticket":  "POST /api/p/{key}/create_ticket",
	"get_ticket":     "GET /api/p/{key}/tickets/{n}",
	"find_tickets":   "GET /api/p/{key}/tickets",
}

type handlerFunc func(c *core.Core, r *http.Request) (any, error)

var handlers = map[string]handlerFunc{
	"create_project": func(c *core.Core, r *http.Request) (any, error) {
		var in core.CreateProjectInput
		if err := decodeBody(r, &in); err != nil {
			return nil, err
		}
		return c.CreateProject(r.Context(), in)
	},
	"get_project": func(c *core.Core, r *http.Request) (any, error) {
		return c.GetProject(r.Context(), r.PathValue("key"))
	},
	"create_spec": func(c *core.Core, r *http.Request) (any, error) {
		var in core.CreateSpecInput
		if err := decodeBody(r, &in); err != nil {
			return nil, err
		}
		return c.CreateSpec(r.Context(), r.PathValue("key"), in)
	},
	"create_ticket": func(c *core.Core, r *http.Request) (any, error) {
		var in core.CreateTicketInput
		if err := decodeBody(r, &in); err != nil {
			return nil, err
		}
		return c.CreateTicket(r.Context(), r.PathValue("key"), in)
	},
	"get_ticket": func(c *core.Core, r *http.Request) (any, error) {
		return c.GetTicket(r.Context(), r.PathValue("key"), core.Ref(r.PathValue("n")))
	},
	"find_tickets": func(c *core.Core, r *http.Request) (any, error) {
		q := r.URL.Query()
		if err := onlyParams(q, "state", "title", "label", "spec"); err != nil {
			return nil, err
		}
		in := core.FindTicketsInput{Title: q.Get("title"), Labels: q["label"], Spec: core.Ref(q.Get("spec"))}
		for _, s := range q["state"] {
			in.State = append(in.State, store.State(s))
		}
		return c.FindTickets(r.Context(), r.PathValue("key"), in)
	},
}

// Register adds the HTTP API under /api/ to the service's one mux.
func Register(mux *http.ServeMux, c *core.Core) {
	for name, pattern := range Routes {
		h := handlers[name]
		mux.HandleFunc(pattern, func(w http.ResponseWriter, r *http.Request) {
			v, err := h(c, r)
			if err != nil {
				writeError(w, err)
				return
			}
			writeJSON(w, http.StatusOK, v)
		})
	}
	mux.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead && r.Method != http.MethodPost {
			// There is no PUT, PATCH or DELETE: writes are named POST actions.
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		writeError(w, &core.Error{Code: core.NotFound, Message: fmt.Sprintf("no API resource at %s %s", r.Method, r.URL.Path)})
	})
}

// Status maps an error code to its HTTP status.
func Status(code core.Code) int {
	switch code {
	case core.UnknownProject, core.NotFound:
		return http.StatusNotFound
	case core.ActionNotAllowed, core.StaleClaim, core.Cycle:
		return http.StatusConflict
	case core.Invalid:
		return http.StatusUnprocessableEntity
	}
	return http.StatusInternalServerError
}

func writeError(w http.ResponseWriter, err error) {
	var e *core.Error
	if !errors.As(err, &e) {
		// Not a domain error: something broke inside the service.
		e = &core.Error{Code: "internal", Message: err.Error()}
	}
	writeJSON(w, Status(e.Code), e)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

// decodeBody decodes a JSON body strictly: unknown fields are invalid.
func decodeBody(r *http.Request, v any) error {
	body, err := io.ReadAll(io.LimitReader(r.Body, 8<<20))
	if err != nil {
		return &core.Error{Code: core.Invalid, Message: "cannot read the request body"}
	}
	return core.Decode(body, v)
}

func onlyParams(q url.Values, allowed ...string) error {
	for k := range q {
		ok := false
		for _, a := range allowed {
			ok = ok || k == a
		}
		if !ok {
			return &core.Error{Code: core.Invalid, Field: k, Message: fmt.Sprintf("unknown query parameter %q (allowed: %s)", k, strings.Join(allowed, ", "))}
		}
	}
	return nil
}

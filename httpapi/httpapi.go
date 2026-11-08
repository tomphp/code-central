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

	"edit_spec":     "POST /api/p/{key}/specs/{n}/edit_spec",
	"edit_ticket":   "POST /api/p/{key}/tickets/{n}/edit_ticket",
	"add_labels":    "POST /api/p/{key}/add_labels",
	"remove_labels": "POST /api/p/{key}/remove_labels",
	"set_tags":      "POST /api/p/{key}/set_tags",
	"remove_tags":   "POST /api/p/{key}/remove_tags",
	"comment":       "POST /api/p/{key}/tickets/{n}/comment",
	"approve":       "POST /api/p/{key}/tickets/{n}/approve",
	"defer":         "POST /api/p/{key}/tickets/{n}/defer",

	"claim":  "POST /api/p/{key}/claim",
	"finish": "POST /api/p/{key}/tickets/{n}/finish",
	"accept": "POST /api/p/{key}/tickets/{n}/accept",
	"reject": "POST /api/p/{key}/tickets/{n}/reject",

	"set_session":  "POST /api/p/{key}/tickets/{n}/set_session",
	"release":      "POST /api/p/{key}/tickets/{n}/release",
	"get_workflow": "GET /api/workflow",
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
		if err := onlyParams(q, "state", "title", "label", "tag", "spec", "claimable"); err != nil {
			return nil, err
		}
		in := core.FindTicketsInput{Title: q.Get("title"), Labels: q["label"], Spec: core.Ref(q.Get("spec"))}
		for _, s := range q["state"] {
			in.State = append(in.State, store.State(s))
		}
		var err error
		if in.Tags, err = tagParams(q); err != nil {
			return nil, err
		}
		if in.Claimable, err = boolParam(q, "claimable"); err != nil {
			return nil, err
		}
		return c.FindTickets(r.Context(), r.PathValue("key"), in)
	},

	"edit_spec": func(c *core.Core, r *http.Request) (any, error) {
		in, err := pathBody(r, func(in *core.EditSpecInput) *core.Ref { return &in.Spec })
		if err != nil {
			return nil, err
		}
		return c.EditSpec(r.Context(), r.PathValue("key"), in)
	},
	"edit_ticket": func(c *core.Core, r *http.Request) (any, error) {
		in, err := pathBody(r, func(in *core.EditTicketInput) *core.Ref { return &in.Ticket })
		if err != nil {
			return nil, err
		}
		return c.EditTicket(r.Context(), r.PathValue("key"), in)
	},
	"add_labels": func(c *core.Core, r *http.Request) (any, error) {
		var in core.LabelsInput
		if err := decodeBody(r, &in); err != nil {
			return nil, err
		}
		return c.AddLabels(r.Context(), r.PathValue("key"), in)
	},
	"remove_labels": func(c *core.Core, r *http.Request) (any, error) {
		var in core.LabelsInput
		if err := decodeBody(r, &in); err != nil {
			return nil, err
		}
		return c.RemoveLabels(r.Context(), r.PathValue("key"), in)
	},
	"set_tags": func(c *core.Core, r *http.Request) (any, error) {
		var in core.SetTagsInput
		if err := decodeBody(r, &in); err != nil {
			return nil, err
		}
		return c.SetTags(r.Context(), r.PathValue("key"), in)
	},
	"remove_tags": func(c *core.Core, r *http.Request) (any, error) {
		var in core.RemoveTagsInput
		if err := decodeBody(r, &in); err != nil {
			return nil, err
		}
		return c.RemoveTags(r.Context(), r.PathValue("key"), in)
	},
	"comment": func(c *core.Core, r *http.Request) (any, error) {
		in, err := pathBody(r, func(in *core.CommentInput) *core.Ref { return &in.Ticket })
		if err != nil {
			return nil, err
		}
		return c.Comment(r.Context(), r.PathValue("key"), in)
	},
	"approve": func(c *core.Core, r *http.Request) (any, error) {
		in, err := pathBody(r, func(in *core.TicketInput) *core.Ref { return &in.Ticket })
		if err != nil {
			return nil, err
		}
		return c.Approve(r.Context(), r.PathValue("key"), in)
	},
	"defer": func(c *core.Core, r *http.Request) (any, error) {
		in, err := pathBody(r, func(in *core.DeferInput) *core.Ref { return &in.Ticket })
		if err != nil {
			return nil, err
		}
		return c.Defer(r.Context(), r.PathValue("key"), in)
	},
	"claim": func(c *core.Core, r *http.Request) (any, error) {
		var in core.ClaimInput
		if err := decodeBody(r, &in); err != nil {
			return nil, err
		}
		return c.Claim(r.Context(), r.PathValue("key"), in)
	},
	"finish": func(c *core.Core, r *http.Request) (any, error) {
		in, err := pathBody(r, func(in *core.FinishInput) *core.Ref { return &in.Ticket })
		if err != nil {
			return nil, err
		}
		return c.Finish(r.Context(), r.PathValue("key"), in)
	},
	"accept": func(c *core.Core, r *http.Request) (any, error) {
		in, err := pathBody(r, func(in *core.TicketInput) *core.Ref { return &in.Ticket })
		if err != nil {
			return nil, err
		}
		return c.Accept(r.Context(), r.PathValue("key"), in)
	},
	"reject": func(c *core.Core, r *http.Request) (any, error) {
		in, err := pathBody(r, func(in *core.RejectInput) *core.Ref { return &in.Ticket })
		if err != nil {
			return nil, err
		}
		return c.Reject(r.Context(), r.PathValue("key"), in)
	},

	"set_session": func(c *core.Core, r *http.Request) (any, error) {
		in, err := pathBody(r, func(in *core.SetSessionInput) *core.Ref { return &in.Ticket })
		if err != nil {
			return nil, err
		}
		return c.SetSession(r.Context(), r.PathValue("key"), in)
	},
	"release": func(c *core.Core, r *http.Request) (any, error) {
		in, err := pathBody(r, func(in *core.ReleaseInput) *core.Ref { return &in.Ticket })
		if err != nil {
			return nil, err
		}
		return c.Release(r.Context(), r.PathValue("key"), in)
	},
	"get_workflow": func(c *core.Core, r *http.Request) (any, error) {
		if err := onlyParams(r.URL.Query()); err != nil {
			return nil, err
		}
		return c.GetWorkflow(r.Context())
	},
}

// Register adds the HTTP API under /api/ to the service's one mux. The
// request's Actor is already in its context (frontdoor.Guard reads the
// Actor header); without one it is agent.
func Register(mux *http.ServeMux, c *core.Core) {
	all := map[string]handlerFunc{WaitPath: wait, TranscriptPath: transcript}
	for name, pattern := range Routes {
		all[pattern] = handlers[name]
	}
	for pattern, h := range all {
		mux.HandleFunc(pattern, func(w http.ResponseWriter, r *http.Request) {
			v, err := h(c, r)
			if err != nil {
				WriteError(w, err)
				return
			}
			writeJSON(w, http.StatusOK, v)
		})
	}
	mux.HandleFunc("POST /api/ping", ping(c))
	mux.HandleFunc(SessionEventsPath, sessionEvents(c))
	mux.HandleFunc(EventsPath, events(c))
	mux.HandleFunc(TranscriptStreamPath, transcriptStream(c))
	mux.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead && r.Method != http.MethodPost {
			// There is no PUT, PATCH or DELETE: writes are named POST actions.
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		WriteError(w, &core.Error{Code: core.NotFound, Message: fmt.Sprintf("no API resource at %s %s", r.Method, r.URL.Path)})
	})
}

// ping is POST /api/ping {session_id}. It is not one of core's
// operations: it updates memory only, and always answers 204, even for an
// unknown session or a body it can't read, so a hook never fails on it.
func ping(c *core.Core) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			SessionID string `json:"session_id"`
		}
		if decodeBody(r, &in) == nil {
			c.Ping(r.Context(), in.SessionID)
		}
		w.WriteHeader(http.StatusNoContent)
	}
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

// WriteError writes err as the API's JSON error with its status; an error
// that is not a *core.Error is internal.
func WriteError(w http.ResponseWriter, err error) {
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

// pathBody decodes a per-item action's body strictly, then sets the
// item's ref (its Ticket or Spec) from the path's {n}, which wins over
// any ref in the body. Every per-item route binds its path this way.
func pathBody[In any](r *http.Request, item func(*In) *core.Ref) (In, error) {
	var in In
	if err := decodeBody(r, &in); err != nil {
		return in, err
	}
	*item(&in) = core.Ref(r.PathValue("n"))
	return in, nil
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

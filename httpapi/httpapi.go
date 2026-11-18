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
	"github.com/tomoram/code-central/ops"
)

// legacyRoutes maps each operation not yet defined in ops to its HTTP
// method and path pattern (net/http ServeMux syntax), with its handler in
// handlers. Routes merges them with the definitions'; a migration guard
// checks that no operation is in both.
var legacyRoutes = map[string]string{
	"create_project": "POST /api/projects/create_project",
	"get_project":    "GET /api/projects/{key}",
	"get_workflow":   "GET /api/workflow",
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
	"get_workflow": func(c *core.Core, r *http.Request) (any, error) {
		if err := onlyParams(r.URL.Query()); err != nil {
			return nil, err
		}
		return c.GetWorkflow(r.Context())
	},
}

// Register adds the HTTP API under /api/ to the service's one mux. The
// request's Actor is already in its context (server.Guard reads the
// Actor header); without one it is agent.
func Register(mux *http.ServeMux, c *core.Core) {
	all := map[string]handlerFunc{WaitPath: wait, TranscriptPath: transcript, UsagePath: usageRead, SessionUsagePath: sessionUsageRead, CIOverviewPath: ciOverviewRead}
	for name, pattern := range legacyRoutes {
		all[pattern] = handlers[name]
	}
	for _, op := range ops.Definitions() {
		method, pattern := op.Route()
		all[method+" "+pattern] = derived[op.Op().Name]
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
	mux.HandleFunc(ChatEventsPath, chatEvents(c))
	mux.HandleFunc(PostUsagePath, postUsage(c))
	mux.HandleFunc(EventsPath, events(c))
	mux.HandleFunc(TranscriptStreamPath, transcriptStream(c))
	mux.HandleFunc(ChatStreamPath, chatStream(c))
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
	return core.Decode(r.Context(), body, v)
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

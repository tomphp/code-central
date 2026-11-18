// Package httpapi is the HTTP front door: a thin translator from requests
// onto core's operations. GET reads resources; every write is a POST to a
// named action with a strictly decoded JSON body.
package httpapi

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/tomoram/code-central/core"
	"github.com/tomoram/code-central/ops"
)

type handlerFunc func(s ops.Service, r *http.Request) (any, error)

// ActorHeader names the Actor making a request: human, agent or
// integration, defaulting to agent. Only the UI sends human. An operation
// passes it on in its ops.Call, unchecked: the dispatch acts as it or
// refuses it.
const ActorHeader = "X-Code-Central-Actor"

// Register adds the HTTP API under /api/ to the service's one mux, over s.
func Register(mux *http.ServeMux, s ops.Service) {
	all := map[string]handlerFunc{WaitPath: wait, TranscriptPath: transcript, UsagePath: usageRead, SessionUsagePath: sessionUsageRead, CIOverviewPath: ciOverviewRead}
	for _, op := range ops.Definitions() {
		method, pattern := op.Route()
		all[method+" "+pattern] = derived[op.Op().Name]
	}
	for pattern, h := range all {
		mux.HandleFunc(pattern, func(w http.ResponseWriter, r *http.Request) {
			v, err := h(s, r)
			if err != nil {
				WriteError(w, err)
				return
			}
			writeJSON(w, http.StatusOK, v)
		})
	}
	mux.HandleFunc("POST /api/ping", ping(s))
	mux.HandleFunc(SessionEventsPath, sessionEvents(s))
	mux.HandleFunc(ChatEventsPath, chatEvents(s))
	mux.HandleFunc(PostUsagePath, postUsage(s))
	mux.HandleFunc(EventsPath, events(s))
	mux.HandleFunc(TranscriptStreamPath, transcriptStream(s))
	mux.HandleFunc(ChatStreamPath, chatStream(s))
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
func ping(s ops.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			SessionID string `json:"session_id"`
		}
		if decodeBody(r, &in) == nil {
			s.Ping(r.Context(), in.SessionID)
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
	e := core.ErrorOf(err)
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

package httpapi

import (
	"net/http"
	"strconv"

	"github.com/tomoram/code-central/core"
)

// SessionEventsPath is where a Factory posts a session's Session Events
// (ADR 0008). Like ping it is not one of core's operations: it updates
// memory only, and always answers 204, even for an unknown session or a
// body it can't read, so a Factory never fails on it.
const SessionEventsPath = "POST /api/session-events"

// TranscriptPath reads one Claim's Transcript. It is outside core's
// operations and outside MCP.
const TranscriptPath = "GET /api/projects/{key}/tickets/{n}/claims/{c}/transcript"

func sessionEvents(c *core.Core) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			SessionID string              `json:"session_id"`
			Events    []core.SessionEvent `json:"events"`
		}
		if decodeBody(r, &in) == nil {
			c.PostSessionEvents(r.Context(), in.SessionID, in.Events)
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

func transcript(c *core.Core, r *http.Request) (any, error) {
	if err := onlyParams(r.URL.Query()); err != nil {
		return nil, err
	}
	claim, err := strconv.Atoi(r.PathValue("c"))
	if err != nil {
		return nil, &core.Error{Code: core.NotFound, Message: "there is no Claim " + strconv.Quote(r.PathValue("c"))}
	}
	return c.Transcript(r.Context(), r.PathValue("key"), core.Ref(r.PathValue("n")), claim)
}

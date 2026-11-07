package httpapi

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/tomoram/code-central/core"
)

// EventsPath is the UI's live feed (ticket 33): Server-Sent Events, one
// per change notice on core's events bus. Each event's name is the notice
// type (ticket.changed, spec.changed, project.changed) and its data names
// what changed: {project, id} or {project}. It is not one of core's
// operations, so it sits outside Routes. There is no replay: a client
// that reconnects refetches everything.
const EventsPath = "GET /api/events"

func events(c *core.Core) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		flusher, ok := w.(http.Flusher)
		if !ok {
			writeError(w, fmt.Errorf("the response can't stream"))
			return
		}
		notices, stop := c.Events().Subscribe()
		defer stop()
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-store")
		w.WriteHeader(http.StatusOK)
		// The opening comment tells the client the subscription is live.
		fmt.Fprint(w, ": connected\n\n")
		flusher.Flush()
		for {
			select {
			case <-r.Context().Done():
				return
			case e, open := <-notices:
				if !open {
					return
				}
				data, _ := json.Marshal(struct {
					Project string `json:"project"`
					ID      string `json:"id,omitempty"`
				}{e.Project, e.ID})
				fmt.Fprintf(w, "event: %s\ndata: %s\n\n", e.Type, data)
				flusher.Flush()
			}
		}
	}
}

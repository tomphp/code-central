package httpapi_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/tomoram/code-central/clock"
	"github.com/tomoram/code-central/core"
	"github.com/tomoram/code-central/httpapi"
	"github.com/tomoram/code-central/store"
)

// asHuman makes the request as the human, as frontdoor.Guard would from
// the Actor header.
func asHuman(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h.ServeHTTP(w, r.WithContext(core.WithActor(r.Context(), core.Human)))
	})
}

func TestOptionsRoutes(t *testing.T) {
	if got := httpapi.Routes["get_options"]; got != "GET /api/options" {
		t.Errorf("get_options route %q", got)
	}
	if got := httpapi.Routes["set_options"]; got != "POST /api/set_options" {
		t.Errorf("set_options route %q", got)
	}
	h := asHuman(newHandler(t))
	if rec := do(h, "GET", "/api/set_options", ""); rec.Code == 200 {
		t.Fatal("GET reached set_options")
	}
}

func TestGetAndSetOptions(t *testing.T) {
	h := asHuman(newHandler(t))
	if rec := do(h, "GET", "/api/options", ""); rec.Code != 200 || strings.TrimSpace(rec.Body.String()) != `{"focus_clear":"activity_dashboard","gh_path":""}` {
		t.Fatalf("get_options: %d %s", rec.Code, rec.Body)
	}
	rec := do(h, "POST", "/api/set_options", `{"focus_clear":"all_clear"}`)
	if rec.Code != 200 || strings.TrimSpace(rec.Body.String()) != `{"focus_clear":"all_clear","gh_path":""}` {
		t.Fatalf("set_options: %d %s", rec.Code, rec.Body)
	}
	if rec := do(h, "GET", "/api/options", ""); strings.TrimSpace(rec.Body.String()) != `{"focus_clear":"all_clear","gh_path":""}` {
		t.Fatalf("get_options after set: %s", rec.Body)
	}
	// An empty change answers the Options as they stand.
	if rec := do(h, "POST", "/api/set_options", `{}`); rec.Code != 200 || strings.TrimSpace(rec.Body.String()) != `{"focus_clear":"all_clear","gh_path":""}` {
		t.Fatalf("empty set_options: %d %s", rec.Code, rec.Body)
	}
}

func TestSetOptionsIsRefusedForAnAgent(t *testing.T) {
	h := newHandler(t) // no Actor: the agent
	rec := do(h, "POST", "/api/set_options", `{"focus_clear":"all_clear"}`)
	if rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), `"code":"action_not_allowed"`) {
		t.Fatalf("agent set_options: %d %s", rec.Code, rec.Body)
	}
	if rec := do(h, "GET", "/api/options", ""); strings.TrimSpace(rec.Body.String()) != `{"focus_clear":"activity_dashboard","gh_path":""}` {
		t.Fatalf("the agent can read the Options, unchanged: %d %s", rec.Code, rec.Body)
	}
}

func TestSetOptionsRefusesBadInput(t *testing.T) {
	h := asHuman(newHandler(t))
	for _, c := range []struct{ name, body, field, mentions string }{
		{"unknown Option", `{"colour":"red"}`, "colour", "the Options are: focus_clear"},
		{"value outside its set", `{"focus_clear":"fireworks"}`, "focus_clear", "allowed: activity_dashboard, all_clear"},
		{"wrong type", `{"focus_clear":3}`, "focus_clear", "focus_clear"},
	} {
		t.Run(c.name, func(t *testing.T) {
			rec := do(h, "POST", "/api/set_options", c.body)
			var e map[string]any
			json.Unmarshal(rec.Body.Bytes(), &e)
			if rec.Code != http.StatusUnprocessableEntity || e["code"] != string(core.Invalid) || e["field"] != c.field {
				t.Fatalf("%d %s", rec.Code, rec.Body)
			}
			if msg, _ := e["message"].(string); !strings.Contains(msg, c.mentions) {
				t.Fatalf("message %q should mention %q", msg, c.mentions)
			}
		})
	}
	if rec := do(h, "GET", "/api/options", ""); strings.TrimSpace(rec.Body.String()) != `{"focus_clear":"activity_dashboard","gh_path":""}` {
		t.Fatalf("a refused set_options changed the Options: %s", rec.Body)
	}
}

func TestEventsCarryOptionsChangedWithNoProjectOrID(t *testing.T) {
	mux, r := liveFeed(t)
	h := asHuman(mux)
	do(h, "POST", "/api/set_options", `{"focus_clear":"all_clear"}`)
	do(h, "POST", "/api/set_options", `{"focus_clear":"all_clear"}`) // no change, no event
	do(h, "POST", "/api/projects/create_project", `{"key":"acme"}`)

	want := [][2]string{
		{"options.changed", `{}`},
		{"project.changed", `{"project":"acme"}`},
	}
	for _, w := range want {
		if name, data := nextEvent(t, r); name != w[0] || data != w[1] {
			t.Fatalf("got %s %s, want %s %s", name, data, w[0], w[1])
		}
	}
}

func TestWaitDoesNotWakeOnOptionsChanged(t *testing.T) {
	clk := clock.NewFake(time.Date(2026, 11, 7, 9, 0, 0, 0, time.UTC))
	h := asHuman(serve(store.NewMemory(), clk))
	do(h, "POST", "/api/projects/create_project", `{"key":"acme"}`)
	do(h, "POST", "/api/p/acme/create_spec", `{"title":"S"}`)
	do(h, "POST", "/api/p/acme/create_ticket", `{"spec":1,"title":"A","state":"Ready"}`)

	done := make(chan *httptest.ResponseRecorder, 1)
	go func() { done <- do(h, "GET", "/api/p/acme/tickets/2/wait?from=Ready", "") }()
	clk.BlockUntil(1)
	if rec := do(h, "POST", "/api/set_options", `{"focus_clear":"all_clear"}`); rec.Code != 200 {
		t.Fatalf("set_options: %d %s", rec.Code, rec.Body)
	}
	select {
	case rec := <-done:
		t.Fatalf("wait returned on options.changed: %s", rec.Body)
	case <-time.After(50 * time.Millisecond):
	}
	// It still holds until its timeout, on the same timer.
	clk.Advance(core.WaitTimeout)
	if rec := <-done; rec.Code != 200 || !strings.Contains(rec.Body.String(), `"state":"Ready"`) {
		t.Fatalf("wait: %d %s", rec.Code, rec.Body)
	}
}

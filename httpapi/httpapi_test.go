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
	"github.com/tomoram/code-central/ids"
	"github.com/tomoram/code-central/store"
)

func newHandler(t *testing.T) http.Handler {
	t.Helper()
	c := core.New(store.NewMemory(), clock.NewFake(time.Date(2026, 11, 7, 9, 0, 0, 0, time.UTC)), &ids.Sequence{})
	mux := http.NewServeMux()
	httpapi.Register(mux, c)
	return mux
}

func do(h http.Handler, method, path, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestErrorStatusAndShape(t *testing.T) {
	h := newHandler(t)
	do(h, "POST", "/api/projects/create_project", `{"key":"acme"}`)
	cases := []struct {
		name, method, path, body string
		status                   int
		code                     core.Code
		field                    string
	}{
		{"unknown project", "GET", "/api/projects/nope", "", 404, core.UnknownProject, ""},
		{"unknown project, scoped", "GET", "/api/p/nope/tickets", "", 404, core.UnknownProject, ""},
		{"not found", "GET", "/api/p/acme/tickets/7", "", 404, core.NotFound, ""},
		{"invalid", "POST", "/api/p/acme/create_spec", `{"title":""}`, 422, core.Invalid, "title"},
		{"unknown request field", "POST", "/api/p/acme/create_spec", `{"title":"S","colour":"red"}`, 422, core.Invalid, "colour"},
		{"malformed JSON", "POST", "/api/p/acme/create_spec", `{`, 422, core.Invalid, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			rec := do(h, c.method, c.path, c.body)
			if rec.Code != c.status {
				t.Fatalf("status %d, want %d: %s", rec.Code, c.status, rec.Body)
			}
			var e map[string]any
			if err := json.Unmarshal(rec.Body.Bytes(), &e); err != nil {
				t.Fatal(err)
			}
			if e["code"] != string(c.code) || e["message"] == "" {
				t.Fatalf("body %s", rec.Body)
			}
			if f, _ := e["field"].(string); f != c.field {
				t.Fatalf("field %q, want %q", f, c.field)
			}
		})
	}
}

func TestFindTicketsQueryParameters(t *testing.T) {
	h := newHandler(t)
	do(h, "POST", "/api/projects/create_project", `{"key":"acme"}`)
	do(h, "POST", "/api/p/acme/create_spec", `{"title":"S"}`)
	do(h, "POST", "/api/p/acme/create_ticket", `{"spec":1,"title":"A","state":"Ready","labels":["x","y"]}`)
	do(h, "POST", "/api/p/acme/create_ticket", `{"spec":"acme-1","title":"B"}`)

	rec := do(h, "GET", "/api/p/acme/tickets?state=Ready&state=Draft&label=x&label=y&title=a&spec=1", "")
	var rows []core.TicketRow
	json.Unmarshal(rec.Body.Bytes(), &rows)
	if rec.Code != 200 || len(rows) != 1 || rows[0].ID != "acme-2" {
		t.Fatalf("%d %s", rec.Code, rec.Body)
	}
	if rec := do(h, "GET", "/api/p/acme/tickets?colour=red", ""); rec.Code != 422 {
		t.Fatalf("unknown query parameter: %d", rec.Code)
	}
}

func TestTheActorHeaderIsRecordedDefaultingToAgent(t *testing.T) {
	h := newHandler(t)
	do(h, "POST", "/api/projects/create_project", `{"key":"acme"}`)
	do(h, "POST", "/api/p/acme/create_spec", `{"title":"S"}`)
	do(h, "POST", "/api/p/acme/create_ticket", `{"spec":1,"title":"A"}`)

	req := httptest.NewRequest("POST", "/api/p/acme/tickets/2/comment", strings.NewReader(`{"body":"From the UI."}`))
	req.Header.Set("X-Code-Central-Actor", "human")
	h.ServeHTTP(httptest.NewRecorder(), req)
	rec := do(h, "POST", "/api/p/acme/tickets/2/comment", `{"body":"No header."}`)
	var v core.TicketView
	json.Unmarshal(rec.Body.Bytes(), &v)
	if len(v.Comments) != 2 || v.Comments[0].Actor != "human" || v.Comments[1].Actor != "agent" {
		t.Fatalf("comments %+v", v.Comments)
	}

	req = httptest.NewRequest("POST", "/api/p/acme/tickets/2/comment", strings.NewReader(`{"body":"x"}`))
	req.Header.Set("X-Code-Central-Actor", "robot")
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != 422 || !strings.Contains(rec.Body.String(), "X-Code-Central-Actor") {
		t.Fatalf("unknown Actor: %d %s", rec.Code, rec.Body)
	}
}

func TestWritesAreOnlyPOSTs(t *testing.T) {
	h := newHandler(t)
	if rec := do(h, "GET", "/api/projects/create_project", ""); rec.Code == 200 {
		t.Fatal("GET reached a write")
	}
	if rec := do(h, "PUT", "/api/p/acme/create_spec", `{"title":"S"}`); rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("PUT: %d", rec.Code)
	}
}

func doAs(h http.Handler, actor, method, path, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	if actor != "" {
		req.Header.Set(httpapi.ActorHeader, actor)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestTheActorComesFromAHeaderDefaultingToAgent(t *testing.T) {
	h := newHandler(t)
	do(h, "POST", "/api/projects/create_project", `{"key":"acme"}`)
	do(h, "POST", "/api/p/acme/create_spec", `{"title":"S"}`)
	do(h, "POST", "/api/p/acme/create_ticket", `{"spec":1,"title":"T","state":"Ready"}`)

	if rec := do(h, "POST", "/api/p/acme/claim", `{}`); rec.Code != 200 {
		t.Fatalf("claim: %d %s", rec.Code, rec.Body)
	}
	do(h, "POST", "/api/p/acme/tickets/2/finish", `{"claim_id":"claim-1","summary":"S"}`)
	if rec := do(h, "POST", "/api/p/acme/tickets/2/accept", `{}`); rec.Code != http.StatusConflict {
		t.Fatalf("accept with no Actor header: %d %s, want 409 (an agent may not accept)", rec.Code, rec.Body)
	}
	if rec := doAs(h, "robot", "POST", "/api/p/acme/tickets/2/accept", `{}`); rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("unknown Actor: %d %s, want 422", rec.Code, rec.Body)
	}
	rec := doAs(h, "human", "POST", "/api/p/acme/tickets/2/accept", `{}`)
	if rec.Code != 200 {
		t.Fatalf("accept as human: %d %s", rec.Code, rec.Body)
	}
	var v core.TicketView
	json.Unmarshal(rec.Body.Bytes(), &v)
	var actors []string
	for _, m := range v.Moves {
		actors = append(actors, m.Actor)
	}
	if strings.Join(actors, ",") != "agent,agent,human" {
		t.Fatalf("move Actors %v, want agent (claim), agent (finish), human (accept)", actors)
	}
}

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
	return httpapi.NewHandler(c, nil)
}

func do(h http.Handler, method, path, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestEveryOperationHasARoute(t *testing.T) {
	for _, op := range core.Operations {
		if _, ok := httpapi.Routes[op.Name]; !ok {
			t.Errorf("operation %s has no HTTP route", op.Name)
		}
	}
	for name := range httpapi.Routes {
		if _, ok := core.Operation(name); !ok {
			t.Errorf("route %s is not a core operation", name)
		}
	}
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

func TestWritesAreOnlyPOSTs(t *testing.T) {
	h := newHandler(t)
	if rec := do(h, "GET", "/api/projects/create_project", ""); rec.Code == 200 {
		t.Fatal("GET reached a write")
	}
	if rec := do(h, "PUT", "/api/p/acme/create_spec", `{"title":"S"}`); rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("PUT: %d", rec.Code)
	}
}

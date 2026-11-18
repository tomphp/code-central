package server_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"github.com/tomoram/code-central/codecentraltest"
	"github.com/tomoram/code-central/core"
	"github.com/tomoram/code-central/httpapi"
	"github.com/tomoram/code-central/ops"
	"github.com/tomoram/code-central/server"
)

func TestGuardAllowsOnlyLocalHostsAndOrigins(t *testing.T) {
	reached := false
	h := server.Guard(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reached = true
	}), "192.168.65.1")

	cases := []struct {
		name, host, origin string
		allowed            bool
	}{
		{"localhost", "localhost:7733", "", true},
		{"localhost without port", "localhost", "", true},
		{"IPv4 loopback", "127.0.0.1:7733", "", true},
		{"IPv6 loopback", "[::1]:7733", "", true},
		{"Docker host", "host.docker.internal:7733", "", true},
		{"a --bind address", "192.168.65.1:7733", "", true},
		{"host names compare without case", "LocalHost:7733", "", true},
		{"rebound host name", "evil.example:7733", "", false},
		{"other address", "10.0.0.5:7733", "", false},
		{"bare IPv6 without brackets", "::1", "", false},
		{"empty host", "", "", false},
		{"local origin", "localhost:7733", "http://localhost:7733", true},
		{"loopback origin on another port", "127.0.0.1:7733", "http://127.0.0.1:5173", true},
		{"IPv6 origin", "[::1]:7733", "http://[::1]:7733", true},
		{"Docker origin", "host.docker.internal:7733", "http://host.docker.internal:7733", true},
		{"foreign origin", "localhost:7733", "http://evil.example", false},
		{"opaque origin", "localhost:7733", "null", false},
		{"origin naming a local host as a subdomain", "localhost:7733", "http://localhost.evil.example", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			reached = false
			req := httptest.NewRequest("POST", "/api/p/acme/create_spec", strings.NewReader(`{}`))
			req.Host = c.host
			if c.origin != "" {
				req.Header.Set("Origin", c.origin)
			}
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)
			if reached != c.allowed {
				t.Fatalf("reached handler: %v, want %v (status %d)", reached, c.allowed, rec.Code)
			}
			if !c.allowed && rec.Code != http.StatusForbidden {
				t.Fatalf("status %d, want 403", rec.Code)
			}
		})
	}
}

// An HTTP operation acts as the Actor its header names, agent when it
// names none; an unknown Actor is refused, naming the header, before the
// operation runs.
func TestTheHTTPAPIActsAsTheActorItsHeaderNames(t *testing.T) {
	srv := newServer(t)
	for _, actor := range []core.Actor{"", core.Agent, core.Human, core.Integration} {
		status, b := httpDoAs(t, srv, actor, httpCall{method: "POST", path: "/api/p/acme/tickets/2/comment", body: `{"body":"Seen."}`})
		if status != http.StatusOK {
			t.Fatalf("Actor %q: status %d: %s", actor, status, b)
		}
	}
	tk, err := srv.Client.GetTicket(context.Background(), "acme", "2")
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, c := range tk.Comments {
		got = append(got, string(c.Actor))
	}
	if want := []string{"agent", "agent", "human", "integration"}; !slices.Equal(got, want) {
		t.Fatalf("comments made as %v, want %v", got, want)
	}

	status, b := httpDoAs(t, srv, "robot", httpCall{method: "POST", path: "/api/p/acme/tickets/2/comment", body: `{"body":"Beep."}`})
	if e := errorJSON(t, b, core.Invalid); status != http.StatusUnprocessableEntity || e.Field != httpapi.ActorHeader {
		t.Fatalf("unknown Actor: status %d: %s", status, b)
	}
	if tk, _ := srv.Client.GetTicket(context.Background(), "acme", "2"); len(tk.Comments) != len(got) {
		t.Fatalf("unknown Actor: the comment was made")
	}
}

// The MCP library's own Host check is off, so a Docker session's
// host.docker.internal reaches MCP; the Guard alone decides.
func TestDockerHostReachesMCP(t *testing.T) {
	srv := codecentraltest.NewServer(t)
	req, _ := http.NewRequest("POST", srv.URL+"/p/acme/mcp", strings.NewReader(
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-11-25","capabilities":{},"clientInfo":{"name":"t","version":"0"}}}`))
	req.Host = "host.docker.internal:7733"
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status %d, want 200", resp.StatusCode)
	}
}

// A GET never writes: not to any write operation's route, nor to MCP.
func TestGETNeverWrites(t *testing.T) {
	srv := codecentraltest.NewServer(t)
	ctx := context.Background()
	if _, err := srv.Core.CreateProject(ctx, core.CreateProjectInput{Key: "acme"}); err != nil {
		t.Fatal(err)
	}
	paths := []string{"/p/acme/mcp"}
	routes := httpapi.Routes()
	for _, op := range ops.All() {
		if op.Write {
			_, pattern, _ := strings.Cut(routes[op.Name], " ")
			paths = append(paths, strings.ReplaceAll(pattern, "{key}", "acme"))
		}
	}
	body := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"create_spec","arguments":{"title":"Made"}},"key":"made","title":"Made","spec":1}`
	for _, p := range paths {
		req, _ := http.NewRequest("GET", srv.URL+p+"?title=Made&key=made", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json, text/event-stream")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode == http.StatusOK {
			t.Errorf("GET %s: 200", p)
		}
	}
	if _, err := srv.Core.GetProject(ctx, "made"); err == nil {
		t.Error("a GET created a Project")
	}
	// Nothing took a number in acme, so the next item is acme-1.
	s, err := srv.Core.CreateSpec(ctx, "acme", core.CreateSpecInput{Title: "First"})
	if err != nil || s.ID != "acme-1" {
		t.Errorf("a GET wrote to acme: next ID %q, %v", s.ID, err)
	}
}

func TestGuardSendsNoCORSHeaders(t *testing.T) {
	h := server.Guard(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	for _, origin := range []string{"http://localhost:7733", "http://evil.example"} {
		for _, method := range []string{"OPTIONS", "GET", "POST"} {
			req := httptest.NewRequest(method, "/api/projects", nil)
			req.Host = "localhost:7733"
			req.Header.Set("Origin", origin)
			req.Header.Set("Access-Control-Request-Method", "POST")
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)
			for k := range rec.Header() {
				if strings.HasPrefix(http.CanonicalHeaderKey(k), "Access-Control-") {
					t.Errorf("%s from %s: sent %s", method, origin, k)
				}
			}
		}
	}
}

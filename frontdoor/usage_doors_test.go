package frontdoor_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/tomoram/code-central/codecentraltest"
	"github.com/tomoram/code-central/core"
)

// Usage (Spec code-central-201): Code Central records each agent MCP
// call itself, and GET /api/usage reads them back as the operations
// table. These drive MCP and the HTTP API of the running service and
// assert only what the read returns.

// usageRead is GET /api/usage's answer.
type usageRead struct {
	Operations []core.UsageOperation `json:"operations"`
	Kinds      []any                 `json:"kinds"`
	Sessions   []any                 `json:"sessions"`
	Days       []any                 `json:"days"`
}

func readUsage(t *testing.T, srv *codecentraltest.Server, query string) usageRead {
	t.Helper()
	status, b := httpDo(t, srv, httpCall{method: "GET", path: "/api/usage" + query})
	if status != http.StatusOK {
		t.Fatalf("GET /api/usage%s: status %d: %s", query, status, b)
	}
	var u usageRead
	if err := json.Unmarshal(b, &u); err != nil {
		t.Fatalf("not the usage shape: %v: %s", err, b)
	}
	return u
}

// acmeServer starts a test service with the one Project acme.
func acmeServer(t *testing.T, opts ...codecentraltest.Option) *codecentraltest.Server {
	t.Helper()
	srv := codecentraltest.NewServer(t, opts...)
	must(t, func() error {
		_, err := srv.Core.CreateProject(context.Background(), core.CreateProjectInput{Key: "acme"})
		return err
	})
	return srv
}

// operation is the row for op, failing the test when there is none.
func operation(t *testing.T, u usageRead, op string) core.UsageOperation {
	t.Helper()
	for _, row := range u.Operations {
		if row.Op == op {
			return row
		}
	}
	t.Fatalf("no row for %s in %+v", op, u.Operations)
	return core.UsageOperation{}
}

func TestUsageCountsEachMCPCallWithItsSizes(t *testing.T) {
	srv := newServer(t)
	s := connect(t, srv, "acme")
	_, first := mcpCall(t, s, "get_ticket", map[string]any{"ticket": 2})
	_, second := mcpCall(t, s, "get_ticket", map[string]any{"ticket": "acme-3"})

	u := readUsage(t, srv, "")
	got := operation(t, u, "get_ticket")
	req := len(`{"ticket":2}`) + len(`{"ticket":"acme-3"}`)
	resp := len(first) + len(second)
	want := core.UsageOperation{
		Op: "get_ticket", Calls: 2,
		RequestBytes: req, AverageRequestBytes: (req + 1) / 2,
		ResponseBytes: resp, AverageResponseBytes: (resp + 1) / 2,
	}
	if got != want {
		t.Fatalf("row %+v, want %+v", got, want)
	}
	if len(u.Operations) != 1 {
		t.Fatalf("rows %+v, want only get_ticket", u.Operations)
	}
	if u.Kinds == nil || u.Sessions == nil || u.Days == nil || len(u.Kinds)+len(u.Sessions)+len(u.Days) != 0 {
		t.Fatalf("kinds, sessions and days should be empty lists, got %+v", u)
	}
}

func TestUsageCountsRejectedCallsAsErrors(t *testing.T) {
	srv := newServer(t)
	s := connect(t, srv, "acme")
	mcpRejected(t, s, "get_ticket", map[string]any{"ticket": "acme-99"}, core.NotFound)
	mcpRejected(t, s, "get_ticket", map[string]any{"nope": 1}, core.Invalid)
	mcpOK(t, s, "get_ticket", map[string]any{"ticket": 2})

	if got := operation(t, readUsage(t, srv, ""), "get_ticket"); got.Calls != 3 || got.Errors != 2 {
		t.Fatalf("row %+v, want 3 calls and 2 errors", got)
	}
}

func TestUsageLeavesOutCallsToAnUnknownProjectAndTheHTTPAPI(t *testing.T) {
	srv := newServer(t)
	mcpRejected(t, connect(t, srv, "nope"), "get_ticket", map[string]any{"ticket": 2}, core.UnknownProject)
	mcpRejected(t, connect(t, srv, "nope"), "get_ticket", map[string]any{"nope": 1}, core.Invalid)
	httpOK(t, srv, httpCall{method: "GET", path: "/api/p/acme/tickets/2"})
	httpOK(t, srv, httpCall{method: "GET", path: "/api/p/acme/tickets"})

	if u := readUsage(t, srv, ""); len(u.Operations) != 0 {
		t.Fatalf("rows %+v, want none", u.Operations)
	}
	// Nothing was filed under the unknown Project, which would otherwise
	// be a folder with no project.yaml in the store.
	if _, err := os.Stat(filepath.Join(srv.UsageDir, "projects", "nope")); !os.IsNotExist(err) {
		t.Fatalf("a folder for the unknown Project: %v", err)
	}
}

func TestUsageReadsOneProjectOrAllOfThemArchivedOnesToo(t *testing.T) {
	srv := newServer(t)
	mcpOK(t, connect(t, srv, "acme"), "get_ticket", map[string]any{"ticket": 2})
	mcpOK(t, connect(t, srv, "other"), "find_tickets", map[string]any{})
	mcpOK(t, connect(t, srv, "other"), "find_tickets", map[string]any{})
	ctx := core.WithActor(context.Background(), core.Human)
	must(t, func() error { _, err := srv.Core.ArchiveProject(ctx, "other"); return err })

	ops := func(u usageRead) map[string]int {
		m := map[string]int{}
		for _, row := range u.Operations {
			m[row.Op] = row.Calls
		}
		return m
	}
	if got := ops(readUsage(t, srv, "?project=acme")); !reflect.DeepEqual(got, map[string]int{"get_ticket": 1}) {
		t.Errorf("acme: %v", got)
	}
	if got := ops(readUsage(t, srv, "?project=other")); !reflect.DeepEqual(got, map[string]int{"find_tickets": 2}) {
		t.Errorf("other: %v", got)
	}
	all := readUsage(t, srv, "")
	if got := ops(all); !reflect.DeepEqual(got, map[string]int{"get_ticket": 1, "find_tickets": 2}) {
		t.Errorf("all: %v", got)
	}
	if all.Operations[0].Op != "find_tickets" {
		t.Errorf("rows %+v, want the most called first", all.Operations)
	}
}

func TestUsageRejectsAnUnknownProjectAndAnyOtherRange(t *testing.T) {
	srv := newServer(t)
	httpRejected(t, srv, httpCall{method: "GET", path: "/api/usage?project=nope"}, core.NotFound)
	for _, days := range []string{"0", "1", "14", "seven"} {
		e := httpRejected(t, srv, httpCall{method: "GET", path: "/api/usage?days=" + days}, core.Invalid)
		if e.Field != "days" {
			t.Errorf("days=%s: field %q, want days", days, e.Field)
		}
	}
	httpRejected(t, srv, httpCall{method: "GET", path: "/api/usage?colour=red"}, core.Invalid)
	for _, days := range []string{"7", "30", "90"} {
		readUsage(t, srv, "?days="+days)
	}
}

// The range is today and the days-1 local days before it, in the
// service's time zone: here UTC+10, where a call at 13:30 UTC is late
// the same local day and the next UTC day starts at 10:00 local.
func TestUsageCountsTheLastDaysInTheServicesLocalTime(t *testing.T) {
	loc := time.FixedZone("AEST", 10*60*60)
	srv := acmeServer(t, codecentraltest.WithLocation(loc))
	call := time.Date(2026, 1, 5, 23, 30, 0, 0, loc)
	srv.Clock.Set(call)
	mcpOK(t, connect(t, srv, "acme"), "find_tickets", map[string]any{})

	calls := func(days string) int {
		u := readUsage(t, srv, "?days="+days)
		if len(u.Operations) == 0 {
			return 0
		}
		return u.Operations[0].Calls
	}
	// The last minute of the seventh local day, and of the thirtieth.
	srv.Clock.Set(time.Date(2026, 1, 11, 23, 59, 0, 0, loc))
	if got := calls("7"); got != 1 {
		t.Errorf("on the seventh day: %d calls, want 1", got)
	}
	// Half an hour into the eighth local day: still the 11th in UTC.
	srv.Clock.Set(time.Date(2026, 1, 12, 0, 30, 0, 0, loc))
	if got := calls("7"); got != 0 {
		t.Errorf("on the eighth day: %d calls, want 0", got)
	}
	if got := calls("30"); got != 1 {
		t.Errorf("30 days on the eighth day: %d calls, want 1", got)
	}
	srv.Clock.Set(time.Date(2026, 2, 3, 23, 59, 0, 0, loc))
	if got := calls("30"); got != 1 {
		t.Errorf("on the thirtieth day: %d calls, want 1", got)
	}
	srv.Clock.Set(time.Date(2026, 2, 4, 0, 0, 0, 0, loc))
	if got := calls("30"); got != 0 {
		t.Errorf("on the thirty-first day: %d calls, want 0", got)
	}
	if got := calls("90"); got != 1 {
		t.Errorf("90 days on the thirty-first day: %d calls, want 1", got)
	}
}

func TestUsageSurvivesARestart(t *testing.T) {
	dir := t.TempDir()
	first := acmeServer(t, codecentraltest.WithUsageDir(dir))
	mcpOK(t, connect(t, first, "acme"), "find_tickets", map[string]any{})

	second := acmeServer(t, codecentraltest.WithUsageDir(dir))
	if got := operation(t, readUsage(t, second, "?project=acme"), "find_tickets"); got.Calls != 1 {
		t.Fatalf("row %+v after a restart, want 1 call", got)
	}
}

func TestUsageSkipsALineItCannotReadAndSaysSoInTheLog(t *testing.T) {
	log := &syncBuffer{}
	srv := acmeServer(t, codecentraltest.WithLog(log))
	mcpOK(t, connect(t, srv, "acme"), "find_tickets", map[string]any{})
	file := filepath.Join(srv.UsageDir, "projects", "acme", "usage", codecentraltest.Start.Format(time.DateOnly)+".jsonl")
	f, err := os.OpenFile(file, os.O_WRONLY|os.O_APPEND, 0)
	if err != nil {
		t.Fatal(err)
	}
	fmt.Fprintln(f, `{"type":"mcp_call","op":"get_t`)
	f.Close()
	mcpOK(t, connect(t, srv, "acme"), "find_tickets", map[string]any{})

	if got := operation(t, readUsage(t, srv, ""), "find_tickets"); got.Calls != 2 {
		t.Fatalf("row %+v, want the 2 readable calls", got)
	}
	if !strings.Contains(log.String(), "skipped 1 unreadable line") {
		t.Fatalf("log %q should count the skipped line", log.String())
	}
}

func TestAFailedUsageAppendStillAnswersTheCall(t *testing.T) {
	log := &syncBuffer{}
	srv := acmeServer(t, codecentraltest.WithLog(log))
	// A file where the usage folder should be, so no append can succeed,
	// on every OS (a read-only folder still takes writes on Windows).
	if err := os.MkdirAll(filepath.Join(srv.UsageDir, "projects", "acme"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(srv.UsageDir, "projects", "acme", "usage"), nil, 0o644); err != nil {
		t.Fatal(err)
	}

	mcpOK(t, connect(t, srv, "acme"), "find_tickets", map[string]any{})
	if !strings.Contains(log.String(), "usage: dropped an mcp_call record for acme") {
		t.Fatalf("log %q should say the record was dropped", log.String())
	}
	if u := readUsage(t, srv, ""); len(u.Operations) != 0 {
		t.Fatalf("rows %+v, want none", u.Operations)
	}
}

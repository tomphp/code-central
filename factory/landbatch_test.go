package factory

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/tomoram/code-central/client"
	"github.com/tomoram/code-central/store"
)

// The Factory reads the Tickets it may land with one find_tickets call
// asking for full (Spec code-central-568), so a poll makes no get_ticket
// call per Accepted Ticket.

// requestLog records the requests that reach Code Central.
type requestLog struct {
	mu   sync.Mutex
	gets []string
}

func (l *requestLog) add(r *http.Request) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.gets = append(l.gets, r.Method+" "+r.URL.RequestURI())
}

// count is the requests whose method and URI satisfy match.
func (l *requestLog) count(match func(string) bool) int {
	l.mu.Lock()
	defer l.mu.Unlock()
	n := 0
	for _, g := range l.gets {
		if match(g) {
			n++
		}
	}
	return n
}

// proxied points the factory at Code Central through a proxy that logs
// each request, and refuses those fail says to with a 503.
func (w *landWorld) proxied(fail func(*http.Request) bool) *requestLog {
	w.t.Helper()
	target, err := url.Parse(w.srv.URL)
	if err != nil {
		w.t.Fatal(err)
	}
	rp := httputil.NewSingleHostReverseProxy(target)
	log := &requestLog{}
	proxy := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		log.add(r)
		if fail != nil && fail(r) {
			http.Error(rw, "down", http.StatusServiceUnavailable)
			return
		}
		rp.ServeHTTP(rw, r)
	}))
	w.t.Cleanup(proxy.Close)
	w.f.client = client.New(proxy.URL)
	return log
}

func isFullFind(r *http.Request) bool {
	return r.Method == "GET" && strings.HasSuffix(r.URL.Path, "/tickets") && r.URL.Query().Get("full") == "true"
}

func TestAPollWithSeveralAcceptedTicketsLandsThemAllThroughTheSingleRead(t *testing.T) {
	t.Parallel()
	w := newLandWorld(t)
	ids := []string{}
	for _, title := range []string{"First", "Second", "Third"} {
		id, _ := w.accepted(title)
		ids = append(ids, id)
	}
	log := w.proxied(nil)

	w.poll()

	for _, id := range ids {
		if tk := w.ticket(id); tk.State != store.Done {
			t.Errorf("%s is %s, want it landed", id, tk.State)
		}
	}
	if n := log.count(func(g string) bool { return strings.HasPrefix(g, "GET ") && strings.Contains(g, "/tickets/") }); n != 0 {
		t.Errorf("%d get_ticket calls during the poll, want none: %v", n, log.gets)
	}
	if n := log.count(func(g string) bool { return strings.Contains(g, "full=true") && strings.Contains(g, "state=Accepted") }); n == 0 {
		t.Errorf("no full find_tickets for Accepted Tickets: %v", log.gets)
	}
}

func TestAFailedBatchReadGivesOneWarningAndLandsNothingThatPoll(t *testing.T) {
	t.Parallel()
	w := newLandWorld(t)
	first, _ := w.accepted("First")
	second, _ := w.accepted("Second")
	var failing atomic.Bool
	failing.Store(true)
	w.proxied(func(r *http.Request) bool { return failing.Load() && isFullFind(r) })

	if got := w.f.landable(context.Background(), store.Accepted); len(got) != 0 {
		t.Errorf("landable gave %d Tickets from a failed read, want none", len(got))
	}
	if n := w.lines("warning: can't find [Accepted] Tickets"); n != 1 {
		t.Errorf("%d warnings from the failed read, want one: %q", n, w.out.String())
	}

	w.poll()
	for _, id := range []string{first, second} {
		if tk := w.ticket(id); tk.State != store.Accepted {
			t.Errorf("%s is %s, want it left Accepted", id, tk.State)
		}
	}

	failing.Store(false)
	w.poll()
	for _, id := range []string{first, second} {
		if tk := w.ticket(id); tk.State != store.Done {
			t.Errorf("%s is %s, want it landed once the read works", id, tk.State)
		}
	}
}

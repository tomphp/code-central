package httpapi_test

import (
	"bufio"
	"context"
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

// sseStream opens GET /api/events and returns a reader positioned after
// the opening comment, so the subscription is live when it returns.
func sseStream(t *testing.T, ctx context.Context, url string) *bufio.Reader {
	t.Helper()
	return openSSE(t, ctx, url+"/api/events", nil)
}

// openSSE opens the SSE stream at url with the request headers, as
// sseStream does.
func openSSE(t *testing.T, ctx context.Context, url string, header http.Header) *bufio.Reader {
	t.Helper()
	req, _ := http.NewRequestWithContext(ctx, "GET", url, nil)
	if header != nil {
		req.Header = header
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { res.Body.Close() })
	if res.StatusCode != 200 || res.Header.Get("Content-Type") != "text/event-stream" {
		t.Fatalf("status %d, content type %q", res.StatusCode, res.Header.Get("Content-Type"))
	}
	r := bufio.NewReader(res.Body)
	if line, err := r.ReadString('\n'); err != nil || !strings.HasPrefix(line, ":") {
		t.Fatalf("opening line %q, %v", line, err)
	}
	r.ReadString('\n') // the blank line ending the comment
	return r
}

// nextEvent reads one SSE event: its event: and data: lines.
func nextEvent(t *testing.T, r *bufio.Reader) (string, string) {
	t.Helper()
	var name, data string
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			t.Fatalf("stream ended: %v", err)
		}
		line = strings.TrimRight(line, "\n")
		switch {
		case line == "":
			return name, data
		case strings.HasPrefix(line, "event: "):
			name = strings.TrimPrefix(line, "event: ")
		case strings.HasPrefix(line, "data: "):
			data = strings.TrimPrefix(line, "data: ")
		}
	}
}

func TestEventsStreamsEveryChangeNoticeAsSSE(t *testing.T) {
	c := core.New(store.NewMemory(), clock.NewFake(time.Date(2026, 11, 7, 9, 0, 0, 0, time.UTC)), &ids.Sequence{})
	mux := http.NewServeMux()
	httpapi.Register(mux, c)
	hs := httptest.NewServer(mux)
	defer hs.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	r := sseStream(t, ctx, hs.URL)
	do(mux, "POST", "/api/projects/create_project", `{"key":"acme"}`)
	do(mux, "POST", "/api/p/acme/create_spec", `{"title":"S"}`)
	do(mux, "POST", "/api/p/acme/create_ticket", `{"spec":1,"title":"A"}`)

	want := [][2]string{
		{"project.changed", `{"project":"acme"}`},
		{"spec.changed", `{"project":"acme","id":"acme-1"}`},
		{"ticket.changed", `{"project":"acme","id":"acme-2"}`},
	}
	for _, w := range want {
		if name, data := nextEvent(t, r); name != w[0] || data != w[1] {
			t.Fatalf("got %s %s, want %s %s", name, data, w[0], w[1])
		}
	}
}

func TestEventsStreamEndsWhenTheRequestContextEnds(t *testing.T) {
	h := newHandler(t)
	ctx, cancel := context.WithCancel(context.Background())
	req := httptest.NewRequest("GET", "/api/events", nil).WithContext(ctx)
	done := make(chan struct{})
	go func() {
		h.ServeHTTP(httptest.NewRecorder(), req)
		close(done)
	}()
	cancel()
	<-done // a hang here fails the test by timeout
}

package world

import (
	"net/http"
	"testing"
	"time"
)

// No Core operation panics on purpose, so this test stands a panicking
// handler in for one.
func TestAHandlersPanicFailsItsRequestAlone(t *testing.T) {
	w := New(time.Date(2026, 11, 14, 9, 0, 0, 0, time.UTC))
	t.Cleanup(w.Drop)
	served := w.handler
	w.handler = http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/panic" {
			panic("a bug in Core")
		}
		served.ServeHTTP(rw, r)
	})

	req, _ := http.NewRequest("GET", "http://localhost/panic", nil)
	if res, err := w.RoundTrip(req); err == nil {
		t.Errorf("a panicking request: status %d, want an error", res.StatusCode)
	}

	req, _ = http.NewRequest("GET", "http://localhost/api/workflow", nil)
	res, err := w.RoundTrip(req)
	if err != nil || res.StatusCode != http.StatusOK {
		t.Fatalf("the next request: %v, want it served", err)
	}
	res.Body.Close()
}

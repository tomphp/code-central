// A world's requests: served in memory, with responses streamed as the
// handler writes them, so the change-notice feed and Transcript streams
// reach the client as they would over a socket.

package world

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"sync"

	"github.com/tomoram/code-central/core"
	"github.com/tomoram/code-central/frontdoor"
)

// RoundTrip serves req through the world's handler as the Actor its
// frontdoor.ActorHeader names, the human when it names none, and answers
// once the handler has written its status. The body streams as the
// handler writes it; closing it, or ending req's context, ends the
// handler.
func (w *World) RoundTrip(req *http.Request) (*http.Response, error) {
	ctx, cancel := context.WithCancel(req.Context())
	in := serverRequest(req.Clone(ctx))
	_, stream := streams.Handler(in)
	r := &request{cancel: cancel, stream: stream}
	w.mu.Lock()
	if w.dropped {
		w.mu.Unlock()
		cancel()
		in.Body.Close()
		return nil, ErrDropped
	}
	w.requests[r] = struct{}{}
	w.mu.Unlock()

	body, pw := io.Pipe()
	rw := &responseWriter{header: http.Header{}, body: pw, written: make(chan struct{})}
	done := make(chan struct{})
	go func() {
		defer func() {
			rw.WriteHeader(http.StatusOK)
			pw.Close()
			cancel()
			w.mu.Lock()
			delete(w.requests, r)
			w.mu.Unlock()
			close(done)
		}()
		w.handler.ServeHTTP(rw, in)
	}()
	// Ending the request fails the handler's next write, and the
	// reader's next read, rather than leaving either blocked.
	go func() {
		select {
		case <-ctx.Done():
			pw.CloseWithError(ctx.Err())
		case <-done:
		}
	}()

	select {
	case <-rw.written:
	case <-ctx.Done():
		// A handler that has returned has written its status before
		// its request ends, so that comes first.
		select {
		case <-rw.written:
		default:
			return nil, ctx.Err()
		}
	}
	return &http.Response{
		Status:        fmt.Sprintf("%d %s", rw.status, http.StatusText(rw.status)),
		StatusCode:    rw.status,
		Proto:         "HTTP/1.1",
		ProtoMajor:    1,
		ProtoMinor:    1,
		Header:        rw.sent,
		Body:          responseBody{body, cancel},
		ContentLength: -1,
		Request:       req,
	}, nil
}

// serverRequest makes a client's request into the one a server hands its
// handler: with a Host (localhost when the URL names none), a body, and
// the human as its Actor when it names none.
func serverRequest(r *http.Request) *http.Request {
	if r.Host == "" {
		r.Host = r.URL.Host
	}
	if r.Host == "" {
		r.Host = "localhost"
	}
	if r.Body == nil {
		r.Body = http.NoBody
	}
	r.RequestURI = r.URL.RequestURI()
	if r.Header.Get(frontdoor.ActorHeader) == "" {
		r.Header.Set(frontdoor.ActorHeader, string(core.Human))
	}
	return r
}

// responseBody is a response's body, whose Close ends its request.
type responseBody struct {
	*io.PipeReader
	cancel context.CancelFunc
}

func (b responseBody) Close() error {
	b.cancel()
	return b.PipeReader.Close()
}

// responseWriter hands what a handler writes to the reader of its
// response's body as it writes it.
type responseWriter struct {
	header http.Header
	body   *io.PipeWriter

	once    sync.Once
	status  int
	sent    http.Header
	written chan struct{}
}

func (rw *responseWriter) Header() http.Header { return rw.header }

// WriteHeader sends the status and the headers set so far; only the
// first call counts.
func (rw *responseWriter) WriteHeader(status int) {
	rw.once.Do(func() {
		rw.status, rw.sent = status, rw.header.Clone()
		close(rw.written)
	})
}

func (rw *responseWriter) Write(b []byte) (int, error) {
	rw.WriteHeader(http.StatusOK)
	return rw.body.Write(b)
}

// Flush sends the status, as each write reaches the reader unbuffered.
func (rw *responseWriter) Flush() { rw.WriteHeader(http.StatusOK) }

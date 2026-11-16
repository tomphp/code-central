// A world's requests: served in memory, with responses streamed as the
// handler writes them, so the change-notice feed and Transcript streams
// reach the client as they would over a socket.

package world

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"runtime/debug"
	"sync"
	"sync/atomic"

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
	rw := &responseWriter{header: http.Header{}, body: pw, unread: &r.unread, written: make(chan struct{})}
	done := make(chan struct{})
	go func() {
		defer func() {
			// A handler's panic fails its request alone, as net/http's
			// does, rather than every world in the process.
			if p := recover(); p != nil {
				err := fmt.Errorf("panic serving %s %s: %v", in.Method, in.URL.Path, p)
				fmt.Fprintf(os.Stderr, "world: %v\n%s", err, debug.Stack())
				rw.fail(err)
				pw.CloseWithError(err)
			}
			rw.WriteHeader(http.StatusOK)
			// It leaves the world before its body ends, so a client
			// that has read the whole body finds it gone (Settled).
			w.mu.Lock()
			delete(w.requests, r)
			w.mu.Unlock()
			pw.Close()
			cancel()
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
	if rw.err != nil {
		return nil, rw.err
	}
	return &http.Response{
		Status:        fmt.Sprintf("%d %s", rw.status, http.StatusText(rw.status)),
		StatusCode:    rw.status,
		Proto:         "HTTP/1.1",
		ProtoMajor:    1,
		ProtoMinor:    1,
		Header:        rw.sent,
		Body:          responseBody{body, &r.unread, cancel},
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
	unread *atomic.Int64
	cancel context.CancelFunc
}

func (b responseBody) Read(p []byte) (int, error) {
	n, err := b.PipeReader.Read(p)
	b.unread.Add(-int64(n))
	return n, err
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
	// unread counts the bytes written, or being written, that the
	// response body's reader has yet to read: an io.Pipe write waits
	// until the reader takes it.
	unread *atomic.Int64

	once    sync.Once
	status  int
	sent    http.Header
	err     error
	written chan struct{}
}

// fail ends a response before its status with err, so RoundTrip answers
// err; after its status it does nothing.
func (rw *responseWriter) fail(err error) {
	rw.once.Do(func() {
		rw.err = err
		close(rw.written)
	})
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
	rw.unread.Add(int64(len(b)))
	n, err := rw.body.Write(b)
	// What a failed write didn't hand over will never be read.
	rw.unread.Add(-int64(len(b) - n))
	return n, err
}

// Flush sends the status, as each write reaches the reader unbuffered.
func (rw *responseWriter) Flush() { rw.WriteHeader(http.StatusOK) }

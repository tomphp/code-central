package cli_test

import (
	"context"
	"io"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/tomoram/code-central/cli"
)

// On shutdown the service lets an in-flight write finish and answers held
// long-polls at once, before it stops.
func TestShutdownFinishesInFlightWritesAndClosesLongPolls(t *testing.T) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	entered := make(chan string, 2)
	release := make(chan struct{})
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		entered <- r.URL.Path
		switch r.URL.Path {
		case "/write":
			<-release // a write still running when shutdown starts
			io.WriteString(w, "written")
		case "/wait":
			<-r.Context().Done() // a long-poll held open
			io.WriteString(w, "closed")
		}
	})
	ctx, cancel := context.WithCancel(context.Background())
	stopped := make(chan error, 1)
	go func() { stopped <- cli.ServeOn(ctx, []net.Listener{l}, handler, nil) }()

	type reply struct {
		body string
		err  error
	}
	get := func(path string) chan reply {
		ch := make(chan reply, 1)
		go func() {
			resp, err := http.Get("http://" + l.Addr().String() + path)
			if err != nil {
				ch <- reply{err: err}
				return
			}
			b, err := io.ReadAll(resp.Body)
			resp.Body.Close()
			ch <- reply{string(b), err}
		}()
		return ch
	}
	write, wait := get("/write"), get("/wait")
	<-entered
	<-entered

	cancel()
	timeout := time.After(5 * time.Second)
	select {
	case r := <-wait:
		if r.err != nil || r.body != "closed" {
			t.Fatalf("long-poll: %q, %v", r.body, r.err)
		}
	case <-timeout:
		t.Fatal("the long-poll was not closed on shutdown")
	}
	select {
	case err := <-stopped:
		t.Fatalf("serve stopped with a write in flight: %v", err)
	default:
	}
	close(release)
	select {
	case r := <-write:
		if r.err != nil || r.body != "written" {
			t.Fatalf("in-flight write: %q, %v", r.body, r.err)
		}
	case <-timeout:
		t.Fatal("the in-flight write never finished")
	}
	select {
	case err := <-stopped:
		if err != nil {
			t.Fatalf("shutdown: %v", err)
		}
	case <-timeout:
		t.Fatal("serve did not stop")
	}
}

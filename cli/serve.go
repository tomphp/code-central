package cli

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"
	"syscall"
	"time"

	"github.com/tomoram/code-central/clock"
	"github.com/tomoram/code-central/core"
	"github.com/tomoram/code-central/frontdoor"
	"github.com/tomoram/code-central/ids"
	"github.com/tomoram/code-central/store"
	"github.com/tomoram/code-central/web"
)

// Port is Code Central's fixed port.
const Port = 7733

// DefaultAddrs are the loopback addresses serve binds: .mcp.json says
// localhost, which may resolve to either. Never 0.0.0.0.
var DefaultAddrs = []string{fmt.Sprintf("127.0.0.1:%d", Port), fmt.Sprintf("[::1]:%d", Port)}

// runServe is the service: lock and load the store, then bind, then serve
// until ctx ends. Startup failures exit 1 with a message on stderr.
func runServe(ctx context.Context, e Env, args []string) error {
	fs := flagSet("serve")
	storeFlag := fs.String("store", "", "store directory")
	pos, err := parse(fs, args)
	if err != nil {
		return err
	}
	if len(pos) > 0 {
		return usagef("unexpected argument %q", pos[0])
	}
	dir, err := store.Dir(*storeFlag, e.Getenv)
	if err != nil {
		return err
	}
	st, err := store.Open(dir)
	if err != nil {
		return fmt.Errorf("cannot open the store: %w", err)
	}
	defer st.Close()

	addrs := e.ServeAddrs
	if l := e.Getenv("CODE_CENTRAL_LISTEN"); addrs == nil && l != "" {
		// For the end-to-end tier, which must never bind the real port.
		addrs = strings.Split(l, ",")
	}
	if addrs == nil {
		addrs = DefaultAddrs
	}
	listeners, err := listen(addrs)
	if err != nil {
		return err
	}

	c := core.New(st, clock.System{}, ids.Random{})
	srv := &http.Server{
		Handler: frontdoor.Handler(c, web.Handler()), ReadHeaderTimeout: 10 * time.Second,
		// Requests share serve's context, so stopping ends held long-polls
		// (each returns its State and `wait` retries until serve is back).
		BaseContext: func(net.Listener) context.Context { return ctx },
	}
	errs := make(chan error, len(listeners))
	bound := make([]string, 0, len(listeners))
	for _, l := range listeners {
		bound = append(bound, l.Addr().String())
		go func() { errs <- srv.Serve(l) }()
	}
	fmt.Fprintf(e.Stdout, "Code Central %s listening on %v (store: %s)\n", Version, bound, dir)
	if e.OnListening != nil {
		e.OnListening(bound)
	}

	select {
	case <-ctx.Done():
	case err := <-errs:
		if !errors.Is(err, http.ErrServerClosed) {
			srv.Close()
			return err
		}
	}
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return srv.Shutdown(shutdownCtx)
}

// listen binds every address, refusing to start if any is taken. An
// address this machine doesn't have (no IPv6 loopback) is skipped.
func listen(addrs []string) ([]net.Listener, error) {
	var out []net.Listener
	closeAll := func() {
		for _, l := range out {
			l.Close()
		}
	}
	for _, a := range addrs {
		l, err := net.Listen("tcp", a)
		if errors.Is(err, syscall.EADDRNOTAVAIL) && len(addrs) > 1 {
			continue
		}
		if errors.Is(err, syscall.EADDRINUSE) {
			closeAll()
			return nil, fmt.Errorf("%s is already in use by another program (is another code-central serve running?); refusing to start", a)
		}
		if err != nil {
			closeAll()
			return nil, fmt.Errorf("cannot listen on %s: %w", a, err)
		}
		out = append(out, l)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("could not bind any of %v", addrs)
	}
	return out, nil
}

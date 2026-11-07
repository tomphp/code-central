package cli

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/tomoram/code-central/clock"
	"github.com/tomoram/code-central/core"
	"github.com/tomoram/code-central/events"
	"github.com/tomoram/code-central/frontdoor"
	"github.com/tomoram/code-central/ids"
	"github.com/tomoram/code-central/store"
	"github.com/tomoram/code-central/web"
)

// Port is Code Central's fixed port. The local `port` setting changes it
// only to work around a real clash.
const Port = 7733

// DefaultAddrs are the loopback addresses serve binds: .mcp.json says
// localhost, which may resolve to either. Never 0.0.0.0.
var DefaultAddrs = loopback(Port)

func loopback(port int) []string {
	p := strconv.Itoa(port)
	return []string{net.JoinHostPort("127.0.0.1", p), net.JoinHostPort("::1", p)}
}

func init() {
	commands["serve"] = command{
		usage: "serve [--store <dir>] [--bind <ip>]  run the service in the foreground; --bind adds one address " +
			"(such as the Docker bridge IP), never a LAN address",
		run: runServe,
	}
}

// serveOptions are serve's flags. Add new serve flags here and in
// parseServe.
type serveOptions struct {
	store string
	// bind is --bind: one extra address, "" for none.
	bind    string
	bindSet bool
}

func parseServe(args []string) (serveOptions, error) {
	fs := flagSet("serve")
	var o serveOptions
	fs.StringVar(&o.store, "store", "", "store directory")
	fs.StringVar(&o.bind, "bind", "", "one extra address to listen on")
	pos, err := parse(fs, args)
	if err != nil {
		return o, err
	}
	if len(pos) > 0 {
		return o, usagef("unexpected argument %q", pos[0])
	}
	o.bindSet = isSet(fs, "bind")
	return o, nil
}

// runServe is the service: read the settings, lock and load the store,
// then bind, then serve until ctx ends. Startup failures exit 1 with a
// message on stderr.
func runServe(ctx context.Context, e Env, args []string) error {
	o, err := parseServe(args)
	if err != nil {
		return err
	}
	settings, err := loadSettings(e.Getenv)
	if err != nil {
		return err
	}
	var bindAddr, bindHost string
	if o.bindSet {
		if bindAddr, bindHost, err = parseBind(o.bind, settings.Port); err != nil {
			return err
		}
	}
	dir, err := store.Dir(o.store, e.Getenv)
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
		addrs = loopback(settings.Port)
	}
	listeners, err := listen(addrs)
	if err != nil {
		return err
	}
	var bindHosts []string
	if bindAddr != "" {
		l, err := listenOne(bindAddr)
		if err != nil {
			closeListeners(listeners)
			return err
		}
		listeners = append(listeners, l)
		bindHosts = append(bindHosts, bindHost)
		fmt.Fprintf(e.Stderr, "code-central serve: --bind: also listening on %s; anything that can reach it can act "+
			"as the human, so use a Docker bridge address, never a LAN address\n", l.Addr())
	}

	c := core.New(st, clock.System{}, ids.Random{})
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	go c.WatchSilent(ctx, ticker.C, func(project, ticketID string) {
		c.Events().Publish(events.Event{Type: events.TicketChanged, Project: project, ID: ticketID})
	})
	handler := frontdoor.Versioned(frontdoor.Handler(c, web.Handler(), bindHosts...), e.version())

	bound := make([]string, 0, len(listeners))
	for _, l := range listeners {
		bound = append(bound, l.Addr().String())
	}
	return serveOn(ctx, listeners, handler, func() {
		fmt.Fprintf(e.Stdout, "Code Central %s listening on %v (store: %s)\n", e.version(), bound, dir)
		if e.OnListening != nil {
			e.OnListening(bound)
		}
	})
}

// shutdownLimit bounds how long serve waits for in-flight requests.
const shutdownLimit = 10 * time.Second

// serveOn serves handler on listeners until ctx ends, then shuts down:
// it stops accepting, lets in-flight requests (writes) finish, and ends
// held long-polls, whose request contexts derive from ctx (each returns
// its current State and `wait` retries until serve is back).
func serveOn(ctx context.Context, listeners []net.Listener, handler http.Handler, started func()) error {
	srv := &http.Server{
		Handler: handler, ReadHeaderTimeout: 10 * time.Second,
		BaseContext: func(net.Listener) context.Context { return ctx },
	}
	errs := make(chan error, len(listeners))
	for _, l := range listeners {
		go func() { errs <- srv.Serve(l) }()
	}
	if started != nil {
		started()
	}
	select {
	case <-ctx.Done():
	case err := <-errs:
		if !errors.Is(err, http.ErrServerClosed) {
			srv.Close()
			return err
		}
	}
	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownLimit)
	defer cancel()
	return srv.Shutdown(shutdownCtx)
}

// parseBind checks --bind: one specific IP address, with an optional port
// (default: the service's port). It is never 0.0.0.0 or ::.
func parseBind(v string, port int) (addr, host string, err error) {
	host, p := v, strconv.Itoa(port)
	if h, pp, err := net.SplitHostPort(v); err == nil {
		host, p = h, pp
	}
	host = strings.Trim(host, "[]")
	ip := net.ParseIP(host)
	if ip == nil {
		return "", "", usagef("--bind takes one IP address, such as the Docker bridge IP, not %q", v)
	}
	if ip.IsUnspecified() {
		return "", "", usagef("--bind never takes %s: it would expose the service to the network; name one specific address", host)
	}
	return net.JoinHostPort(host, p), host, nil
}

// listen binds every address, refusing to start if any is taken. An
// address this machine doesn't have (no IPv6 loopback) is skipped.
func listen(addrs []string) ([]net.Listener, error) {
	var out []net.Listener
	for _, a := range addrs {
		l, err := net.Listen("tcp", a)
		if errors.Is(err, syscall.EADDRNOTAVAIL) && len(addrs) > 1 {
			continue
		}
		if err != nil {
			closeListeners(out)
			return nil, listenError(a, err)
		}
		out = append(out, l)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("could not bind any of %v", addrs)
	}
	return out, nil
}

func listenOne(addr string) (net.Listener, error) {
	l, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, listenError(addr, err)
	}
	return l, nil
}

func listenError(addr string, err error) error {
	if errors.Is(err, syscall.EADDRINUSE) {
		return fmt.Errorf("%s is already in use by another program (is another code-central serve running?); "+
			"refusing to start. To move Code Central, set `port` in the local settings and point agents at it with CODE_CENTRAL_URL", addr)
	}
	return fmt.Errorf("cannot listen on %s: %w", addr, err)
}

func closeListeners(ls []net.Listener) {
	for _, l := range ls {
		l.Close()
	}
}

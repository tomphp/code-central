package cli

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"runtime/debug"
	"strconv"
	"strings"
	"time"

	"github.com/tomoram/code-central/ciwatcher"
	"github.com/tomoram/code-central/clock"
	"github.com/tomoram/code-central/core"
	"github.com/tomoram/code-central/events"
	"github.com/tomoram/code-central/frontdoor"
	"github.com/tomoram/code-central/ids"
	"github.com/tomoram/code-central/issuewatcher"
	"github.com/tomoram/code-central/notify"
	"github.com/tomoram/code-central/store"
	"github.com/tomoram/code-central/usage"
	"github.com/tomoram/code-central/watchers"
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
		usage: "serve [--store <dir>] [--bind <ip>] [--no-notify] [--log <file>]  run the service in the foreground; --bind adds one address " +
			"(such as the Docker bridge IP), never a LAN address; --log appends serve's output to a file",
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
	// noNotify is --no-notify: no OS Notifications this run.
	noNotify bool
	// log is --log: the file serve appends its output to, "" for none.
	// Windows' scheduled task needs it, as its action can't redirect.
	log string
}

func parseServe(args []string) (serveOptions, error) {
	fs := flagSet("serve")
	var o serveOptions
	fs.StringVar(&o.store, "store", "", "store directory")
	fs.StringVar(&o.bind, "bind", "", "one extra address to listen on")
	fs.BoolVar(&o.noNotify, "no-notify", false, "send no OS Notifications this run")
	fs.StringVar(&o.log, "log", "", "file to append output to")
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

// runServe is the service: read the settings, lock the store (rolling
// forward an Import, migrating an older format, loading every file),
// then bind, then serve until ctx ends. Startup failures exit 1 with a
// message on stderr.
func runServe(ctx context.Context, e Env, args []string) (err error) {
	o, err := parseServe(args)
	if err != nil {
		return err
	}
	if o.log != "" {
		f, ferr := os.OpenFile(o.log, os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0o644)
		if ferr != nil {
			return ferr
		}
		defer f.Close()
		// A crash's stack goes there too, and so does the error serve
		// stops with, which Run prints only to the original stderr.
		debug.SetCrashOutput(f, debug.CrashOptions{})
		defer debug.SetCrashOutput(nil, debug.CrashOptions{})
		defer func() {
			if err != nil {
				fmt.Fprintf(f, "code-central serve: %v\n", err)
			}
		}()
		e.Stdout, e.Stderr = f, f
	}
	local, err := loadSettings(e.home())
	if err != nil {
		return err
	}
	var bindAddr, bindHost string
	if o.bindSet {
		if bindAddr, bindHost, err = parseBind(o.bind, local.Port); err != nil {
			return err
		}
	}
	dir, err := store.Dir(o.store, e.home())
	if err != nil {
		return err
	}
	st, err := store.Open(dir)
	if err != nil {
		return fmt.Errorf("cannot open the store: %w", err)
	}
	defer st.Close()
	if m := st.Migrated(); m != nil {
		fmt.Fprintf(e.Stderr, "code-central serve: migrated the store from format %d to %d; the old store is kept at %s\n",
			m.From, m.To, m.Backup)
	}

	// The Watchers run as the service's own executable.
	self, err := os.Executable()
	if err != nil {
		return fmt.Errorf("cannot find the service's own executable, which runs the Watchers: %w", err)
	}

	addrs := e.ServeAddrs
	if l := e.Getenv("CODE_CENTRAL_LISTEN"); addrs == nil && l != "" {
		// For the end-to-end tier, which must never bind the real port.
		addrs = strings.Split(l, ",")
	}
	if addrs == nil {
		addrs = loopback(local.Port)
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

	clk := clock.System{}
	c := core.New(st, clk, ids.Random{})
	c.SetUsage(usage.New(dir, clk, time.Local, e.Stderr))
	c.SetCICheck(ciwatcher.Checker(self, os.Environ()))
	c.SetIssuesCheck(issuewatcher.Checker(self, os.Environ()))
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	go c.WatchTimedReasons(ctx, ticker.C, func(project, ticketID string) {
		c.Events().Publish(events.Event{Type: events.TicketChanged, Project: project, ID: ticketID})
	})
	handler := frontdoor.Versioned(frontdoor.Handler(c, web.Handler(), e.Stderr, bindHosts...), e.version())

	bound := make([]string, 0, len(listeners))
	for _, l := range listeners {
		bound = append(bound, l.Addr().String())
	}
	c.SetUIURL(uiURL(bound[0]))
	// Serve returns once every Watcher has stopped, so none outlives
	// it; one that does, if the service is killed, exits when the pipe
	// that is its stdin closes.
	superviseCtx, stopSupervising := context.WithCancel(ctx)
	supervised := make(chan struct{})
	defer func() {
		stopSupervising()
		<-supervised
	}()
	return serveOn(ctx, listeners, handler, func() {
		go func() {
			defer close(supervised)
			ticker := time.NewTicker(time.Minute)
			defer ticker.Stop()
			watchers.Supervise(superviseCtx, watchers.Supervision{Projects: c, Kinds: []watchers.Kind{ciwatcher.Kind, issuewatcher.Kind}, Program: self, Clock: clk, Tick: ticker.C, Log: e.Stderr,
				Environ: append(os.Environ(), "CODE_CENTRAL_URL=http://"+bound[0])})
		}()
		if e.Notifier != nil && local.Notifications == "on" && !o.noNotify {
			// Before OnListening, so the startup summary has been sent by then.
			notify.Start(ctx, notify.Config{Source: c, Clock: clk, Sender: e.Notifier, BaseURL: uiURL(bound[0]), Log: e.Stderr})
		}
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

// uiURL is the address a Notification click opens: localhost on the
// bound port, as .mcp.json names it.
func uiURL(bound string) string {
	_, port, err := net.SplitHostPort(bound)
	if err != nil {
		port = fmt.Sprint(Port)
	}
	return "http://localhost:" + port
}

// listen binds every address, refusing to start if any is taken. An
// address this machine doesn't have (no IPv6 loopback) is skipped.
func listen(addrs []string) ([]net.Listener, error) {
	var out []net.Listener
	for _, a := range addrs {
		l, err := net.Listen("tcp", a)
		if addrNotAvail(err) && len(addrs) > 1 {
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
	if addrInUse(err) {
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

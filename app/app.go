// Package app is the assembled service: Run opens the store, builds the
// Core and wires its Watcher check modes, usage and timers, serves the
// handler on its listeners, supervises the Watchers and sends
// Notifications, until its context ends. The serve command runs it; it
// is the service's one composition root.
package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"time"

	"github.com/tomoram/code-central/ciwatcher"
	"github.com/tomoram/code-central/clock"
	"github.com/tomoram/code-central/core"
	"github.com/tomoram/code-central/events"
	"github.com/tomoram/code-central/ids"
	"github.com/tomoram/code-central/issuewatcher"
	"github.com/tomoram/code-central/notify"
	"github.com/tomoram/code-central/pullrequestwatcher"
	"github.com/tomoram/code-central/server"
	"github.com/tomoram/code-central/store"
	"github.com/tomoram/code-central/usage"
	"github.com/tomoram/code-central/watchers"
	"github.com/tomoram/code-central/web"
	"github.com/tomoram/code-central/workitemwatcher"
)

// Port is Code Central's fixed port. The local `port` setting changes it
// only to work around a real clash.
const Port = 7733

// Options are what the service is run with. The serve command fills them
// from its flags, the local settings and its environment.
type Options struct {
	// StoreDir is the store directory, already resolved (store.Dir).
	StoreDir string
	// Addrs are the addresses to listen on. One this machine doesn't
	// have (no IPv6 loopback) is skipped; one that is taken stops Run.
	Addrs []string
	// Bind is one extra address to listen on (serve's --bind), whose host
	// the Guard also accepts; "" for none. The caller has checked it names
	// one specific IP, never 0.0.0.0 or :: (serve's parseBind).
	Bind string
	// Version is the service's version, sent on every response.
	Version string
	// Stdout gets the line saying where the service listens; Stderr
	// everything else it logs. nil discards it.
	Stdout, Stderr io.Writer
	// Notifier shows OS Notifications; nil sends none.
	Notifier notify.Sender
	// OnListening, when set, is called with the bound addresses once the
	// service listens.
	OnListening func(addrs []string)
}

// Run is the service: lock the store (rolling forward an Import,
// migrating an older format, loading every file), then bind, then serve
// until ctx ends, and shut down. It returns once every Watcher it
// supervises has stopped.
func Run(ctx context.Context, o Options) error {
	if o.Stdout == nil {
		o.Stdout = io.Discard
	}
	if o.Stderr == nil {
		o.Stderr = io.Discard
	}
	st, err := openStore(o)
	if err != nil {
		return err
	}
	defer st.Close()
	// The Watchers run as the service's own executable.
	self, err := os.Executable()
	if err != nil {
		return fmt.Errorf("cannot find the service's own executable, which runs the Watchers: %w", err)
	}
	listeners, bindHosts, err := listenAll(o)
	if err != nil {
		return err
	}
	clk := clock.System{}
	c := newCore(st, clk, o, self)
	stopTimers := watchTimers(ctx, c)
	defer stopTimers()
	handler := newHandler(c, o, bindHosts)

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
			supervise(superviseCtx, c, clk, self, bound[0], o.Stderr)
		}()
		if o.Notifier != nil {
			// Before OnListening, so the startup summary has been sent by then.
			startNotifying(ctx, c, clk, o, bound[0])
		}
		fmt.Fprintf(o.Stdout, "Code Central %s listening on %v (store: %s)\n", o.Version, bound, o.StoreDir)
		if o.OnListening != nil {
			o.OnListening(bound)
		}
	})
}

// openStore locks and loads the store, saying so when it migrated it.
func openStore(o Options) (*store.FileStore, error) {
	st, err := store.Open(o.StoreDir)
	if err != nil {
		return nil, fmt.Errorf("cannot open the store: %w", err)
	}
	if m := st.Migrated(); m != nil {
		fmt.Fprintf(o.Stderr, "code-central serve: migrated the store from format %d to %d; the old store is kept at %s\n",
			m.From, m.To, m.Backup)
	}
	return st, nil
}

// listenAll binds o.Addrs and then o.Bind, warning about the latter.
// bindHosts is the Bind address's host, for the Guard.
func listenAll(o Options) (listeners []net.Listener, bindHosts []string, err error) {
	listeners, err = listen(o.Addrs)
	if err != nil {
		return nil, nil, err
	}
	if o.Bind == "" {
		return listeners, nil, nil
	}
	host, _, err := net.SplitHostPort(o.Bind)
	if err != nil {
		closeListeners(listeners)
		return nil, nil, err
	}
	l, err := listenOne(o.Bind)
	if err != nil {
		closeListeners(listeners)
		return nil, nil, err
	}
	fmt.Fprintf(o.Stderr, "code-central serve: --bind: also listening on %s; anything that can reach it can act "+
		"as the human, so use a Docker bridge address, never a LAN address\n", l.Addr())
	return append(listeners, l), []string{host}, nil
}

// newCore is the Core on st, with usage and the Watchers' check modes,
// which run as self.
func newCore(st store.Store, clk clock.System, o Options, self string) *core.Core {
	c := core.New(st, clk, ids.Random{})
	c.SetUsage(usage.NewRecorder(st.Usage(), clk, time.Local, o.Stderr))
	c.SetCICheck(ciwatcher.Checker(self, os.Environ()))
	c.SetIssuesCheck(issuewatcher.Checker(self, os.Environ()))
	c.SetAzureBoardsCheck(workitemwatcher.Checker(self, os.Environ()))
	c.SetPullRequestCheck(pullrequestwatcher.Checker(self, os.Environ()))
	return c
}

// watchTimers publishes the changes time alone makes, timed reasons and
// the Factory's status, until ctx ends; stop stops their tickers.
func watchTimers(ctx context.Context, c *core.Core) (stop func()) {
	ticker := time.NewTicker(time.Minute)
	go c.WatchTimedReasons(ctx, ticker.C, func(project, ticketID string) {
		c.Events().Publish(events.Event{Type: events.TicketChanged, Project: project, ID: ticketID})
	})
	factoryTicker := time.NewTicker(10 * time.Second)
	go c.WatchFactoryStatus(ctx, factoryTicker.C, func(project string) {
		c.Events().Publish(events.Event{Type: events.ProjectChanged, Project: project})
	})
	return func() {
		ticker.Stop()
		factoryTicker.Stop()
	}
}

// newHandler is the service's handler: the API, MCP and the UI, behind
// the Guard, stamped with the version.
func newHandler(c *core.Core, o Options, bindHosts []string) http.Handler {
	return server.Versioned(server.Handler(c, web.Handler(), o.Stderr, bindHosts...), o.Version)
}

// supervise runs every kind of Watcher the Projects configure, as self,
// pointed at the service on bound, until ctx ends.
func supervise(ctx context.Context, c *core.Core, clk clock.System, self, bound string, log io.Writer) {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	watchers.Supervise(ctx, watchers.Supervision{
		Projects: c,
		Kinds:    []watchers.Kind{ciwatcher.Kind, issuewatcher.Kind, pullrequestwatcher.Kind, workitemwatcher.Kind},
		Program:  self, Clock: clk, Tick: ticker.C, Log: log,
		Environ: append(os.Environ(), "CODE_CENTRAL_URL=http://"+bound),
	})
}

// startNotifying sends the startup summary and then Notifications as
// Needs Me changes, until ctx ends.
func startNotifying(ctx context.Context, c *core.Core, clk clock.System, o Options, bound string) {
	notify.Start(ctx, notify.Config{Source: c, Clock: clk, Sender: o.Notifier, BaseURL: uiURL(bound), Log: o.Stderr})
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

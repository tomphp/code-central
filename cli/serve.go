package cli

import (
	"context"
	"fmt"
	"net"
	"os"
	"runtime/debug"
	"strconv"
	"strings"

	"github.com/tomoram/code-central/app"
	"github.com/tomoram/code-central/store"
)

// DefaultAddrs are the loopback addresses serve binds: .mcp.json says
// localhost, which may resolve to either. Never 0.0.0.0.
var DefaultAddrs = loopback(app.Port)

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

// runServe is the serve command: it reads the flags and the local
// settings, then runs the service (app.Run) until ctx ends. Startup
// failures exit 1 with a message on stderr.
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
	var bind string
	if o.bindSet {
		if bind, err = parseBind(o.bind, local.Port); err != nil {
			return err
		}
	}
	dir, err := store.Dir(o.store, e.home())
	if err != nil {
		return err
	}
	addrs := e.ServeAddrs
	if l := e.Getenv("CODE_CENTRAL_LISTEN"); addrs == nil && l != "" {
		// For the end-to-end tier, which must never bind the real port.
		addrs = strings.Split(l, ",")
	}
	if addrs == nil {
		addrs = loopback(local.Port)
	}
	notifier := e.Notifier
	if local.Notifications != "on" || o.noNotify {
		notifier = nil
	}
	return app.Run(ctx, app.Options{
		StoreDir: dir, Addrs: addrs, Bind: bind, Version: e.version(),
		Stdout: e.Stdout, Stderr: e.Stderr, Notifier: notifier, OnListening: e.OnListening,
	})
}

// parseBind checks --bind: one specific IP address, with an optional port
// (default: the service's port). It is never 0.0.0.0 or ::.
func parseBind(v string, port int) (addr string, err error) {
	host, p := v, strconv.Itoa(port)
	if h, pp, err := net.SplitHostPort(v); err == nil {
		host, p = h, pp
	}
	host = strings.Trim(host, "[]")
	ip := net.ParseIP(host)
	if ip == nil {
		return "", usagef("--bind takes one IP address, such as the Docker bridge IP, not %q", v)
	}
	if ip.IsUnspecified() {
		return "", usagef("--bind never takes %s: it would expose the service to the network; name one specific address", host)
	}
	return net.JoinHostPort(host, p), nil
}

// Package notify sends OS Notifications when an agent is stopped and
// waiting on the human: a new Round, a new Failure Report, or a Claim
// turning Silent. It listens to core's change notices and, because Silent
// Claims are computed and produce no notice, also looks at every Ticket on
// a timer. Events are gathered over Window into one Notification or one
// summary. At startup it sends one summary if Needs Me is not empty.
package notify

import (
	"context"
	"fmt"
	"io"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/tomoram/code-central/clock"
	"github.com/tomoram/code-central/core"
	"github.com/tomoram/code-central/events"
)

// Window is how long notify gathers events into one Notification.
const Window = 15 * time.Second

// Tick is how often notify looks at every Ticket for a Claim turning
// Silent (and for anything a dropped change notice hid).
const Tick = time.Minute

// Notification is one OS Notification. URL is opened on a click where the
// platform allows it.
type Notification struct {
	Title string
	Body  string
	URL   string
}

// Sender shows a Notification. It must not wait for the human.
type Sender interface {
	Send(Notification) error
}

// Source is what notify reads; *core.Core is one.
type Source interface {
	Events() *events.Bus
	Attention(context.Context) []core.AttentionTicket
	AttentionOf(ctx context.Context, project, id string) (core.AttentionTicket, bool)
}

// Config wires notify.
type Config struct {
	Source Source
	Clock  clock.Clock
	Sender Sender
	// BaseURL is the UI's address, such as http://localhost:7733.
	BaseURL string
	// Log receives failures to send; nil discards them.
	Log io.Writer
}

// The reasons that notify.
type reason int

const (
	newRound reason = iota
	newFailure
	silentClaim
)

type event struct {
	reason reason
	ticket core.AttentionTicket
}

// Start takes its first look at every Ticket, sends the startup summary if
// Needs Me is not empty, and then watches in the background until ctx
// ends. It returns once the startup summary is sent.
func Start(ctx context.Context, cfg Config) {
	if cfg.Log == nil {
		cfg.Log = io.Discard
	}
	w := &watcher{cfg: cfg, known: map[string]core.AttentionTicket{}}
	// Subscribe before the first look so no change falls between them.
	notices, stop := cfg.Source.Events().Subscribe()
	all := cfg.Source.Attention(ctx)
	for _, t := range all {
		w.known[t.ID] = t
	}
	w.startup(all)
	go func() {
		defer stop()
		w.run(ctx, notices)
	}()
}

type watcher struct {
	cfg     Config
	known   map[string]core.AttentionTicket // by display ID
	pending []event
}

func (w *watcher) run(ctx context.Context, notices <-chan events.Event) {
	tick := w.cfg.Clock.After(Tick)
	var window <-chan time.Time
	for {
		select {
		case <-ctx.Done():
			return
		case e, ok := <-notices:
			if !ok {
				return
			}
			w.notice(ctx, e)
		case <-tick:
			for _, t := range w.cfg.Source.Attention(ctx) {
				w.observe(t)
			}
			tick = w.cfg.Clock.After(Tick)
		case <-window:
			// Every notice published before the window closed is already
			// queued; take them all before sending.
			for drained := false; !drained; {
				select {
				case e := <-notices:
					w.notice(ctx, e)
				default:
					drained = true
				}
			}
			w.flush()
			window = nil
		}
		if window == nil && len(w.pending) > 0 {
			window = w.cfg.Clock.After(Window)
		}
	}
}

func (w *watcher) notice(ctx context.Context, e events.Event) {
	if e.Type != events.TicketChanged {
		return
	}
	if t, ok := w.cfg.Source.AttentionOf(ctx, e.Project, e.ID); ok {
		w.observe(t)
	}
}

// observe compares t with the last look at it and gathers what is new.
func (w *watcher) observe(t core.AttentionTicket) {
	was := w.known[t.ID]
	w.known[t.ID] = t
	for range t.Rounds - was.Rounds {
		w.pending = append(w.pending, event{newRound, t})
	}
	for range t.FailureReports - was.FailureReports {
		w.pending = append(w.pending, event{newFailure, t})
	}
	if t.SilentClaim != 0 && t.SilentClaim != was.SilentClaim {
		w.pending = append(w.pending, event{silentClaim, t})
	}
}

func (w *watcher) flush() {
	if len(w.pending) == 0 {
		return
	}
	var n Notification
	if len(w.pending) == 1 {
		n = w.single(w.pending[0])
	} else {
		n = w.summary(w.pending)
	}
	w.pending = nil
	w.send(n)
}

func (w *watcher) send(n Notification) {
	if err := w.cfg.Sender.Send(n); err != nil {
		fmt.Fprintf(w.cfg.Log, "notify: %v\n", err)
	}
}

func (w *watcher) single(e event) Notification {
	title := map[reason]string{
		newRound:    "%s has a new Round",
		newFailure:  "%s has a new Failure Report",
		silentClaim: "%s has a Silent Claim",
	}[e.reason]
	return Notification{
		Title: fmt.Sprintf(title, e.ticket.ID), Body: e.ticket.Title,
		URL: w.cfg.BaseURL + "/?view=inbox&ticket=" + url.QueryEscape(e.ticket.ID),
	}
}

func (w *watcher) summary(es []event) Notification {
	var counts [3]int
	var projects []string
	for _, e := range es {
		counts[e.reason]++
		projects = append(projects, e.ticket.Project)
	}
	var parts []string
	for r, words := range [3][2]string{
		{"new Round", "new Rounds"}, {"failure", "failures"}, {"Silent Claim", "Silent Claims"},
	} {
		if n := counts[r]; n > 0 {
			parts = append(parts, plural(n, words[0], words[1]))
		}
	}
	return w.needsMe(strings.Join(parts, ", ") + where(projects))
}

// startup is the one summary sent at startup when Needs Me is not empty.
// Claims already Silent go into it rather than each notifying.
func (w *watcher) startup(all []core.AttentionTicket) {
	var projects []string
	for _, t := range all {
		if t.NeedsMe {
			projects = append(projects, t.Project)
		}
	}
	if len(projects) == 0 {
		return
	}
	verb := " need you"
	if len(projects) == 1 {
		verb = " needs you"
	}
	w.send(w.needsMe(plural(len(projects), "Ticket", "Tickets") + verb + where(projects)))
}

// needsMe is a summary Notification; a click opens Needs Me.
func (w *watcher) needsMe(body string) Notification {
	return Notification{Title: "Code Central", Body: body, URL: w.cfg.BaseURL + "/?view=inbox"}
}

// where is " in acme" for one Project, else " across 3 Projects".
func where(projects []string) string {
	slices.Sort(projects)
	projects = slices.Compact(projects)
	if len(projects) == 1 {
		return " in " + projects[0]
	}
	return fmt.Sprintf(" across %d Projects", len(projects))
}

func plural(n int, one, many string) string {
	if n == 1 {
		return "1 " + one
	}
	return fmt.Sprintf("%d %s", n, many)
}

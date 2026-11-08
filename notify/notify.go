// Package notify sends OS Notifications when an agent is stopped and
// waiting on the human: a new Round, a new Failure Report, a Claim
// turning Silent, or a Claim turning Long (once per Claim). It listens to core's change notices and also looks at
// every Ticket on its own timer, as the spec has it: serve's
// core.WatchTimedReasons does publish a notice when a Claim turns Silent, but
// the events bus drops notices for a slow subscriber, and the timer is
// what still catches a crossing (or a Round) whose notice was dropped,
// and works wherever WatchTimedReasons isn't running. Events are gathered over Window into one Notification or one
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

// Tick is how often notify looks at every Ticket: for a Claim turning
// Silent and for anything a dropped change notice hid.
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
	longClaim
)

// reasons is each reason's wording: the one-event title (%s is the
// Ticket ID) and how a summary counts it.
var reasons = [...]struct{ title, one, many string }{
	newRound:    {"%s has a new Round", "new Round", "new Rounds"},
	newFailure:  {"%s has a new Failure Report", "failure", "failures"},
	silentClaim: {"%s has a Silent Claim", "Silent Claim", "Silent Claims"},
	longClaim:   {"%s has a Long Claim", "Long Claim", "Long Claims"},
}

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
	w := &watcher{cfg: cfg, known: map[string]core.AttentionTicket{}, long: map[longKey]bool{}}
	// Subscribe before the first look so no change falls between them.
	notices, stop := cfg.Source.Events().Subscribe()
	all := cfg.Source.Attention(ctx)
	for _, t := range all {
		w.known[t.ID] = t
		if t.LongClaim != 0 {
			w.long[longKey{t.ID, t.LongClaim}] = true
		}
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
	// long is every Claim already counted as Long: one Notification each,
	// however often it stops and starts being Long.
	long map[longKey]bool
}

// longKey names one Claim by its Ticket's display ID and its number.
type longKey struct {
	ticket string
	claim  int
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
	if k := (longKey{t.ID, t.LongClaim}); t.LongClaim != 0 && !w.long[k] {
		w.long[k] = true
		w.pending = append(w.pending, event{longClaim, t})
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
	return Notification{
		Title: fmt.Sprintf(reasons[e.reason].title, e.ticket.ID), Body: e.ticket.Title,
		URL: w.cfg.BaseURL + "/?view=inbox&ticket=" + url.QueryEscape(e.ticket.ID),
	}
}

func (w *watcher) summary(es []event) Notification {
	var counts [len(reasons)]int
	var projects []string
	for _, e := range es {
		counts[e.reason]++
		projects = append(projects, e.ticket.Project)
	}
	var parts []string
	for r, words := range reasons {
		if n := counts[r]; n > 0 {
			parts = append(parts, plural(n, words.one, words.many))
		}
	}
	return w.needsMe(strings.Join(parts, ", ") + where(projects))
}

// startup is the one summary sent at startup when Needs Me is not empty.
// Claims already Silent or Long go into it rather than each notifying.
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

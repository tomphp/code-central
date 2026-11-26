// The settled signal: what the Factory under test tells the rig of
// its work in flight (factory.Work), and the waits built on it, Settle,
// Tick and Poll. The package doc says what settled means.

package factorytest

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/tomoram/code-central/factory"
)

// tracker hears of one Running Factory's work in flight: it is the
// Factory's factory.Work.
type tracker struct {
	h *Rig

	mu sync.Mutex
	// started is set once the Factory has told of its first work.
	started bool
	next    int
	open    map[int]*piece
	// pollWait is the open poll wait, or nil while the poll loop isn't
	// waiting for a poll.
	pollWait *piece
	// graces is how many stop steps each running turn has been sent, by
	// its key: one per kill grace since its claude started.
	graces map[string]int
	// signalsSent and signalWaitsEnded count the signals the test sent
	// and the waits for a signal that have ended: the Factory has taken
	// every signal once the second has caught up with the first.
	signalsSent, signalWaitsEnded int
}

// piece is one piece of the Factory's work in flight.
type piece struct {
	name    string
	resting bool
	// until is the deadline of the timer resting work waits on, or zero.
	until time.Time
	// turn is the key of the turn whose claude this is the running of.
	turn string
	// answered marks a long-poll whose answer is on its way to the
	// Factory through the proxy.
	answered bool
}

func newTracker(h *Rig) *tracker {
	return &tracker{h: h, open: map[int]*piece{}, graces: map[string]int{}}
}

// Busy is factory.Work's.
func (k *tracker) Busy(name string) func() { return k.add(&piece{name: name}) }

// Rest is factory.Work's.
func (k *tracker) Rest(name string, until time.Time) func() {
	p := &piece{name: name, resting: true, until: until}
	k.mu.Lock()
	if key, ok := cutAround(name, factory.TurnRunning, factory.TurnRunningEnd); ok {
		p.turn = key
		k.graces[key] = 0
	}
	if key, ok := strings.CutPrefix(name, factory.KillGrace); ok {
		k.graces[key]++
	}
	k.mu.Unlock()
	end := k.add(p)
	if p.turn == "" {
		return end
	}
	// A claude killed in a Wait step leaves its marker: it goes with the
	// run, which has exited by the time its work ends.
	return func() {
		end()
		k.h.removeMarker(p.turn)
	}
}

// PollWait is factory.Work's.
func (k *tracker) PollWait(deadline time.Time) func() {
	p := &piece{name: "waiting for the next poll", resting: true, until: deadline}
	end := k.add(p)
	k.mu.Lock()
	k.pollWait = p
	k.mu.Unlock()
	return func() {
		k.mu.Lock()
		if k.pollWait == p {
			k.pollWait = nil
		}
		k.mu.Unlock()
		end()
	}
}

// add opens p and returns the func that ends it, which ends it once
// however many times it is called.
func (k *tracker) add(p *piece) func() {
	k.mu.Lock()
	k.started = true
	k.next++
	id := k.next
	k.open[id] = p
	k.mu.Unlock()
	k.h.changed()
	var once sync.Once
	return func() {
		once.Do(func() {
			k.mu.Lock()
			delete(k.open, id)
			if p.name == factory.SignalWait {
				k.signalWaitsEnded++
			}
			k.mu.Unlock()
			k.h.changed()
		})
	}
}

// cutAround is s between prefix and suffix, and whether s has both.
func cutAround(s, prefix, suffix string) (string, bool) {
	s, ok := strings.CutPrefix(s, prefix)
	if !ok {
		return "", false
	}
	return strings.CutSuffix(s, suffix)
}

// answered marks the latest open long-poll called name as answered: its
// answer is on its way to the Factory.
func (k *tracker) answered(name string) {
	k.mu.Lock()
	latest := 0
	for id, p := range k.open {
		if p.name == name && id > latest {
			latest = id
		}
	}
	if latest != 0 {
		k.open[latest].answered = true
	}
	k.mu.Unlock()
	k.h.changed()
}

// signal counts a signal the test sends.
func (k *tracker) signal() {
	k.mu.Lock()
	k.signalsSent++
	k.mu.Unlock()
	k.h.changed()
}

// unsettled is every condition of settled that doesn't hold yet, and
// whether one of them is outside the Factory, in the fake's files or in
// Code Central, which tell the rig of no change.
func (k *tracker) unsettled() (unmet []string, outside bool) {
	now := k.h.Clock.Now()
	k.mu.Lock()
	if !k.started {
		k.mu.Unlock()
		return []string{"the Factory hasn't started"}, false
	}
	var turns []string
	graces := map[string]int{}
	var wakePolls, chatPolls int
	for _, p := range k.open {
		switch {
		case !p.resting:
			unmet = append(unmet, p.name)
		case !p.until.IsZero() && !p.until.After(now):
			unmet = append(unmet, p.name+", due at "+p.until.Format(time.TimeOnly))
		case p.answered:
			unmet = append(unmet, "the answer to the "+p.name+" on its way")
		case p.turn != "":
			turns = append(turns, p.turn)
			graces[p.turn] = k.graces[p.turn]
		case p.name == factory.WakeLongPoll:
			wakePolls++
		case p.name == factory.ChatLongPoll:
			chatPolls++
		}
	}
	if k.signalsSent > k.signalWaitsEnded {
		unmet = append(unmet, "a signal not yet taken")
	}
	k.mu.Unlock()

	for _, key := range turns {
		noted, parked := k.h.parked(key)
		switch {
		case !parked:
			unmet = append(unmet, "turn "+key+" not in a Wait step")
			outside = true
		case noted < graces[key]:
			unmet = append(unmet, "turn "+key+" yet to take its stop signal")
			outside = true
		}
	}
	if why := k.h.longPollsUnsettled(wakePolls, chatPolls); why != "" {
		unmet = append(unmet, why)
		outside = true
	}
	if reqs := k.h.inflight.busy(time.Now()); len(reqs) > 0 {
		unmet = append(unmet, "requests through the proxy: "+strings.Join(reqs, ", "))
	}
	sort.Strings(unmet)
	return unmet, outside
}

// polling is the deadline of the poll the poll loop waits for, and false
// while it isn't waiting for one.
func (k *tracker) polling() (time.Time, bool) {
	k.mu.Lock()
	defer k.mu.Unlock()
	if k.pollWait == nil {
		return time.Time{}, false
	}
	return k.pollWait.until, true
}

// inKillGrace reports whether a turn being stopped is in a kill grace.
func (k *tracker) inKillGrace() bool {
	k.mu.Lock()
	defer k.mu.Unlock()
	for _, p := range k.open {
		if strings.HasPrefix(p.name, factory.KillGrace) {
			return true
		}
	}
	return false
}

// changes is closed at the next change to what any Factory of the
// rig has told, or to the requests through its proxy.
func (h *Rig) changes() <-chan struct{} {
	h.changeMu.Lock()
	defer h.changeMu.Unlock()
	if h.change == nil {
		h.change = make(chan struct{})
	}
	return h.change
}

// changed wakes every wait on changes.
func (h *Rig) changed() {
	h.changeMu.Lock()
	defer h.changeMu.Unlock()
	if h.change != nil {
		close(h.change)
		h.change = nil
	}
}

// marker is the file the fake claude keeps while a run of key is in a
// Wait step.
func (h *Rig) marker(key string) string { return filepath.Join(h.data, "waiting", key) }

// parked reports whether a run of key is in a Wait step, and how many
// stop signals it has noted.
func (h *Rig) parked(key string) (noted int, ok bool) {
	b, err := readFile(h.marker(key))
	if err != nil {
		return 0, false
	}
	// A marker being written may read empty for a moment.
	n, err := strconv.Atoi(strings.TrimSpace(string(b)))
	return n, err == nil
}

// removeMarker removes key's marker, retrying while Windows holds it.
func (h *Rig) removeMarker(key string) {
	deadline := time.Now().Add(2 * time.Second)
	for {
		err := os.Remove(h.marker(key))
		if err == nil || errors.Is(err, os.ErrNotExist) || !inUse(err) || time.Now().After(deadline) {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// Settle waits for the Factory to settle, or to exit, and fails the
// test after Timeout naming what is still in flight.
func (r *Running) Settle() {
	r.h.t.Helper()
	if unmet := r.settle(Timeout); unmet != nil {
		r.h.t.Fatalf("the Factory didn't settle: %s", strings.Join(unmet, "; "))
	}
}

// settle waits up to within for the Factory to settle or exit, and
// returns what is still in flight if it hasn't. It waits on the
// Factory's own reports, and reads the fake's markers and Code Central's
// long-polls again every few milliseconds only while it waits on them.
func (r *Running) settle(within time.Duration) []string {
	timeout := time.After(within)
	for {
		changes := r.h.changes()
		if r.hasExited() {
			return nil
		}
		unmet, outside := r.track.unsettled()
		if unmet == nil {
			return nil
		}
		var again <-chan time.Time
		if outside {
			again = time.After(5 * time.Millisecond)
		}
		select {
		case <-r.done:
			return nil
		case <-changes:
		case <-again:
		case <-timeout:
			return unmet
		}
	}
}

// hasExited reports whether the Factory has exited.
func (r *Running) hasExited() bool {
	select {
	case <-r.done:
		return true
	default:
		return false
	}
}

// waitPoll settles the Factory, moving the clock to each timer due while
// a poll is in progress, such as a claim retry pause or the kill grace of
// a turn the poll stops, until the poll loop waits for its next poll, or
// a draining one for its next beat. It returns that deadline, and false
// once the Factory has exited.
func (r *Running) waitPoll() (time.Time, bool) {
	r.h.t.Helper()
	deadline := time.Now().Add(Timeout)
	for {
		r.Settle()
		changes := r.h.changes()
		if r.hasExited() {
			return time.Time{}, false
		}
		if at, ok := r.track.polling(); ok {
			return at, true
		}
		if next, ok := r.h.Clock.Next(); ok {
			r.h.Clock.Set(next)
			continue
		}
		// No timer: the Factory is on its way out, or a turn it stops
		// lingers.
		select {
		case <-changes:
		case <-r.done:
		case <-time.After(time.Until(deadline)):
			r.h.t.Fatalf("the Factory's poll waits on something no timer ends: %s", r.track.describe())
		}
	}
}

// Tick lets the Factory make its next poll. Once the Factory has settled
// and its poll loop waits, it moves the clock to each timer due before
// the poll, earliest first, settling after each, and then to the poll's
// deadline, firing the poll and every timer due with it. A poll in
// progress is finished first, passing the claim retry pause or kill
// grace it waits on. Tick returns as the poll starts.
func (r *Running) Tick() {
	r.h.t.Helper()
	at, ok := r.waitPoll()
	if !ok {
		return
	}
	for {
		next, ok := r.h.Clock.Next()
		if !ok || !next.Before(at) {
			break
		}
		r.h.Clock.Set(next)
		r.Settle()
	}
	r.h.Clock.Set(at)
}

// Poll is Tick, then waits for the poll's consequences to finish: it
// returns once the poll loop waits again and the Factory has settled.
// Turns the poll started may still be running, parked in a Wait step.
func (r *Running) Poll() {
	r.h.t.Helper()
	r.Tick()
	r.waitPoll()
}

// PollUntil polls until cond holds, failing the test after Timeout. The
// Factory posts its Session Events, token usage and Chat items in batches
// on its clock, and each poll moves the clock past a batch, so cond can
// wait on what a post makes true; a turn's claude printing is an outside
// event, so what it printed may reach a later batch.
func (r *Running) PollUntil(what string, cond func() bool) {
	r.h.t.Helper()
	deadline := time.Now().Add(Timeout)
	for !cond() {
		if time.Now().After(deadline) {
			r.h.t.Fatalf("timed out polling for %s", what)
		}
		r.Poll()
	}
}

// PassKillGrace waits for a turn being stopped to be in its kill grace,
// settles, and moves the clock past the grace, taking the turn to its
// next stop step.
func (r *Running) PassKillGrace() {
	r.h.t.Helper()
	timeout := time.After(Timeout)
	for !r.track.inKillGrace() {
		changes := r.h.changes()
		if r.track.inKillGrace() {
			break
		}
		select {
		case <-changes:
		case <-r.done:
			r.h.t.Fatal("the Factory exited before a turn's kill grace")
		case <-timeout:
			r.h.t.Fatal("timed out waiting for a turn's kill grace")
		}
	}
	r.Settle()
	r.h.Clock.Advance(KillGrace)
}

// exited waits for the Factory to exit, and returns its exit code. Each
// time the Factory settles it moves the clock to the next timer, such as
// a kill grace or a claim retry pause, but never to the poll the poll
// loop waits for. It fails the test with failure after Timeout.
func (r *Running) exited(failure string) int {
	r.h.t.Helper()
	deadline := time.Now().Add(Timeout)
	for {
		unmet := r.settle(time.Until(deadline))
		if r.hasExited() {
			return r.code
		}
		if unmet != nil {
			// What is open, with the clock's time, says whether a due timer
			// went unheard or a turn never parked, which unmet alone can't.
			r.h.t.Fatalf("%s: still in flight: %s; open work %s at clock %s; stdout:\n%s", failure, strings.Join(unmet, "; "), r.track.describe(), r.h.Clock.Now().Format(time.TimeOnly), r.stdout)
		}
		changes := r.h.changes()
		next, ok := r.h.Clock.Next()
		if poll, polling := r.track.polling(); ok && !(polling && next.Equal(poll)) {
			r.h.Clock.Set(next)
			continue
		}
		select {
		case <-r.done:
		case <-changes:
		case <-time.After(time.Until(deadline)):
			r.h.t.Fatalf("%s; stdout:\n%s", failure, r.stdout)
		}
	}
}

// started waits for the Factory to tell of its first work, or to exit.
func (r *Running) started() {
	r.h.t.Helper()
	timeout := time.After(Timeout)
	for {
		changes := r.h.changes()
		r.track.mu.Lock()
		started := r.track.started
		r.track.mu.Unlock()
		if started {
			return
		}
		select {
		case <-changes:
		case <-r.done:
			return
		case <-timeout:
			r.h.t.Fatal("the Factory didn't start")
		}
	}
}

// longPolls is what the proxy knows of the Factory's long-polls.
type longPolls struct {
	mu sync.Mutex
	// upstream is, by long-poll name, the calls sent on to Code Central
	// whose answer hasn't come back.
	upstream map[string]int
	// parked is the next_factory_wake answers held until PassWakes.
	parked int
}

// longPollOf names the long-poll r is, or "" when it isn't one.
func longPollOf(path string) string {
	switch {
	case strings.HasSuffix(path, wakeSuffix):
		return factory.WakeLongPoll
	case strings.HasSuffix(path, chatWorkSuffix):
		return factory.ChatLongPoll
	}
	return ""
}

// longPollsUnsettled says why the Factory's open long-polls, wakes of
// next_factory_wake and chats of next_chat_work, aren't settled, or ""
// when they are: each must be open at Code Central and unanswered, or,
// for a wake, answered and held until PassWakes. A wake's call is open
// at Code Central once Code Central holds as many unwoken calls as the
// proxy has sent on.
func (h *Rig) longPollsUnsettled(wakes, chats int) string {
	h.polls.mu.Lock()
	upWakes, upChats, parked := h.polls.upstream[factory.WakeLongPoll], h.polls.upstream[factory.ChatLongPoll], h.polls.parked
	h.polls.mu.Unlock()
	if wakes > parked {
		if upWakes < wakes-parked || h.Server.Core.Factories().WaitingFactoryWakes(h.Project) < upWakes {
			return "the " + factory.WakeLongPoll + " not open at Code Central"
		}
	}
	if chats > upChats {
		return "the " + factory.ChatLongPoll + " not open at Code Central"
	}
	return ""
}

// sendLongPoll counts a long-poll called name sent on to Code Central
// until done is called with whether its answer is to be parked.
func (h *Rig) sendLongPoll(name string) (done func(park bool)) {
	h.polls.mu.Lock()
	if h.polls.upstream == nil {
		h.polls.upstream = map[string]int{}
	}
	h.polls.upstream[name]++
	h.polls.mu.Unlock()
	h.changed()
	return func(park bool) {
		h.polls.mu.Lock()
		h.polls.upstream[name]--
		if park {
			h.polls.parked++
		}
		h.polls.mu.Unlock()
		h.changed()
	}
}

// unpark counts a parked wake answer gone, passed on or given up.
func (h *Rig) unpark() {
	h.polls.mu.Lock()
	h.polls.parked--
	h.polls.mu.Unlock()
	h.changed()
}

// answered marks the long-poll called name of every Factory running as
// answered, before its answer goes on to it.
func (h *Rig) answered(name string) {
	h.runMu.Lock()
	runs := slices.Clone(h.runs)
	h.runMu.Unlock()
	for _, r := range runs {
		r.track.answered(name)
	}
}

// describe lists the open pieces of work, for a failure.
func (k *tracker) describe() string {
	k.mu.Lock()
	defer k.mu.Unlock()
	var names []string
	for _, p := range k.open {
		names = append(names, p.name)
	}
	sort.Strings(names)
	return fmt.Sprintf("%q", names)
}

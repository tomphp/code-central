package factorytest

import (
	"net/http"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/tomoram/code-central/store"
)

// requestHold has the proxy hold the requests whose path ends in a
// suffix the test names, so a test can stop the Factory or a turn's
// claude mid-way.
type requestHold struct {
	suffix   atomic.Value // string; "" holds none
	released chan struct{}
	once     sync.Once
}

// newRequestHold sets h's proxy up to hold requests; call it before
// Start.
func newRequestHold(h *Harness) *requestHold {
	rh := &requestHold{released: make(chan struct{})}
	rh.suffix.Store("")
	h.stall = func(r *http.Request) {
		if s := rh.suffix.Load().(string); s != "" && strings.HasSuffix(r.URL.Path, s) {
			select {
			case <-rh.released:
			case <-r.Context().Done():
			}
		}
	}
	h.t.Cleanup(rh.release)
	return rh
}

// hold holds every request whose path ends in suffix from now until
// release.
func (rh *requestHold) hold(suffix string) { rh.suffix.Store(suffix) }

// release lets the held requests go on, and holds no more.
func (rh *requestHold) release() {
	rh.once.Do(func() {
		rh.suffix.Store("")
		close(rh.released)
	})
}

// waitUnsettledBy waits until what the Factory is yet to settle has an
// entry starting with want, and returns all it names.
func waitUnsettledBy(t *testing.T, h *Harness, r *Running, want string) []string {
	t.Helper()
	var unmet []string
	h.WaitForDescribed(func() string { return want + " in flight (have " + strings.Join(unmet, "; ") + ")" }, func() bool {
		unmet, _ = r.track.unsettled()
		return slices.ContainsFunc(unmet, func(s string) bool { return strings.HasPrefix(s, want) })
	})
	return unmet
}

// shortly is how long a test watches Settle not return: no time at all
// while what it waits for is held.
const shortly = 50 * time.Millisecond

func TestSettleReturnsWhileATurnIsParkedInAWaitStep(t *testing.T) {
	t.Parallel()
	h := New(t)
	id := h.ReadyTicket("Long job")
	h.Script(id, Wait())
	r := h.Start()

	r.Settle()
	if n := len(h.Calls()); n != 1 {
		t.Fatalf("claude ran %d times, want once", n)
	}
	if _, parked := h.parked(id); !parked {
		t.Error("Settle returned with the turn not in its Wait step")
	}
	if s := h.Ticket(id).State; s != store.InProgress {
		t.Errorf("%s is %s, want In Progress while its turn runs", id, s)
	}
}

func TestSettleDoesNotReturnWhileATurnIsBetweenScriptedSteps(t *testing.T) {
	t.Parallel()
	h := New(t)
	holds := newRequestHold(h)
	id := h.ReadyTicket("Two waits")
	h.Script(id, Wait(), CLI("comment", "{{ticket}}", "--body", "Between waits."), Wait())
	r := h.Start()

	holds.hold("/comment")
	h.Continue(id)
	waitUnsettledBy(t, h, r, "turn "+id+" not in a Wait step")
	if unmet := r.settle(shortly); !slices.Contains(unmet, "turn "+id+" not in a Wait step") {
		t.Fatalf("Settle returned (%q) while the turn's claude was between its steps", unmet)
	}

	holds.release()
	r.Settle()
	if _, parked := h.parked(id); !parked {
		t.Error("Settle returned before the turn reached its second Wait step")
	}
	if n := len(h.Ticket(id).Comments); n != 1 {
		t.Errorf("%s has %d comments, want the step's one", id, n)
	}
}

func TestSettleDoesNotReturnWhileATurnsExitIsHandled(t *testing.T) {
	t.Parallel()
	h := New(t)
	holds := newRequestHold(h)
	id := h.ReadyTicket("Finish")
	h.Script(id, Wait(), Finish("Done."), Result("success"))
	r := h.Start()

	// Handling the exit starts by reading the Ticket.
	holds.hold("/tickets/" + id)
	h.Continue(id)
	waitUnsettledBy(t, h, r, "handling the exit of turn "+id)
	if unmet := r.settle(shortly); !slices.Contains(unmet, "handling the exit of turn "+id) {
		t.Fatalf("Settle returned (%q) while the turn's exit was being handled", unmet)
	}

	holds.release()
	r.Settle()
	if !strings.Contains(r.Stdout(), id+" turn ended") {
		t.Errorf("Settle returned before the turn's exit was handled; stdout:\n%s", r.Stdout())
	}
}

func TestSettleTimesOutNamingTheWorkInFlight(t *testing.T) {
	t.Parallel()
	h := New(t)
	holds := newRequestHold(h)
	r := h.Start()

	// The poll Tick starts holds on its find_tickets.
	holds.hold("/tickets")
	r.Tick()
	waitUnsettledBy(t, h, r, "requests through the proxy: GET /api/p/acme/tickets")
	unmet := r.settle(shortly)
	if !slices.Contains(unmet, "poll 2") {
		t.Errorf("Settle timed out naming %q, want poll 2 named", unmet)
	}
	if !slices.ContainsFunc(unmet, func(s string) bool {
		return strings.HasPrefix(s, "requests through the proxy: GET /api/p/acme/tickets")
	}) {
		t.Errorf("Settle timed out naming %q, want the held find_tickets named", unmet)
	}
	holds.release()
	r.Settle()
}

// A claim whose answer was lost leaves its poll resting on the claim
// retry pause; the next Tick passes the pause before it makes the poll.
func TestTickFiresADueClaimRetryBeforeThePoll(t *testing.T) {
	t.Parallel()
	h := New(t)
	r := h.Start()
	id := h.ReadyTicket("Lost answer")
	h.LoseAnswer("/claim", 1)
	r.Tick()

	before := h.Clock.Now()
	r.Tick()
	if got, want := h.Clock.Now().Sub(before), time.Second+30*time.Second; got != want {
		t.Errorf("the clock moved %v, want the claim retry pause and the poll, %v", got, want)
	}
	r.Settle()
	if tk := h.Ticket(id); len(tk.Claims) != 1 || tk.State != store.InReview {
		t.Errorf("%s is %s with %d Claims, want In Review with the one Claim the retry got back", id, tk.State, len(tk.Claims))
	}
	if n := len(h.Calls()); n != 1 {
		t.Errorf("claude ran %d times, want once", n)
	}
}

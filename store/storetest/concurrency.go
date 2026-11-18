// The shared-Store cases (Spec code-central-1032): two services' Updates
// on one Store at the same moment, each in a transaction of its own, as
// two machines on one Postgres Store run them. The Store orders them: one
// Claim wins, two creates take different numbers, and an Update that
// keeps colliding gives up with ErrConflict. A Kind one service holds
// alone leaves Factory.Second nil and skips them: its process-wide lock
// would hold the second closure at the barrier for ever.

package storetest

import (
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/tomoram/code-central/domain"
	"github.com/tomoram/code-central/store"
)

// barrier lets n closures meet once: each goes on when all have arrived.
// One left waiting too long fails rather than hangs, saying so.
type barrier struct{ wg sync.WaitGroup }

func newBarrier(n int) *barrier {
	b := &barrier{}
	b.wg.Add(n)
	return b
}

func (b *barrier) wait() error {
	b.wg.Done()
	met := make(chan struct{})
	go func() {
		b.wg.Wait()
		close(met)
	}()
	select {
	case <-met:
		return nil
	case <-time.After(10 * time.Second):
		return errors.New("the other service never reached the barrier")
	}
}

func runConcurrency(t *testing.T, f Factory) {
	if f.Second == nil {
		return
	}
	// twoServices is a fresh Store with the Project, as two services hold it.
	twoServices := func(t *testing.T) (a, b store.Store) {
		a = f.New(t)
		seedProject(t, a)
		return a, f.Second(t, a)
	}
	// race runs update on both services at once, telling it which service
	// (0 or 1) it is, with meet to wait for the other at the barrier: on
	// the service's first attempt only, the other having gone on by any
	// retry. It returns each service's Update error.
	race := func(a, b store.Store, update func(w store.Writer, i int, meet func() error) error) (errs [2]error) {
		barrier := newBarrier(2)
		var wg sync.WaitGroup
		for i, s := range []store.Store{a, b} {
			wg.Add(1)
			go func() {
				defer wg.Done()
				first := true
				meet := func() error {
					if !first {
						return nil
					}
					first = false
					return barrier.wait()
				}
				errs[i] = s.Update(func(w store.Writer) error { return update(w, i, meet) })
			}()
		}
		wg.Wait()
		return errs
	}

	t.Run("two services claiming one ticket at the same moment give one winner", func(t *testing.T) {
		a, b := twoServices(t)
		put(t, a, func(w store.Writer) error {
			return w.PutTicket("acme", domain.Ticket{Number: 1, Title: "T", State: domain.Ready, Spec: 1, Created: t0, Updated: t0})
		})
		errClaimed := errors.New("already claimed")
		errs := race(a, b, func(w store.Writer, i int, meet func() error) error {
			tk, ok := w.Ticket("acme", 1, store.Claims)
			if !ok {
				return errors.New("the ticket is missing")
			}
			// Both have read the unclaimed Ticket before either writes.
			if err := meet(); err != nil {
				return err
			}
			if len(tk.Claims) > 0 {
				return errClaimed
			}
			tk.State = domain.InProgress
			tk.Claims = []domain.Claim{{Number: 1, ID: fmt.Sprintf("CLAIM%d", i), SessionID: fmt.Sprintf("sess-%d", i), Started: t0}}
			return w.PutTicket("acme", tk, store.Claims)
		})
		winners := 0
		for i, err := range errs {
			switch {
			case err == nil:
				winners++
			case !errors.Is(err, errClaimed):
				t.Fatalf("service %d: %v", i, err)
			}
		}
		if winners != 1 {
			t.Fatalf("%d winners, want 1 (errors %v)", winners, errs)
		}
		read(t, a, func(r store.Reader) {
			if tk, _ := r.Ticket("acme", 1, store.Claims); len(tk.Claims) != 1 || tk.State != domain.InProgress {
				t.Fatalf("after both: state %s, claims %+v", tk.State, tk.Claims)
			}
		})
	})

	t.Run("two services creating a ticket at the same moment take different numbers", func(t *testing.T) {
		a, b := twoServices(t)
		var numbers [2]int
		errs := race(a, b, func(w store.Writer, i int, meet func() error) error {
			numbers[i] = w.NextNumber("acme")
			// Both have picked their number before either writes.
			if err := meet(); err != nil {
				return err
			}
			return w.PutTicket("acme", domain.Ticket{Number: numbers[i], Title: fmt.Sprintf("by %d", i), State: domain.Draft, Spec: 1, Created: t0, Updated: t0})
		})
		for i, err := range errs {
			if err != nil {
				t.Fatalf("service %d: %v", i, err)
			}
		}
		if numbers[0] == numbers[1] {
			t.Fatalf("both took number %d", numbers[0])
		}
		read(t, a, func(r store.Reader) {
			if got := r.FindTickets("acme", store.TicketFilter{}); len(got) != 2 {
				t.Fatalf("%d tickets, want 2", len(got))
			}
			if n := r.NextNumber("acme"); n != 3 {
				t.Fatalf("NextNumber = %d, want 3", n)
			}
		})
	})

	t.Run("an update that keeps conflicting fails with ErrConflict after five attempts", func(t *testing.T) {
		a, b := twoServices(t)
		put(t, a, func(w store.Writer) error {
			return w.PutTicket("acme", domain.Ticket{Number: 1, Title: "T", State: domain.Draft, Spec: 1, Created: t0, Updated: t0})
		})
		attempts := 0
		err := a.Update(func(w store.Writer) error {
			attempts++
			tk, ok := w.Ticket("acme", 1)
			if !ok {
				return errors.New("the ticket is missing")
			}
			// The other service changes the Ticket between this read and
			// the write, every time.
			put(t, b, func(w store.Writer) error {
				other, _ := w.Ticket("acme", 1)
				other.Title = fmt.Sprintf("by b, %d", attempts)
				return w.PutTicket("acme", other)
			})
			tk.Title = "by a"
			return w.PutTicket("acme", tk)
		})
		if !errors.Is(err, store.ErrConflict) {
			t.Fatalf("error %v, want store.ErrConflict", err)
		}
		if attempts != 5 {
			t.Fatalf("%d attempts, want 5", attempts)
		}
		read(t, a, func(r store.Reader) {
			if tk, _ := r.Ticket("acme", 1); tk.Title != "by b, 5" {
				t.Fatalf("title %q: a's write landed, or b's last didn't", tk.Title)
			}
		})
	})
}

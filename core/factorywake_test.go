package core_test

import (
	"context"
	"testing"
	"time"

	"github.com/tomoram/code-central/core"
	"github.com/tomoram/code-central/store"
)

// wakeAsync opens Factory factory's next_factory_wake call on Project key
// from host box, and waits until it holds.
func (f fixture) wakeAsync(key, factory string) chan core.FactoryWake {
	out := make(chan core.FactoryWake, 1)
	n := f.clock.Waiters()
	go func() {
		w, err := f.core.NextFactoryWake(ctx, key, core.NextFactoryWakeInput{Factory: factory, Host: "box"})
		if err != nil {
			panic(err)
		}
		out <- w
	}()
	f.clock.BlockUntil(n + 1)
	return out
}

// ended is the call's result once WaitTimeout has passed, so a call no
// write woke ends empty instead of holding the test.
func (f fixture) ended(call chan core.FactoryWake) core.FactoryWake {
	f.clock.Advance(core.WaitTimeout)
	return <-call
}

func (f fixture) approve(t *testing.T, ref core.Ref) {
	t.Helper()
	if _, err := f.core.Approve(human, "acme", core.TicketInput{Ticket: ref}); err != nil {
		t.Fatalf("approve %s: %v", ref, err)
	}
}

// Every move into Ready, Answered, Accepted, Done or Cancelled wakes the
// Project's Factories: the Ticket may be claimable, to resume, to land,
// or the last Blocker of another.
func TestEveryMoveTheFactorysPollActsOnWakesIt(t *testing.T) {
	for _, tc := range []struct {
		name  string
		setup func(t *testing.T, f fixture)
		move  func(f fixture) error
	}{
		{"create a Ready Ticket", func(t *testing.T, f fixture) { f.work(t) }, func(f fixture) error {
			_, err := f.core.CreateTicket(ctx, "acme", core.CreateTicketInput{Spec: "1", Title: "Ta", State: store.Ready})
			return err
		}},
		{"approve", func(t *testing.T, f fixture) { f.work(t, store.Draft) }, func(f fixture) error {
			_, err := f.core.Approve(human, "acme", core.TicketInput{Ticket: "2"})
			return err
		}},
		{"send back", func(t *testing.T, f fixture) { f.failed(t) }, func(f fixture) error {
			_, err := f.core.SendBack(human, "acme", core.SendBackInput{Ticket: "2", Note: "Run migration 12 first."})
			return err
		}},
		{"reject", func(t *testing.T, f fixture) { f.inReview(t) }, func(f fixture) error {
			_, err := f.core.Reject(human, "acme", core.RejectInput{Ticket: "2", Comment: "Name the button."})
			return err
		}},
		{"release", func(t *testing.T, f fixture) {
			f.work(t, store.Ready)
			f.claim(t, core.ClaimInput{})
		}, func(f fixture) error {
			_, err := f.core.Release(human, "acme", core.ReleaseInput{Ticket: "2"})
			return err
		}},
		{"give up", func(t *testing.T, f fixture) {
			f.work(t, store.Ready)
			f.claim(t, core.ClaimInput{})
		}, func(f fixture) error {
			_, err := f.core.GiveUp(ctx, "acme", core.GiveUpInput{Ticket: "2", ClaimID: "claim-1", Note: "Out of ideas."})
			return err
		}},
		{"a Round's last answer", func(t *testing.T, f fixture) { f.asking(t, qDB) }, func(f fixture) error {
			_, err := f.core.Answer(human, "acme", core.AnswerInput{Ticket: "2", Question: 1, Text: "SQLite"})
			return err
		}},
		{"accept", func(t *testing.T, f fixture) { f.inReviewOnBranch(t) }, func(f fixture) error {
			_, err := f.core.Accept(human, "acme", core.TicketInput{Ticket: "2"})
			return err
		}},
		{"land", func(t *testing.T, f fixture) { f.accepted(t) }, func(f fixture) error {
			_, err := f.core.Land(ctx, "acme", core.LandInput{Ticket: "2", Commit: "def456"})
			return err
		}},
		{"implement", func(t *testing.T, f fixture) { f.specInReview(t) }, func(f fixture) error {
			_, err := f.core.Implement(human, "acme", core.TicketInput{Ticket: "4"})
			return err
		}},
		{"cancel", func(t *testing.T, f fixture) { f.work(t, store.Draft) }, func(f fixture) error {
			_, err := f.core.Cancel(human, "acme", core.CancelInput{Ticket: "2"})
			return err
		}},
		{"a move by hand to Done", func(t *testing.T, f fixture) { f.inReview(t) }, func(f fixture) error {
			_, err := f.core.Move(human, "acme", core.MoveInput{Ticket: "2", ToState: store.Done})
			return err
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newFixture(t)
			tc.setup(t, f)
			woken := f.wakeAsync("acme", "cc")
			if err := tc.move(f); err != nil {
				t.Fatal(err)
			}
			if w := f.ended(woken); !w.Woken {
				t.Fatalf("next_factory_wake = %+v, want woken", w)
			}
		})
	}
}

// Running agents write all the time; none of these is a change the
// Factory's poll acts on, so the call holds until WaitTimeout and ends
// empty.
func TestWritesTheFactorysPollDoesNotActOnLeaveItAsleepUntilTheTimeout(t *testing.T) {
	f := newFixture(t)
	f.work(t, store.Ready, store.Ready)
	woken := f.wakeAsync("acme", "cc")
	writes := []func() error{
		func() error {
			_, err := f.core.Claim(ctx, "acme", core.ClaimInput{SessionID: "s1"})
			return err
		},
		func() error {
			_, err := f.core.SetSession(ctx, "acme", core.SetSessionInput{Ticket: "2", ClaimID: "claim-1", SessionID: "s1"})
			return err
		},
		func() error {
			f.core.PostSessionEvents(ctx, "s1", []core.SessionEvent{{Time: f.clock.Now(), Kind: "tool_call", Text: "Bash: go test ./..."}})
			return nil
		},
		func() error {
			_, err := f.core.Comment(ctx, "acme", core.CommentInput{Ticket: "2", Body: "Halfway."})
			return err
		},
		func() error {
			_, err := f.core.Finish(ctx, "acme", core.FinishInput{Ticket: "2", ClaimID: "claim-1", Summary: "Done on acme-2."})
			return err
		},
		func() error {
			_, err := f.core.Claim(ctx, "acme", core.ClaimInput{SessionID: "s2"})
			return err
		},
		func() error {
			_, err := f.core.PostRound(ctx, "acme", core.PostRoundInput{Ticket: "3", ClaimID: "claim-2", Questions: []core.QuestionInput{qDB}})
			return err
		},
	}
	for i, write := range writes {
		if err := write(); err != nil {
			t.Fatalf("write %d: %v", i, err)
		}
	}
	f.clock.Advance(core.WaitTimeout - time.Second)
	select {
	case w := <-woken:
		t.Fatalf("returned before the timeout: %+v", w)
	default:
	}
	f.clock.Advance(time.Second)
	if w := <-woken; w.Woken {
		t.Fatalf("next_factory_wake = %+v, want empty", w)
	}
}

func TestAChangeInAnotherProjectDoesNotWakeTheFactory(t *testing.T) {
	f := newFixture(t)
	f.work(t, store.Draft)
	f.project(t, "other")
	woken := f.wakeAsync("other", "cc")
	f.approve(t, "2")
	if w := f.ended(woken); w.Woken {
		t.Fatalf("other's next_factory_wake = %+v, want empty", w)
	}
}

func TestEveryFactoryOnTheProjectIsWoken(t *testing.T) {
	f := newFixture(t)
	f.work(t, store.Draft)
	first, second := f.wakeAsync("acme", "cc"), f.wakeAsync("acme", "cc-2")
	f.approve(t, "2")
	f.clock.Advance(core.WaitTimeout)
	for _, woken := range []chan core.FactoryWake{first, second} {
		if w := <-woken; !w.Woken {
			t.Fatalf("next_factory_wake = %+v, want woken", w)
		}
	}
}

func TestANewerCallFromTheSameFactoryEndsTheOlderOneEmpty(t *testing.T) {
	f := newFixture(t)
	f.work(t, store.Draft)
	older := f.wakeAsync("acme", "cc")
	newer := make(chan core.FactoryWake, 1)
	go func() {
		w, _ := f.core.NextFactoryWake(ctx, "acme", core.NextFactoryWakeInput{Factory: "cc", Host: "box"})
		newer <- w
	}()
	if w := <-older; w.Woken {
		t.Fatalf("older next_factory_wake = %+v, want empty", w)
	}
	f.clock.BlockUntil(2) // the newer call's timeout, beside the older's
	f.approve(t, "2")
	if w := f.ended(newer); !w.Woken {
		t.Fatalf("newer next_factory_wake = %+v, want woken", w)
	}
}

func TestNextFactoryWakeEndsEmptyWhenItsRequestIsCancelled(t *testing.T) {
	f := newFixture(t)
	f.project(t, "acme")
	cctx, cancel := context.WithCancel(ctx)
	done := make(chan core.FactoryWake, 1)
	go func() {
		w, _ := f.core.NextFactoryWake(cctx, "acme", core.NextFactoryWakeInput{Factory: "cc", Host: "box"})
		done <- w
	}()
	f.clock.BlockUntil(1)
	cancel()
	if w := <-done; w.Woken {
		t.Fatalf("next_factory_wake = %+v, want empty", w)
	}
}

// Like next_chat_work, it is a read: an archived Project still answers.
func TestNextFactoryWakeHoldsOnAnArchivedProject(t *testing.T) {
	f := newFixture(t)
	f.project(t, "acme")
	f.archive(t, "acme")
	if w := f.ended(f.wakeAsync("acme", "cc")); w.Woken {
		t.Fatalf("next_factory_wake = %+v, want empty", w)
	}
}

func TestNextFactoryWakeRejections(t *testing.T) {
	f := newFixture(t)
	f.project(t, "acme")
	_, err := f.core.NextFactoryWake(ctx, "acme", core.NextFactoryWakeInput{Host: "box"})
	wantCode(t, err, core.Invalid, "factory")
	_, err = f.core.NextFactoryWake(ctx, "acme", core.NextFactoryWakeInput{Factory: "cc", Host: " "})
	wantCode(t, err, core.Invalid, "host")
	_, err = f.core.NextFactoryWake(ctx, "nope", core.NextFactoryWakeInput{Factory: "cc", Host: "box"})
	wantCode(t, err, core.UnknownProject, "")
}

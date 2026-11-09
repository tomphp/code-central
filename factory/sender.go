package factory

import (
	"context"
	"sync"
	"time"
)

// The Factory's posts to Code Central of what its turns do, Session
// Events and token usage, never block a turn and never fail one: items
// wait in a bounded queue, and a batch that can't be posted is dropped.

const (
	// postQueueMax is the most items waiting to be posted; when it is
	// full the oldest are dropped.
	postQueueMax = 1000
	// postEvery is how often waiting items are posted.
	postEvery = time.Second
	// postBatchMax is how many waiting items make a post at once.
	postBatchMax = 100
	// postTimeout bounds each post, the last flush's too.
	postTimeout = 2 * time.Second
)

// queued is an item waiting to be posted for a session.
type queued[T any] struct {
	session string
	item    T
}

// sender is one of the Factory's posters: of Session Events, or of
// model responses' token usage. It posts what is waiting every postEvery, or sooner once
// postBatchMax are waiting, one post per session in the order they
// came, so each session's order holds. every is postEvery unless a
// test shortens it.
type sender[T any] struct {
	post  func(ctx context.Context, session string, items []T) error
	every time.Duration

	mu      sync.Mutex
	waiting []queued[T]
	full    chan struct{} // signalled when postBatchMax are waiting
	stop    chan struct{}
	done    chan struct{}
}

func newSender[T any](post func(context.Context, string, []T) error, every time.Duration) *sender[T] {
	return &sender[T]{post: post, every: every, full: make(chan struct{}, 1), stop: make(chan struct{}), done: make(chan struct{})}
}

// add queues item for session, dropping the oldest waiting item when the
// queue is full. It never blocks on a post.
func (s *sender[T]) add(session string, item T) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.waiting) >= postQueueMax {
		s.waiting = s.waiting[1:]
	}
	s.waiting = append(s.waiting, queued[T]{session, item})
	if len(s.waiting) >= postBatchMax {
		select {
		case s.full <- struct{}{}:
		default:
		}
	}
}

// run posts batches until close.
func (s *sender[T]) run() {
	defer close(s.done)
	tick := time.NewTicker(s.every)
	defer tick.Stop()
	for {
		select {
		case <-s.stop:
			// The last flush has one postTimeout in all, so a hung
			// Code Central holds up the stop no longer than that.
			ctx, cancel := context.WithTimeout(context.Background(), postTimeout)
			s.flush(ctx)
			cancel()
			return
		case <-tick.C:
		case <-s.full:
		}
		s.flush(context.Background())
	}
}

// close makes one last flush and stops the sender.
func (s *sender[T]) close() {
	close(s.stop)
	<-s.done
}

// flush posts every waiting item, one post per session, each with
// postTimeout within ctx. A post that fails, an older Code
// Central's 404 included, is dropped, never retried.
func (s *sender[T]) flush(ctx context.Context) {
	s.mu.Lock()
	waiting := s.waiting
	s.waiting = nil
	s.mu.Unlock()
	var sessions []string
	bySession := map[string][]T{}
	for _, w := range waiting {
		if _, ok := bySession[w.session]; !ok {
			sessions = append(sessions, w.session)
		}
		bySession[w.session] = append(bySession[w.session], w.item)
	}
	for _, session := range sessions {
		ctx, cancel := context.WithTimeout(ctx, postTimeout)
		_ = s.post(ctx, session, bySession[session])
		cancel()
	}
}

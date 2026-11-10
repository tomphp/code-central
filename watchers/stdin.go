// The Watcher's side of its lifetime: it runs until the pipe that is its
// stdin closes, which the service closes to stop it, and which closes by
// itself when the service dies, as a stopped Windows scheduled task
// doesn't kill its children.

package watchers

import (
	"context"
	"io"
)

// UntilStdinCloses is ctx, also ended once stdin, a pipe from the
// service, closes. A nil stdin ends it only with ctx.
func UntilStdinCloses(ctx context.Context, stdin io.Reader) (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancel(ctx)
	if stdin != nil {
		go func() {
			io.Copy(io.Discard, stdin)
			cancel()
		}()
	}
	return ctx, cancel
}

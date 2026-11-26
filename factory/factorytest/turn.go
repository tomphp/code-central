// A Turn run straight through the Harness or the Turn runner, without the
// Factory's loop: the rig builds it for a Script's id and records the
// lines it gets.

package factorytest

import (
	"fmt"
	"slices"
	"sync"

	"github.com/tomoram/code-central/factory/harness"
)

// Turn is a Turn of id, whose Script the fake claude runs, in the rig's
// repo, its lines recorded in lines.
func (h *Rig) Turn(id string, lines *Lines) harness.Turn {
	return harness.Turn{
		Dir:     h.Repo,
		Session: "11111111-2222-4333-8444-555555555555",
		Prompt:  "Do the work.\n\nTicket: " + id + "\n",
		Linef:   lines.Linef,
	}
}

// Lines records the lines a Turn gives its line function, safe for the
// Turn's goroutines.
type Lines struct {
	mu    sync.Mutex
	lines []string
}

// Linef records one line.
func (l *Lines) Linef(format string, args ...any) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.lines = append(l.lines, fmt.Sprintf(format, args...))
}

// Got is the lines recorded so far.
func (l *Lines) Got() []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return slices.Clone(l.lines)
}

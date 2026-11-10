package factory

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
)

// claim is one Claim the Factory holds, as the state file keeps it. The
// worktree path is derived from the Ticket ID, so it isn't stored.
type claim struct {
	Ticket       string   `json:"ticket"`
	ClaimID      string   `json:"claim_id"`
	SessionID    string   `json:"session_id"`
	AllowedTools []string `json:"allowed_tools"`
	// SystemPrompt is what every turn adds to claude's system prompt
	// (systemPrompt); empty for a Claim started before the Factory added
	// one, whose first prompt holds it instead.
	SystemPrompt string `json:"system_prompt,omitempty"`
	// Denials is the Round the Factory posted for the calls its last turn
	// had denied, while it waits for answers; nil otherwise.
	Denials *denialRound `json:"denials,omitempty"`
}

// denialRound is a Round of the Factory's own: one Question per distinct
// denied call, in order, each with the pattern a bare "allow" allows.
type denialRound struct {
	Round    int      `json:"round"`
	Patterns []string `json:"patterns"`
}

// updateClaim applies change to the held Claim claimID and saves.
func (f *factory) updateClaim(claimID string, change func(*claim)) error {
	s := f.state
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.claims {
		if s.claims[i].ClaimID == claimID {
			change(&s.claims[i])
		}
	}
	return s.write()
}

// stateFile is ~/.local/state/code-central-factory/<project-key>.json,
// rewritten atomically after every change.
type stateFile struct {
	path   string
	mu     sync.Mutex
	claims []claim
}

// errLockHeld means another process holds the lock lockFile tried.
var errLockHeld = errors.New("the lock is held")

type stateDoc struct {
	Claims []claim `json:"claims"`
}

// lock takes the exclusive lock (lockFile) that makes this the Project's
// only local Factory, for as long as the returned file stays open. The lock is on
// <state file>.lock rather than the state file itself: each atomic write
// renames a new file into place, which would drop a lock held on the old
// one. The lock file holds the holder's pid.
func (s *stateFile) lock(project string) (*os.File, error) {
	lf, err := os.OpenFile(s.path+".lock", os.O_RDWR|os.O_CREATE, 0o644)
	if err != nil {
		return nil, err
	}
	if err := lockFile(lf); err != nil {
		b, _ := os.ReadFile(lf.Name())
		lf.Close()
		if errors.Is(err, errLockHeld) {
			return nil, fmt.Errorf("a Factory is already running for %s (pid %s)", project, strings.TrimSpace(string(b)))
		}
		return nil, err
	}
	if err := lf.Truncate(0); err == nil {
		_, err = lf.WriteAt([]byte(strconv.Itoa(os.Getpid())+"\n"), 0)
	}
	return lf, nil
}

// load reads the Claims a past Factory saved. A missing file holds none:
// its Claims are lost, and never adopted.
func (s *stateFile) load() error {
	b, err := os.ReadFile(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	var doc stateDoc
	if err := json.Unmarshal(b, &doc); err != nil {
		return fmt.Errorf("%s: %v", s.path, err)
	}
	s.claims = doc.Claims
	return nil
}

func (f *factory) addClaim(c claim) error {
	s := f.state
	s.mu.Lock()
	defer s.mu.Unlock()
	s.claims = append(s.claims, c)
	return s.write()
}

// heldClaims is the Claims the Factory holds, oldest first.
func (f *factory) heldClaims() []claim {
	s := f.state
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]claim{}, s.claims...)
}

// dropClaim takes the Claim off the state file.
func (f *factory) dropClaim(claimID string) error {
	s := f.state
	s.mu.Lock()
	defer s.mu.Unlock()
	s.claims = slices.DeleteFunc(s.claims, func(c claim) bool { return c.ClaimID == claimID })
	return s.write()
}

// write replaces the file through a rename, so a reader never sees half.
func (s *stateFile) write() error {
	b, err := json.MarshalIndent(stateDoc{Claims: append([]claim{}, s.claims...)}, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(s.path), filepath.Base(s.path)+".tmp-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(append(b, '\n')); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), s.path)
}

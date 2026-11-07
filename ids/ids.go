// Package ids is the source of random identifiers (Claim IDs) injected
// into core, so tests can make them predictable.
package ids

import (
	"crypto/rand"
	"encoding/base32"
	"fmt"
	"sync"
)

// Source issues new Claim IDs.
type Source interface {
	ClaimID() string
}

// Random issues 128 random bits, base32-encoded without padding.
type Random struct{}

// ClaimID returns a new random Claim ID.
func (Random) ClaimID() string {
	var b [16]byte
	rand.Read(b[:])
	return base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(b[:])
}

// Sequence is a fake Source issuing claim-1, claim-2, ...
type Sequence struct {
	mu sync.Mutex
	n  int
}

// ClaimID returns the next ID in the sequence.
func (s *Sequence) ClaimID() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.n++
	return fmt.Sprintf("claim-%d", s.n)
}

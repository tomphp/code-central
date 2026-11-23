// Package factories holds what the Factories and their sessions report
// or wait on: Heartbeats and the Factory Status, Plan Limits, the base
// and clone reports, Transcripts and latest events, Chat, the Factory's
// wake and the sessions' pings. It imports neither core nor another area.
package factories

import (
	"time"

	"github.com/tomoram/code-central/api"
	"github.com/tomoram/code-central/core/foundation"
)

// Factories is the Factories area. Build one with New.
type Factories struct {
	f *foundation.Foundation
	// started is when the service started, a floor on every session's
	// last activity.
	started time.Time
	pings   *pings
	// transcripts holds each Claim's Session Events, in memory only.
	transcripts *transcripts
	// latestNotices throttles the ticket.changed a new latest event sends.
	latestNotices *latestNotices
	// heartbeats holds each Factory's latest Heartbeat, in memory, read
	// from and written through to the Store.
	heartbeats *heartbeats
	// chats holds each Project's current Conversation, in memory only.
	chats *chats
	// factoryWakes holds the Factories' open next_factory_wake calls, in
	// memory only.
	factoryWakes *factoryWakes
}

// New returns the Factories area over f, whose Store it reads when it
// first needs it.
func New(f *foundation.Foundation) *Factories {
	return &Factories{
		f:             f,
		started:       f.Now(),
		pings:         &pings{mine: map[string]time.Time{}, shared: map[string]time.Time{}},
		transcripts:   &transcripts{all: map[markKey]*transcript{}, feeds: map[markKey]map[*TranscriptFeed]struct{}{}},
		latestNotices: &latestNotices{sent: map[markKey]time.Time{}, pending: map[markKey]bool{}},
		heartbeats: &heartbeats{all: map[heartbeatKey]heartbeat{}, seen: map[string]string{}, base: map[string]map[string]api.BaseView{},
			clones: map[string]map[string]api.CloneView{}, reportsAt: map[string]time.Time{}, live: map[string]string{}},
		chats:        newChats(),
		factoryWakes: newFactoryWakes(),
	}
}

// markKey names one Claim, or, with claim 0, a whole Ticket, across
// Projects.
type markKey struct {
	project string
	ticket  int
	claim   int
}

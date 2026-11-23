package factories

import (
	"time"

	"github.com/tomoram/code-central/domain"
	"github.com/tomoram/code-central/store"
)

// Transcripts on a Shared Store (Spec code-central-1107): a Claim's
// Transcript stays in memory on the service its Session Events reach, its
// Reporting Service. That service writes a Transcript holder to the
// Store's Services side when it starts holding the Transcript, so another
// service sharing the Store can name its host instead of showing an empty
// Transcript. A Store that can't be reached is logged and never fails the
// post or the read: the read is then this service's own.

// startHolding records this service as the holder of Claim k's
// Transcript, as it starts holding it, and says whether another service
// held it before: the session has moved here, so what this service holds
// is a tail.
func (c *Factories) startHolding(k markKey) (moved bool) {
	prev, ok := c.transcriptHolder(k)
	err := c.f.Store.Services().PutTranscriptHolder(store.TranscriptHolder{Project: k.project, Ticket: k.ticket, Claim: k.claim,
		Service: c.f.Service().ID, ServiceHost: c.f.Service().Host, At: c.f.Now()})
	if err != nil {
		c.f.Logf("cannot record that this service holds %s's Claim %d's Transcript: %v", domain.DisplayID(k.project, k.ticket), k.claim, err)
	}
	return ok && prev.Service != c.f.Service().ID
}

// heldElsewhere is the host of the service holding Claim k's Transcript,
// by its Transcript holder, when that is another service still running
// (registered): it holds the session's newest events, even when this
// service holds older ones. It is empty when this service holds it, when
// none is recorded, as for a hand-run session, which posts no Session
// Events, or when the holder has stopped, as nobody holds more.
func (c *Factories) heldElsewhere(k markKey) string {
	h, ok := c.transcriptHolder(k)
	if !ok || h.Service == c.f.Service().ID {
		return ""
	}
	regs, err := c.f.Store.Services().Registrations(c.f.Now())
	if err != nil {
		c.f.Logf("cannot read the services' Registrations: %v", err)
		return ""
	}
	for _, r := range regs {
		if r.ID == h.Service {
			return h.ServiceHost
		}
	}
	return ""
}

// transcriptHolder is Claim k's Transcript holder, if one is recorded.
// What the Store can't read is logged, as none recorded.
func (c *Factories) transcriptHolder(k markKey) (store.TranscriptHolder, bool) {
	h, ok, err := c.f.Store.Services().TranscriptHolder(k.project, k.ticket, k.claim)
	if err != nil {
		c.f.Logf("cannot read who holds %s's Claim %d's Transcript: %v", domain.DisplayID(k.project, k.ticket), k.claim, err)
		return store.TranscriptHolder{}, false
	}
	return h, ok
}

// transcriptHolderLife is how long a Transcript holder stays in the Store
// after it was written, the pings' horizon: a Claim's session is long
// over by then.
const transcriptHolderLife = 7 * 24 * time.Hour

// pruneTranscriptHolders deletes the Transcript holders older than
// transcriptHolderLife. What it can't delete is logged, to try next time.
func (c *Factories) pruneTranscriptHolders() {
	if err := c.f.Store.Services().PruneTranscriptHolders(c.f.Now().Add(-transcriptHolderLife)); err != nil {
		c.f.Logf("cannot prune the Transcript holders: %v", err)
	}
}

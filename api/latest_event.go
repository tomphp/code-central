package api

import "time"

// LatestEvent is a Claim's latest Session Event, shown short: its text is
// one line, cut to LatestEventLimit. Computed on read, never stored.
type LatestEvent struct {
	Kind SessionEventKind `json:"kind"`
	Text string           `json:"text"`
	Time time.Time        `json:"time"`
}

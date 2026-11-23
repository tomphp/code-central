package coretest

import (
	"time"

	"github.com/tomoram/code-central/store"
)

// FailingServices is a Store whose Services fail with Err, once it is
// set.
type FailingServices struct {
	store.Store
	Err error
}

func (s *FailingServices) Services() store.Services {
	return failing{s.Store.Services(), s}
}

type failing struct {
	store.Services
	s *FailingServices
}

func (f failing) HoldLease(project, id string, now time.Time) (store.Lease, error) {
	if f.s.Err != nil {
		return store.Lease{}, f.s.Err
	}
	return f.Services.HoldLease(project, id, now)
}

func (f failing) PutHeartbeat(h store.Heartbeat) error {
	if f.s.Err != nil {
		return f.s.Err
	}
	return f.Services.PutHeartbeat(h)
}

func (f failing) Heartbeats() ([]store.Heartbeat, error) {
	if f.s.Err != nil {
		return nil, f.s.Err
	}
	return f.Services.Heartbeats()
}

func (f failing) PutFactoryReports(r store.FactoryReports) error {
	if f.s.Err != nil {
		return f.s.Err
	}
	return f.Services.PutFactoryReports(r)
}

func (f failing) FactoryReports() ([]store.FactoryReports, error) {
	if f.s.Err != nil {
		return nil, f.s.Err
	}
	return f.Services.FactoryReports()
}

func (f failing) PutTranscriptHolder(h store.TranscriptHolder) error {
	if f.s.Err != nil {
		return f.s.Err
	}
	return f.Services.PutTranscriptHolder(h)
}

func (f failing) TranscriptHolder(project string, ticket, claim int) (store.TranscriptHolder, bool, error) {
	if f.s.Err != nil {
		return store.TranscriptHolder{}, false, f.s.Err
	}
	return f.Services.TranscriptHolder(project, ticket, claim)
}

func (f failing) PutPing(p store.Ping) error {
	if f.s.Err != nil {
		return f.s.Err
	}
	return f.Services.PutPing(p)
}

func (f failing) Pings() ([]store.Ping, error) {
	if f.s.Err != nil {
		return nil, f.s.Err
	}
	return f.Services.Pings()
}

func (f failing) PrunePings(before time.Time) error {
	if f.s.Err != nil {
		return f.s.Err
	}
	return f.Services.PrunePings(before)
}

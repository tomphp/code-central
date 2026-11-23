package coretest

import (
	"context"
	"testing"
	"time"

	"github.com/tomoram/code-central/api"
)

// Beat reports a Heartbeat on acme from Factory factory on host.
func (f Fixture) Beat(t *testing.T, factory, host string, state api.HeartbeatState, interval int) {
	t.Helper()
	f.BeatNote(t, factory, host, state, "", interval)
}

// BeatNote is Beat with a note.
func (f Fixture) BeatNote(t *testing.T, factory, host string, state api.HeartbeatState, note string, interval int) {
	t.Helper()
	_, err := f.Core.ReportHeartbeat(Ctx, "acme", api.ReportHeartbeatInput{Factory: factory, Host: host, State: state, Note: note, IntervalSeconds: interval})
	if err != nil {
		t.Fatal(err)
	}
}

// ReportBase reports the Factory's base on acme.
func (f Fixture) ReportBase(t *testing.T, in api.ReportBaseInput) {
	t.Helper()
	if _, err := f.Core.ReportBase(Ctx, "acme", in); err != nil {
		t.Fatal(err)
	}
}

// ReportRepositories reports the Factory's clones on acme.
func (f Fixture) ReportRepositories(t *testing.T, rs ...api.RepositoryReport) {
	t.Helper()
	if _, err := f.Core.ReportRepositories(Ctx, "acme", api.ReportRepositoriesInput{Repositories: rs}); err != nil {
		t.Fatal(err)
	}
}

// Clone is the clone report acme's own Repository shows, or nil.
func (f Fixture) Clone(t *testing.T) *api.CloneView {
	t.Helper()
	p, err := f.Core.GetProject(Ctx, "acme")
	if err != nil {
		t.Fatal(err)
	}
	return p.Repositories[0].Clone
}

// FactoryStatus is acme's Factory Status.
func (f Fixture) FactoryStatus(t *testing.T) api.FactoryStatusView {
	t.Helper()
	p, err := f.Core.GetProject(Ctx, "acme")
	if err != nil {
		t.Fatal(err)
	}
	return p.FactoryStatus
}

// OnlyFactory is the one Factory f lists for acme.
func (f Fixture) OnlyFactory(t *testing.T) api.FactoryView {
	t.Helper()
	fs := f.FactoryStatus(t).Factories
	if len(fs) != 1 {
		t.Fatalf("Factories %+v, want one", fs)
	}
	return fs[0]
}

// WatchFactories runs the Factory Status watch until the test ends, and
// returns tick, which ticks it twice and gives the Projects it reported
// changed.
func (f Fixture) WatchFactories(t *testing.T) (tick func() []string) {
	t.Helper()
	ticks := make(chan time.Time)
	changes := make(chan string, 10)
	watchCtx, stop := context.WithCancel(Ctx)
	done := make(chan struct{})
	go func() {
		f.Core.Factories().WatchFactoryStatus(watchCtx, ticks, func(project string) { changes <- project })
		close(done)
	}()
	t.Cleanup(func() { stop(); <-done })
	return func() []string {
		ticks <- f.Clock.Now()
		ticks <- f.Clock.Now() // the second send returns once the first tick is handled
		var got []string
		for {
			select {
			case p := <-changes:
				got = append(got, p)
			default:
				return got
			}
		}
	}
}

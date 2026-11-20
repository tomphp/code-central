//go:build !js

package core_test

import (
	"testing"

	"github.com/tomoram/code-central/api"
	"github.com/tomoram/code-central/core"
)

// A Factory reporting to one service on a Postgres Store shows on another
// service with its own handle on the Store (Spec code-central-1107).
func TestAFactoryReportingToOneServiceShowsOnAnotherOnPostgres(t *testing.T) {
	here, there := twoPostgresServices(t)
	here.core.SetService(core.Service{ID: "a", Host: "laptop"})
	there.core.SetService(core.Service{ID: "b", Host: "desktop"})
	here.project(t, "acme")
	tick := watchFactories(t, there)
	beat(here, t, "cc", "box", api.HeartbeatOnline, 10)
	here.reportBase(t, api.ReportBaseInput{State: api.BaseCurrent, Local: "aaa111", Remote: "aaa111"})
	tick()
	got := onlyFactory(t, there)
	if got.Status != api.FactoryOnline || got.ReportsTo != "laptop" {
		t.Fatalf("there, %+v, want Online, reporting to laptop", got)
	}
	if b := factoryStatus(there, t).Base; b == nil || b.State != api.BaseCurrent {
		t.Fatalf("there, the base is %+v, want current", b)
	}
}

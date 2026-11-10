package factory_test

import (
	"context"
	"testing"

	"github.com/tomoram/code-central/core"
	"github.com/tomoram/code-central/factory/factorytest"
	"github.com/tomoram/code-central/store"
)

// pushRemoteURL is the Push remote URL the Factory last reported on the
// harness's Project.
func pushRemoteURL(t *testing.T, h *factorytest.Harness) string {
	t.Helper()
	p, err := h.Human().GetProject(context.Background(), h.Project)
	if err != nil {
		t.Fatal(err)
	}
	return p.CI.PushRemoteURL
}

func TestTheFactoryReportsThePushRemoteURLWhenItStarts(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t)
	h.AddRemote("origin")
	h.Git("remote", "set-url", "origin", "git@github.com:acme/web.git")

	h.Start()
	if got := pushRemoteURL(t, h); got != "git@github.com:acme/web.git" {
		t.Fatalf("push remote URL %q, want origin's", got)
	}
}

func TestThePushRemoteReportedIsTheOneTheLandingMethodNames(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t)
	h.AddRemote("origin")
	h.AddRemote("upstream")
	h.Git("remote", "set-url", "upstream", "https://github.com/acme/web.git")
	h.SetLandingMethod(core.SetLandingMethodInput{Method: store.LandingMerge, Push: true, Remote: "upstream"})

	h.Start()
	if got := pushRemoteURL(t, h); got != "https://github.com/acme/web.git" {
		t.Fatalf("push remote URL %q, want upstream's", got)
	}
}

func TestAFactoryWithNoPushRemoteReportsNothingAndStillStarts(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t)
	id := h.ReadyTicket("Still claimed")
	h.Script(id, factorytest.Wait())

	h.Start()
	h.WaitForState(id, store.InProgress)
	if got := pushRemoteURL(t, h); got != "" {
		t.Fatalf("push remote URL %q, want none reported", got)
	}
}

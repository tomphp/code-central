package core_test

import (
	"context"
	"testing"

	"github.com/tomoram/code-central/core"
)

func TestTheActorTravelsInTheContextAndDefaultsToAgent(t *testing.T) {
	if got := core.ActorOf(context.Background()); got != core.Agent {
		t.Fatalf("default Actor %q, want agent", got)
	}
	if got := core.ActorOf(core.WithActor(context.Background(), core.Human)); got != core.Human {
		t.Fatalf("Actor %q, want human", got)
	}
}

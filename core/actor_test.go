package core_test

import (
	"context"
	"testing"

	"github.com/tomoram/code-central/api"
)

func TestTheActorTravelsInTheContextAndDefaultsToAgent(t *testing.T) {
	if got := api.ActorOf(context.Background()); got != api.Agent {
		t.Fatalf("default Actor %q, want agent", got)
	}
	if got := api.ActorOf(api.WithActor(context.Background(), api.Human)); got != api.Human {
		t.Fatalf("Actor %q, want human", got)
	}
}

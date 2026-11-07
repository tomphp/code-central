package core

import "context"

// Actor is who makes a write: human, agent or integration. Front doors
// put it in the context; MCP and the CLI always write as Agent.
type Actor string

// The Actors.
const (
	Human       Actor = "human"
	Agent       Actor = "agent"
	Integration Actor = "integration"
)

type actorKey struct{}

// WithActor returns ctx carrying the Actor a.
func WithActor(ctx context.Context, a Actor) context.Context {
	return context.WithValue(ctx, actorKey{}, a)
}

// ActorOf is the Actor ctx carries, defaulting to Agent.
func ActorOf(ctx context.Context) Actor {
	if a, ok := ctx.Value(actorKey{}).(Actor); ok {
		return a
	}
	return Agent
}

package ops

import (
	"context"

	"github.com/tomoram/code-central/api"
	"github.com/tomoram/code-central/domain"
	"github.com/tomoram/code-central/events"
)

// Call is one call of an operation through a door's generic dispatch.
type Call struct {
	Op Operation
	// Key is the Project key the request names: the HTTP path's {key} or
	// the MCP URL's; "" on an HTTP route without one.
	Key string
	// In is the input the door decoded, a *In; nil to decode Args.
	In any
	// Args are the arguments as sent, as JSON: MCP's, which Dispatch
	// decodes strictly into a new *In, and its Usage record measures; nil
	// over HTTP.
	Args []byte
	// Actor is the Actor the request names, unchecked: the HTTP API's
	// Actor header, "" when it sends none. MCP names agent.
	Actor api.Actor
}

// Dispatcher calls operations: all the MCP door needs.
type Dispatcher interface {
	Dispatch(ctx context.Context, call Call) (any, error)
}

// Service is all the HTTP door needs: Dispatch for every operation, and
// the reads, streams and Factory posts that aren't operations. ops/bind
// serves the Core as one.
type Service interface {
	Dispatcher
	// Events is the change feed.
	Events() *events.Bus
	Wait(ctx context.Context, key string, ref api.Ref, from domain.State) (api.WaitResult, error)
	Transcript(ctx context.Context, key string, ticket api.Ref, claim int) (api.TranscriptView, error)
	FollowTranscript(ctx context.Context, key string, ticket api.Ref, claim, after int) (TranscriptFeed, error)
	FollowChat(ctx context.Context, key string, after int) (ChatFeed, error)
	Usage(ctx context.Context, in api.UsageInput) (api.UsageView, error)
	SessionUsage(ctx context.Context, in api.SessionUsageInput) (api.SessionUsageView, error)
	CIOverview(ctx context.Context, in api.CIOverviewInput) (api.CIOverview, error)
	Ping(ctx context.Context, sessionID string)
	PostSessionEvents(ctx context.Context, sessionID string, events []api.SessionEvent)
	PostChatEvents(ctx context.Context, key string, in api.PostChatEventsInput)
	PostUsage(ctx context.Context, sessionID string, responses []api.UsageResponse)
}

// A TranscriptFeed follows one Claim's Transcript: the stream read
// (GET .../transcript/stream) sends what Next gives each time Wake fires,
// until Stop.
type TranscriptFeed interface {
	Wake() <-chan struct{}
	Next() (tail bool, events []api.HeldSessionEvent)
	Stop()
}

// A ChatFeed follows a Project's current Conversation: the stream read
// sends the frames Next gives each time Wake fires, until Stop.
type ChatFeed interface {
	Wake() <-chan struct{}
	Next() []api.ChatFrame
	Stop()
}

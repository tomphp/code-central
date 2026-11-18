package ops

import (
	"context"
	"strings"

	"github.com/tomoram/code-central/core"
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
	Actor core.Actor
}

// Dispatcher calls operations: all the MCP door needs.
type Dispatcher interface {
	Dispatch(ctx context.Context, call Call) (any, error)
}

// Service is all the HTTP door needs: Dispatch for every operation, and
// the reads, streams and Factory posts that aren't operations.
type Service interface {
	Dispatcher
	// Events is the change feed.
	Events() *events.Bus
	Wait(ctx context.Context, key string, ref core.Ref, from domain.State) (core.WaitResult, error)
	Transcript(ctx context.Context, key string, ticket core.Ref, claim int) (core.TranscriptView, error)
	FollowTranscript(ctx context.Context, key string, ticket core.Ref, claim, after int) (*core.TranscriptFeed, error)
	FollowChat(ctx context.Context, key string, after int) (*core.ChatFeed, error)
	Usage(ctx context.Context, in core.UsageInput) (core.UsageView, error)
	SessionUsage(ctx context.Context, in core.SessionUsageInput) (core.SessionUsageView, error)
	CIOverview(ctx context.Context, in core.CIOverviewInput) (core.CIOverview, error)
	Ping(ctx context.Context, sessionID string)
	PostSessionEvents(ctx context.Context, sessionID string, events []core.SessionEvent)
	PostChatEvents(ctx context.Context, key string, in core.PostChatEventsInput)
	PostUsage(ctx context.Context, sessionID string, responses []core.UsageResponse)
}

// Serve is c as a Service: the one adapter between core and the doors.
func Serve(c *core.Core) Service { return served{c} }

type served struct{ *core.Core }

// Dispatch invokes the call's operation, first decoding its Args when the
// door decoded no input. A key given to an operation that isn't on one
// Project, whose route has no {key}, must still name a Project: MCP's URL
// names one for every tool, and an unknown one is unknown_project
// whatever the tool.
func (s served) Dispatch(ctx context.Context, call Call) (any, error) {
	in := call.In
	if in == nil {
		in = call.Op.NewInput()
		if err := core.Decode(ctx, call.Args, in); err != nil {
			return nil, err
		}
	}
	if _, pattern := call.Op.Route(); call.Key != "" && !strings.Contains(pattern, "{key}") {
		if _, err := s.GetProject(ctx, call.Key); err != nil {
			return nil, err
		}
	}
	return call.Op.Invoke(ctx, s.Core, call.Key, in)
}

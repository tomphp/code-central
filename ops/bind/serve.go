package bind

import (
	"context"
	"fmt"
	"strings"

	"github.com/tomoram/code-central/api"
	"github.com/tomoram/code-central/core"
	"github.com/tomoram/code-central/ops"
)

// Serve is c as an ops.Service: the one adapter between core and the
// doors.
func Serve(c *core.Core) ops.Service { return served{Core: c, bindings: bindings} }

type served struct {
	*core.Core
	bindings map[ops.Operation]binding
}

// Dispatch invokes the call's operation, first decoding its Args when the
// door decoded no input. A key given to an operation that isn't on one
// Project, whose route has no {key}, must still name a Project: MCP's URL
// names one for every tool, and an unknown one is unknown_project
// whatever the tool.
func (s served) Dispatch(ctx context.Context, call ops.Call) (any, error) {
	b, ok := s.bindings[call.Op]
	if !ok {
		return nil, fmt.Errorf("operation %s has no binding", call.Op.Op().Name)
	}
	in := call.In
	if in == nil {
		in = call.Op.NewInput()
		if err := api.Decode(ctx, call.Args, in); err != nil {
			return nil, err
		}
	}
	if _, pattern := call.Op.Route(); call.Key != "" && !strings.Contains(pattern, "{key}") {
		if _, err := s.GetProject(ctx, call.Key); err != nil {
			return nil, err
		}
	}
	return b.invoke(ctx, s.Core, call.Key, in)
}

// FollowTranscript is the Core's, as the feed ops.Service names: a nil
// feed on an error, never a nil *core.TranscriptFeed inside one.
func (s served) FollowTranscript(ctx context.Context, key string, ticket api.Ref, claim, after int) (ops.TranscriptFeed, error) {
	f, err := s.Core.Factories().FollowTranscript(ctx, key, ticket, claim, after)
	if err != nil {
		return nil, err
	}
	return f, nil
}

// FollowChat is the Core's, as the feed ops.Service names.
func (s served) FollowChat(ctx context.Context, key string, after int) (ops.ChatFeed, error) {
	f, err := s.Core.Factories().FollowChat(ctx, key, after)
	if err != nil {
		return nil, err
	}
	return f, nil
}

// Ping is the Factories area's.
func (s served) Ping(ctx context.Context, sessionID string) { s.Core.Factories().Ping(ctx, sessionID) }

// PostSessionEvents is the Factories area's.
func (s served) PostSessionEvents(ctx context.Context, sessionID string, events []api.SessionEvent) {
	s.Core.Factories().PostSessionEvents(ctx, sessionID, events)
}

// PostChatEvents is the Factories area's.
func (s served) PostChatEvents(ctx context.Context, key string, in api.PostChatEventsInput) {
	s.Core.Factories().PostChatEvents(ctx, key, in)
}

// Transcript is the Factories area's.
func (s served) Transcript(ctx context.Context, key string, ticket api.Ref, claim int) (api.TranscriptView, error) {
	return s.Core.Factories().Transcript(ctx, key, ticket, claim)
}

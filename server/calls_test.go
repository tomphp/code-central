package server_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/tomoram/code-central/core"
	"github.com/tomoram/code-central/httpapi"
	"github.com/tomoram/code-central/ops"
	"github.com/tomoram/code-central/server"
	"github.com/tomoram/code-central/usage"
)

// fakeService records the Actor each dispatch ran as and answers with
// answer and err; any other method panics.
type fakeService struct {
	ops.Service
	actors []core.Actor
	answer any
	err    error
}

func (s *fakeService) Dispatch(ctx context.Context, _ ops.Call) (any, error) {
	s.actors = append(s.actors, core.ActorOf(ctx))
	return s.answer, s.err
}

type recorded struct {
	key  string
	call usage.Call
}

type fakeRecorder []recorded

func (r *fakeRecorder) RecordCall(key string, call usage.Call) {
	*r = append(*r, recorded{key, call})
}

// The middleware puts the Actor a call names in its context, agent when
// it names none, and refuses an unknown one, naming the Actor header,
// before the operation runs.
func TestCallsActAsTheActorTheCallNamesDefaultingToAgent(t *testing.T) {
	next := &fakeService{}
	h := server.Calls(next, &fakeRecorder{}, nil, server.Door{Name: "http"})
	for named, want := range map[core.Actor]core.Actor{"": core.Agent, "agent": core.Agent, "human": core.Human, "integration": core.Integration} {
		next.actors = nil
		if _, err := h.Dispatch(context.Background(), ops.Call{Op: ops.Comment, Key: "acme", Actor: named}); err != nil {
			t.Fatal(err)
		}
		if len(next.actors) != 1 || next.actors[0] != want {
			t.Fatalf("named %q: acted as %v, want %q", named, next.actors, want)
		}
	}

	next.actors = nil
	_, err := h.Dispatch(context.Background(), ops.Call{Op: ops.Comment, Key: "acme", Actor: "robot"})
	var e *core.Error
	if !errors.As(err, &e) || e.Code != core.Invalid || e.Field != httpapi.ActorHeader || len(next.actors) != 0 {
		t.Fatalf("unknown Actor: err %v, dispatched %d", err, len(next.actors))
	}
}

// A door that records Usage gets one Usage record and one log line per
// call, rejected ones included, with the ID it names or returns; a door
// that doesn't gets neither.
func TestCallsRecordUsageAndLogOnlyForADoorThatRecordsIt(t *testing.T) {
	next := &fakeService{answer: map[string]string{"id": "acme-4"}}
	rec, log := &fakeRecorder{}, &bytes.Buffer{}
	mcpDoor := server.Calls(next, rec, log, server.Door{Name: "mcp", Usage: true})

	args := []byte(`{"title":"Search"}`)
	if _, err := mcpDoor.Dispatch(context.Background(), ops.Call{Op: ops.CreateSpec, Key: "acme", Actor: core.Agent, Args: args}); err != nil {
		t.Fatal(err)
	}
	next.answer, next.err = nil, &core.Error{Code: core.NotFound, Message: "No Ticket acme-99."}
	_, _ = mcpDoor.Dispatch(context.Background(), ops.Call{Op: ops.GetTicket, Key: "acme", Actor: core.Agent, Args: []byte(`{"ticket":99}`)})

	created, rejected := len(`{"id":"acme-4"}`), len(`{"code":"not_found","message":"No Ticket acme-99."}`)
	want := []recorded{
		{"acme", usage.Call{Op: "create_spec", ID: "acme-4", RequestBytes: len(args), ResponseBytes: created}},
		{"acme", usage.Call{Op: "get_ticket", ID: "acme-99", RequestBytes: len(`{"ticket":99}`), ResponseBytes: rejected, Error: "not_found"}},
	}
	if len(*rec) != len(want) || (*rec)[0] != want[0] || (*rec)[1] != want[1] {
		t.Fatalf("recorded %+v, want %+v", *rec, want)
	}
	wantLog := fmt.Sprintf("code-central serve: mcp create_spec project=acme id=acme-4 bytes=%d\n", created) +
		fmt.Sprintf("code-central serve: mcp get_ticket project=acme id=acme-99 bytes=%d error=not_found\n", rejected)
	if log.String() != wantLog {
		t.Fatalf("log %q, want %q", log, wantLog)
	}

	*rec, next.err = nil, nil
	log.Reset()
	httpDoor := server.Calls(next, rec, log, server.Door{Name: "http"})
	if _, err := httpDoor.Dispatch(context.Background(), ops.Call{Op: ops.GetTicket, Key: "acme"}); err != nil {
		t.Fatal(err)
	}
	if len(*rec) != 0 || log.Len() != 0 {
		t.Fatalf("http: recorded %+v, logged %q", *rec, log)
	}
}

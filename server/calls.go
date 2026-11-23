package server

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"regexp"

	"github.com/tomoram/code-central/api"
	"github.com/tomoram/code-central/core/foundation"
	"github.com/tomoram/code-central/httpapi"
	"github.com/tomoram/code-central/ops"
	"github.com/tomoram/code-central/usage"
)

// Door is how Calls treats one front door's calls.
type Door struct {
	// Name names the door in the service log, as "mcp".
	Name string
	// Usage, when set, records each call as Usage and logs it. Only MCP's
	// are: Usage is what agents load through their tools (ADR 0065).
	Usage bool
}

// Recorder records a call as Usage under key's Project; *core.Core is one.
type Recorder interface {
	RecordCall(key string, call usage.Call)
}

// Calls is the one middleware on both doors' dispatch. It acts as the
// Actor the call names, agent when it names none, and refuses an unknown
// one, naming the Actor header. Then, for a door that records Usage, it
// records each call, rejected ones included, with rec, and writes its
// line to log (see logCall).
func Calls(next ops.Service, rec Recorder, log io.Writer, d Door) ops.Service {
	if log == nil {
		log = io.Discard
	}
	return calls{Service: next, rec: rec, log: log, door: d}
}

type calls struct {
	ops.Service
	rec  Recorder
	log  io.Writer
	door Door
}

func (m calls) Dispatch(ctx context.Context, call ops.Call) (any, error) {
	a, err := actor(call.Actor)
	if err != nil {
		return nil, err
	}
	v, err := m.Service.Dispatch(api.WithActor(ctx, a), call)
	if m.door.Usage {
		m.record(call, v, err)
	}
	return v, err
}

// actor is the Actor a call names, defaulting to agent.
func actor(named api.Actor) (api.Actor, error) {
	switch named {
	case "":
		return api.Agent, nil
	case api.Human, api.Agent, api.Integration:
		return named, nil
	default:
		return "", &api.Error{Code: api.Invalid, Field: httpapi.ActorHeader, Message: fmt.Sprintf("unknown Actor %q: use human, agent or integration", named)}
	}
}

// record records the call's Usage and logs it. Its response is the JSON
// text the door gives: the result, or the error in the one error shape.
// A result that can't be encoded is the door's to report, so it goes
// unrecorded.
func (m calls) record(call ops.Call, v any, err error) {
	var code api.Code
	if err != nil {
		e := foundation.ErrorOf(err)
		v, code = e, e.Code
	}
	text, merr := json.Marshal(v)
	if merr != nil {
		return
	}
	op, id := call.Op.Op().Name, callID(call.Key, call.Args, text)
	m.logCall(op, call.Key, id, len(text), code)
	m.rec.RecordCall(call.Key, usage.Call{
		Op: op, ID: id, RequestBytes: len(call.Args), ResponseBytes: len(text), Error: string(code),
	})
}

// logCall writes a call's line to the service log:
//
//	code-central serve: mcp get_ticket project=acme id=acme-2 bytes=1234
//
// bytes is the length of the text the agent is given, the context the
// call loads. id is the Ticket or Spec the call names, or else the one it
// returns, and is left out when there is neither; a rejected call ends
// with error=<code>.
func (m calls) logCall(op, key, id string, bytes int, code api.Code) {
	line := fmt.Sprintf("code-central serve: %s %s project=%s", m.door.Name, op, key)
	if id != "" {
		line += " id=" + id
	}
	line += fmt.Sprintf(" bytes=%d", bytes)
	if code != "" {
		line += " error=" + string(code)
	}
	fmt.Fprintln(m.log, line)
}

// callID is the ID a call's arguments name (ticket, spec or target), as
// <key>-<n>, or else the ID its result holds, or "".
func callID(key string, args, text []byte) string {
	var in struct{ Ticket, Spec, Target api.Ref }
	_ = json.Unmarshal(args, &in)
	for _, r := range []api.Ref{in.Ticket, in.Spec, in.Target} {
		if r == "" {
			continue
		}
		if bareNumber.MatchString(string(r)) {
			return key + "-" + string(r)
		}
		return string(r)
	}
	var out struct {
		ID     string `json:"id"`
		Ticket *struct {
			ID string `json:"id"`
		} `json:"ticket"`
	}
	_ = json.Unmarshal(text, &out)
	if out.ID == "" && out.Ticket != nil {
		return out.Ticket.ID
	}
	return out.ID
}

var bareNumber = regexp.MustCompile(`^[0-9]+$`)

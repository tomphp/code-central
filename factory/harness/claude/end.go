// How a Turn ended: the result claude printed last, its exit, and the
// neutral Outcome they make.

package claude

import (
	"encoding/json"
	"fmt"

	"github.com/tomoram/code-central/factory/harness"
)

// result is the stream-json result message that ends a Turn.
type result struct {
	Subtype           string   `json:"subtype"`
	IsError           bool     `json:"is_error"`
	TerminalReason    string   `json:"terminal_reason"`
	APIErrorStatus    int      `json:"api_error_status"`
	PermissionDenials []denial `json:"permission_denials"`
	Result            string   `json:"result"`
}

// denial is a call the Turn had denied, as the result lists it.
type denial struct {
	Tool  string          `json:"tool_name"`
	ID    string          `json:"tool_use_id"`
	Input json.RawMessage `json:"tool_input"`
}

// end is how a Turn ended: how claude exited, its result message if it
// printed one, and what its stream said along the way.
type end struct {
	exit     harness.Exit
	result   *result
	lastText string
	// apiError is the API error a message reported, such as "rate_limit"
	// or "authentication_failed"; empty when there was none.
	apiError     string
	reviewCalled bool
}

// outcome is the end as an Outcome, stopped when its Turn was.
func (e end) outcome(stopped bool) harness.Outcome {
	o := harness.Outcome{
		Kind:         e.kind(stopped),
		ExitSummary:  e.String(),
		ExitDetail:   e.how(),
		Ended:        e.result != nil,
		LastText:     e.lastText,
		ErrorClass:   e.errorClass(),
		ReviewCalled: e.reviewCalled,
	}
	if e.result != nil {
		o.EndText = e.result.Result
		for _, d := range e.result.PermissionDenials {
			o.Denials = append(o.Denials, harness.Denial{Tool: d.Tool, Input: d.Input})
		}
	}
	return o
}

func (e end) String() string {
	s := e.exitString()
	if e.result == nil {
		return s + ", no result"
	}
	s += ", " + e.result.Subtype
	if e.result.TerminalReason != "" {
		s += " (" + e.result.TerminalReason + ")"
	}
	return s
}

// exitString is how claude's process ended: the signal that ended it,
// else its exit code.
func (e end) exitString() string {
	if e.exit.Signal != "" {
		return e.exit.Signal
	}
	return fmt.Sprintf("exit %d", e.exit.Code)
}

// how is how the Turn ended in full: its exit code or signal, subtype and
// terminal reason.
func (e end) how() string {
	s := e.exitString()
	if e.result == nil {
		return s + ", with no result message"
	}
	s += ", subtype " + e.result.Subtype
	if e.result.TerminalReason != "" {
		s += ", terminal_reason " + e.result.TerminalReason
	}
	return s
}

// kind classifies the end, first match winning: stopped when its Turn
// was; then an expired login, a rejected model and an Outage, read from
// its API error before its exit and result; then done when it exited 0
// with a result that isn't an error; else failed.
func (e end) kind(stopped bool) harness.Kind {
	switch class := e.errorClass(); {
	case stopped:
		return harness.Stopped
	case class == authFailed:
		return harness.LoginFailed
	case class == modelNotFound:
		return harness.ModelRejected
	case outageErrors[class]:
		return harness.Outage
	case e.exit.Code == 0 && e.exit.Signal == "" && e.result != nil && !e.result.IsError:
		return harness.Done
	}
	return harness.Failed
}

// outageErrors are the API errors of an Outage: the trouble is the
// service's or the account's limits, not the Ticket's: usage and rate
// limits, overload, server and network errors. A Claim's Turn cut off by
// one is resumed later, never failed.
//
// Source: docs/research/agent-sdk-host.md (ticket 50, on branch
// research/agent-sdk-host), section 5, names the stream-json
// `system/api_retry` event's `error` values `rate_limit`,
// `authentication_failed` and `billing_error` (the usage limit), from the
// headless docs. The other codes here (overload, server, network and
// timeout, and the `_error` spellings) are unverified: neither that note
// nor docs/research/claude-code-session-host.md lists them, nor the
// result's `terminal_reason` values.
var outageErrors = map[string]bool{
	"rate_limit":       true,
	"rate_limit_error": true,
	"billing_error":    true,
	"overloaded":       true,
	"overloaded_error": true,
	"server_error":     true,
	"api_error":        true,
	"network_error":    true,
	"connection_error": true,
	"timeout":          true,
}

// authFailed is the API error of an expired or missing login, which only
// the human can fix.
const authFailed = "authentication_failed"

// modelNotFound is the API error of a model claude rejects: the Turn's
// model, not the service or the Factory, is at fault.
const modelNotFound = "model_not_found"

// errorClass is the API error the Turn ended on, from its messages or its
// result's terminal reason. An api_error with status 404 is a model
// claude rejects, even with no message saying so.
func (e end) errorClass() string {
	if e.apiError != "" {
		return e.apiError
	}
	if e.result != nil && e.result.TerminalReason == "api_error" && e.result.APIErrorStatus == 404 {
		return modelNotFound
	}
	if e.result != nil && (outageErrors[e.result.TerminalReason] || e.result.TerminalReason == authFailed) {
		return e.result.TerminalReason
	}
	return ""
}

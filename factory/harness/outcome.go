// The neutral Outcome of a Turn, the only thing the Factory reads of how
// it ended.

package harness

import "encoding/json"

// Kind is how a Turn ended, classified.
type Kind string

const (
	Done           Kind = "done"
	Failed         Kind = "failed"
	Outage         Kind = "outage"
	LoginFailed    Kind = "login failed"
	ModelRejected  Kind = "model rejected"
	EffortRejected Kind = "effort rejected"
	Stopped        Kind = "stopped"
)

// Outcome is how a Turn ended, in terms that name no Harness's flags or
// output.
type Outcome struct {
	Kind Kind
	// ExitSummary says how the Turn ended briefly, as the "turn ended"
	// line does, and ExitDetail says it in full, as Failure Reports and
	// Chat do; both empty for a Turn that never started.
	ExitSummary, ExitDetail string
	// Ended is whether the Harness reported the session's own end: a
	// Turn without one may have been refused before it began, as by an
	// argument.
	Ended bool
	// LastText is the agent's last text, and EndText the text the
	// Harness ended the Turn with.
	LastText, EndText string
	// ErrorClass is the API error the Turn ended on, such as
	// "rate_limit" or "model_not_found"; empty when there was none.
	ErrorClass string
	// Denials are the calls the Turn had denied that the human could
	// allow.
	Denials []Denial
	// ReviewCalled is whether the Turn called the code-review skill.
	ReviewCalled bool
	// StartErr is why the Turn's process never started.
	StartErr error
	// Efforts are the effort levels the Harness accepts, for an effort
	// rejected Turn.
	Efforts []string
}

// Denial is one call a Turn had denied: its tool and that tool's input.
type Denial struct {
	Tool  string
	Input json.RawMessage
}

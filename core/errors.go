package core

import "fmt"

// Code names the kind of an Error. The set is fixed by the spec.
type Code string

// Error codes.
const (
	UnknownProject   Code = "unknown_project"
	NotFound         Code = "not_found"
	ActionNotAllowed Code = "action_not_allowed"
	StaleClaim       Code = "stale_claim"
	Invalid          Code = "invalid"
	Cycle            Code = "cycle"
	// ServiceUnavailable is produced only by the client, never by core.
	ServiceUnavailable Code = "service_unavailable"
)

// Error is the one error shape every front door uses.
type Error struct {
	Code           Code     `json:"code"`
	Message        string   `json:"message"`
	AllowedActions []string `json:"allowed_actions,omitempty"`
	Field          string   `json:"field,omitempty"`
}

func (e *Error) Error() string {
	if e.Field != "" {
		return fmt.Sprintf("%s (%s): %s", e.Code, e.Field, e.Message)
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

func invalid(field, format string, args ...any) *Error {
	return &Error{Code: Invalid, Field: field, Message: fmt.Sprintf(format, args...)}
}

func unknownProject(key string) *Error {
	return &Error{Code: UnknownProject, Message: fmt.Sprintf("no Project with key %q; ask the human to run `code-central init` in its repository", key)}
}

func notFound(format string, args ...any) *Error {
	return &Error{Code: NotFound, Message: fmt.Sprintf(format, args...)}
}

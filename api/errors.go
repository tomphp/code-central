package api

import (
	"fmt"
)

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
	// Conflict means another service sharing the Store wrote what the
	// write read at the same moment, until its attempts ran out: the
	// write changed nothing, and trying again may succeed.
	Conflict Code = "conflict"
	// ServiceUnavailable is produced only by the client, never by core.
	ServiceUnavailable Code = "service_unavailable"
)

// Error is the one error shape every front door uses.
type Error struct {
	Code           Code     `json:"code"`
	Message        string   `json:"message"`
	AllowedActions []string `json:"allowed_actions,omitempty"`
	Field          string   `json:"field,omitempty"`
	// Fix says how the human puts right what was refused, when the
	// message alone doesn't: a failed CI check's fix, for one.
	Fix string `json:"fix,omitempty"`
}

func (e *Error) Error() string {
	if e.Field != "" {
		return fmt.Sprintf("%s (%s): %s", e.Code, e.Field, e.Message)
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

// InvalidField is an invalid Error about field.
func InvalidField(field, format string, args ...any) *Error {
	return &Error{Code: Invalid, Field: field, Message: fmt.Sprintf(format, args...)}
}

// UnknownProjectError is the Error for a Project key that names none.
func UnknownProjectError(key string) *Error {
	return &Error{Code: UnknownProject, Message: fmt.Sprintf("no Project with key %q; ask the human to run `code-central init` in its repository", key)}
}

// NotFoundError is a not_found Error.
func NotFoundError(format string, args ...any) *Error {
	return &Error{Code: NotFound, Message: fmt.Sprintf(format, args...)}
}

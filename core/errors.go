package core

import (
	"errors"
	"fmt"

	"github.com/tomoram/code-central/store"
)

// ErrorOf is err in the one error shape: the *Error it is or wraps, a
// conflict for store.ErrConflict, or else an internal one, as something
// broke inside the service.
func ErrorOf(err error) *Error {
	var e *Error
	switch {
	case errors.As(err, &e):
	case errors.Is(err, store.ErrConflict):
		e = &Error{Code: Conflict, Message: "another service changed this at the same moment, so nothing was written; try again"}
	default:
		e = &Error{Code: "internal", Message: err.Error()}
	}
	return e
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

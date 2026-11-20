package core

import (
	"errors"
	"fmt"

	"github.com/tomoram/code-central/api"
	"github.com/tomoram/code-central/store"
)

// ErrorOf is err in the one error shape: the *Error it is or wraps, a
// conflict for store.ErrConflict, or else an internal one, as something
// broke inside the service.
func ErrorOf(err error) *api.Error {
	var e *api.Error
	switch {
	case errors.As(err, &e):
	case errors.Is(err, store.ErrConflict):
		e = &api.Error{Code: api.Conflict, Message: "another service changed this at the same moment, so nothing was written; try again"}
	default:
		e = &api.Error{Code: "internal", Message: err.Error()}
	}
	return e
}

func invalid(field, format string, args ...any) *api.Error {
	return &api.Error{Code: api.Invalid, Field: field, Message: fmt.Sprintf(format, args...)}
}

func unknownProject(key string) *api.Error {
	return &api.Error{Code: api.UnknownProject, Message: fmt.Sprintf("no Project with key %q; ask the human to run `code-central init` in its repository", key)}
}

func notFound(format string, args ...any) *api.Error {
	return &api.Error{Code: api.NotFound, Message: fmt.Sprintf(format, args...)}
}

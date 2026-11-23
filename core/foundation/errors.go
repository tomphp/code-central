// Package foundation holds what every area of the service shares: the
// helpers that need the Store, the clock or the bus but no area's state.
// It imports neither core nor an area.
package foundation

import (
	"errors"

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

package core

import (
	"github.com/tomoram/code-central/api"
	"github.com/tomoram/code-central/core/foundation"
	"github.com/tomoram/code-central/domain"
	"github.com/tomoram/code-central/store"
)

// A Ticket read loads only the History parts it names (Spec
// code-central-1032), so a write names the parts it read: readForWrite
// returns them as a held, and held.put writes exactly those back.

// held is the History parts a Ticket was read with for a write.
type held []store.Part

// whole is a Ticket read or written with its whole History: a write's
// usual read, since one Ticket's History is small and an action's checks,
// its records and its acknowledgement then see all of it, and every new
// Ticket's write.
var whole = held(store.History)

// readForWrite reads the Ticket ref names, op's field, with the parts h
// names, for a write that h.put makes.
func readForWrite(r store.Reader, key, op, field string, ref api.Ref, h held) (domain.Ticket, held, error) {
	t, err := foundation.ResolveTicket(r, key, op, field, ref, h...)
	return t, h, err
}

// put writes t with the parts h names, the others staying as stored, and
// reads it back as the Store now holds it, so its derived head fields are
// fresh for the views built from it.
func (h held) put(w store.Writer, key string, t *domain.Ticket) error {
	if err := w.PutTicket(key, *t, h...); err != nil {
		return err
	}
	*t, _ = w.Ticket(key, t.Number, h...)
	return nil
}

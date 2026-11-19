package cli

import (
	"github.com/tomoram/code-central/domain"
	"github.com/tomoram/code-central/store"
)

// WindowsArg exposes the scheduled task's argument quoting to its test.
var WindowsArg = windowsArg

// AddATicketOnMove makes store move's copy write one more Ticket into
// the target than the source holds, until t ends, so the counts differ.
func AddATicketOnMove(t interface{ Cleanup(func()) }) {
	was := copyStore
	copyStore = func(to, from store.Store) error {
		if err := was(to, from); err != nil {
			return err
		}
		return to.Update(func(w store.Writer) error {
			return w.PutTicket("acme", domain.Ticket{Number: 99, Title: "Extra", State: domain.Ready, Spec: 2})
		})
	}
	t.Cleanup(func() { copyStore = was })
}

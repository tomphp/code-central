package foundation

import (
	"context"
	"fmt"

	"github.com/tomoram/code-central/api"
	"github.com/tomoram/code-central/domain"
	"github.com/tomoram/code-central/store"
)

// ArchivedError is the refusal of a write to Archived Project key.
func ArchivedError(key string) *api.Error {
	return &api.Error{
		Code:           api.ActionNotAllowed,
		Message:        fmt.Sprintf("Project %s is archived, so it takes no writes; only the human can bring it back, with unarchive_project", key),
		AllowedActions: []string{"unarchive_project"},
	}
}

// Writable is the check every write makes on its Project first: it
// exists and isn't archived.
func Writable(r store.Reader, key string) error {
	p, ok := r.Project(key)
	if !ok {
		return api.UnknownProjectError(key)
	}
	if !p.Archived.IsZero() {
		return ArchivedError(key)
	}
	return nil
}

// HumanOnly refuses op, a human action, when ctx is made as another Actor.
func HumanOnly(ctx context.Context, op string) error {
	if a := api.ActorOf(ctx); a != api.Human {
		return &api.Error{Code: api.ActionNotAllowed, Message: fmt.Sprintf("%s is a human action; this request is made as %s", op, a)}
	}
	return nil
}

// ArchiveGuard wraps a store so no write lands on an Archived Project.
// A Project's own record may change only by being unarchived.
type ArchiveGuard struct{ store.Store }

func (g ArchiveGuard) Update(fn func(store.Writer) error) error {
	return g.Store.Update(func(w store.Writer) error { return fn(guardedWriter{w}) })
}

type guardedWriter struct{ store.Writer }

func (w guardedWriter) PutProject(p domain.Project) error {
	if old, ok := w.Project(p.Key); ok && !old.Archived.IsZero() && !p.Archived.IsZero() {
		return ArchivedError(p.Key)
	}
	return w.Writer.PutProject(p)
}

func (w guardedWriter) PutSpec(key string, s domain.Spec) error {
	if err := Writable(w, key); err != nil {
		return err
	}
	return w.Writer.PutSpec(key, s)
}

func (w guardedWriter) PutTicket(key string, t domain.Ticket, parts ...store.Part) error {
	if err := Writable(w, key); err != nil {
		return err
	}
	return w.Writer.PutTicket(key, t, parts...)
}

func (w guardedWriter) PutCIRuns(key string, runs []domain.CIRun) error {
	if err := Writable(w, key); err != nil {
		return err
	}
	return w.Writer.PutCIRuns(key, runs)
}

func (w guardedWriter) PutImport(key string, im store.Import) error {
	if err := Writable(w, key); err != nil {
		return err
	}
	return w.Writer.PutImport(key, im)
}

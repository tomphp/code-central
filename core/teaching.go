package core

import (
	"context"
	"embed"

	"github.com/tomoram/code-central/store"
)

// A Project's mode (store.Mode) says where init puts its files: committed
// to the repository, or kept local to one checkout (`init --local`).
const (
	ModeCommitted = store.ModeCommitted
	ModeLocal     = store.ModeLocal
)

func invalidMode(m store.Mode) *Error {
	return invalid("mode", "mode %q is neither %q nor %q", m, ModeCommitted, ModeLocal)
}

// SetProjectModeInput is set_project_mode's input.
type SetProjectModeInput struct {
	Mode store.Mode `json:"mode"`
}

// SetProjectMode is set_project_mode(key, mode): init calls it when the
// human agrees to switch a Project between committed and local.
func (c *Core) SetProjectMode(_ context.Context, key string, in SetProjectModeInput) (ProjectView, error) {
	if !in.Mode.Valid() {
		return ProjectView{}, invalidMode(in.Mode)
	}
	var v ProjectView
	err := c.store.Update(func(w store.Writer) error {
		if err := writable(w, key); err != nil {
			return err
		}
		p, _ := w.Project(key)
		unchanged := p.Mode == in.Mode
		p.Mode = in.Mode
		v = projectView(p)
		if unchanged {
			return nil
		}
		return w.PutProject(p)
	})
	return v, err
}

// TeachingFile is one docs/agents/ file init writes into a repository.
type TeachingFile struct {
	// Path is relative to the repository root, with forward slashes.
	Path string
	Body string
}

//go:embed teaching/*.md
var teaching embed.FS

// TeachingFiles are the docs/agents/ files init writes: pointers that map
// the skills' operations to Code Central's by name and send agents to
// get_workflow for the rules, so nothing in a repository goes stale.
func TeachingFiles() []TeachingFile {
	var out []TeachingFile
	for _, name := range []string{"issue-tracker.md", "triage-labels.md"} {
		b, err := teaching.ReadFile("teaching/" + name)
		if err != nil {
			panic(err)
		}
		out = append(out, TeachingFile{Path: "docs/agents/" + name, Body: string(b)})
	}
	return out
}

package core

import (
	"context"
	"strings"

	"github.com/tomoram/code-central/api"
	"github.com/tomoram/code-central/core/foundation"
	"github.com/tomoram/code-central/domain"
	"github.com/tomoram/code-central/store"
)

// The Check Command (Spec code-central-1229): the shell command an agent
// runs in a Repository to check its code, set for a Project as the
// default and for a Repository as its own.

// parseCheckCommand is command trimmed, refusing one that holds a line
// break, as a single shell command can't.
func parseCheckCommand(command string) (string, error) {
	command = strings.TrimSpace(command)
	if strings.ContainsAny(command, "\r\n") {
		return "", &api.Error{Code: api.Invalid, Field: "check_command", Message: "a Check Command is one line, and this one holds a line break",
			Fix: "give one shell command, such as npm run check, joining several with &&"}
	}
	return command, nil
}

// setRepositoryCheckCommand sets r's own Check Command from command, when
// given: empty has it use the Project's.
func setRepositoryCheckCommand(r *domain.Repository, command *string) error {
	if command == nil {
		return nil
	}
	own, err := parseCheckCommand(*command)
	if err != nil {
		return err
	}
	r.CheckCommand = own
	return nil
}

// effectiveCheckCommand is r's Check Command in p and whether it is its
// own: its own, else the Project's, else none.
func effectiveCheckCommand(p domain.Project, r domain.Repository) (string, bool) {
	if r.CheckCommand != "" {
		return r.CheckCommand, true
	}
	return p.CheckCommand, false
}

// SetCheckCommand is the human's set_check_command: the Project's Check
// Command, which every Repository without its own uses; empty for none.
// Like the Project's other settings it adds no Move, Edit or Comment; the
// Project's notice announces it.
func (c *Core) SetCheckCommand(ctx context.Context, key string, in api.SetCheckCommandInput) (api.Written[api.ProjectView], error) {
	if err := foundation.HumanOnly(ctx, "set_check_command"); err != nil {
		return api.Written[api.ProjectView]{}, err
	}
	command, err := parseCheckCommand(in.CheckCommand)
	if err != nil {
		return api.Written[api.ProjectView]{}, err
	}
	var before, after api.ProjectView
	err = c.store.Update(func(w store.Writer) error {
		if err := foundation.Writable(w, key); err != nil {
			return err
		}
		p, _ := w.Project(key)
		before = c.projectView(p)
		if p.CheckCommand == command {
			after = before
			return nil
		}
		p.CheckCommand = command
		after = c.projectView(p)
		return w.PutProject(p)
	})
	if err != nil {
		return api.Written[api.ProjectView]{}, err
	}
	return acknowledgeProject(before, after, in.Full, c.now())
}

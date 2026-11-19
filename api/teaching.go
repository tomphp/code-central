package api

import "github.com/tomoram/code-central/domain"

// SetProjectModeInput is set_project_mode's input.
type SetProjectModeInput struct {
	Mode domain.Mode `json:"mode"`
}

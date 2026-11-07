package httpapi

import (
	"net/http"

	"github.com/tomoram/code-central/core"
)

// Finished work and Archived Projects (ticket 39). Both take an empty
// JSON body.
func init() {
	Routes["archive_project"] = "POST /api/p/{key}/archive_project"
	Routes["unarchive_project"] = "POST /api/p/{key}/unarchive_project"

	handlers["archive_project"] = func(c *core.Core, r *http.Request) (any, error) {
		var in struct{}
		if err := decodeBody(r, &in); err != nil {
			return nil, err
		}
		return c.ArchiveProject(r.Context(), r.PathValue("key"))
	}
	handlers["unarchive_project"] = func(c *core.Core, r *http.Request) (any, error) {
		var in struct{}
		if err := decodeBody(r, &in); err != nil {
			return nil, err
		}
		return c.UnarchiveProject(r.Context(), r.PathValue("key"))
	}
}

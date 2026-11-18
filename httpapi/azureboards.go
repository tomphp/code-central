package httpapi

import (
	"net/http"

	"github.com/tomoram/code-central/core"
)

// Azure Boards (Spec code-central-894): the human's Azure Boards settings.
func init() {
	legacyRoutes["set_azure_boards"] = "POST /api/p/{key}/set_azure_boards"

	handlers["set_azure_boards"] = func(c *core.Core, r *http.Request) (any, error) {
		var in core.SetAzureBoardsInput
		if err := decodeBody(r, &in); err != nil {
			return nil, err
		}
		return c.SetAzureBoards(r.Context(), r.PathValue("key"), in)
	}
}

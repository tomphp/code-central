// Package frontdoor builds the service's one handler: MCP at /p/<key>/mcp,
// the HTTP API under /api/ and the embedded UI at /, on one ServeMux,
// behind the one Guard middleware.
package frontdoor

import (
	"io"
	"net/http"

	"github.com/tomoram/code-central/core"
	"github.com/tomoram/code-central/httpapi"
	"github.com/tomoram/code-central/mcp"
)

// Handler returns the service's handler. ui may be nil; log, when not
// nil, gets a line per MCP tool call; bindHosts are extra addresses (from
// --bind) the Guard accepts as Host and Origin.
func Handler(c *core.Core, ui http.Handler, log io.Writer, bindHosts ...string) http.Handler {
	mux := http.NewServeMux()
	httpapi.Register(mux, c)
	mcp.Register(mux, c, log)
	if ui != nil {
		mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodGet && r.Method != http.MethodHead {
				w.WriteHeader(http.StatusMethodNotAllowed)
				return
			}
			ui.ServeHTTP(w, r)
		})
	}
	return Guard(mux, bindHosts...)
}

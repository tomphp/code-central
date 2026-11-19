// Package server builds the service's one handler: MCP at /p/<key>/mcp,
// the HTTP API under /api/ and the embedded UI at /, on one ServeMux,
// behind the Guard. Both doors reach the Core through bind.Serve, wrapped
// in the one Calls middleware for the Actor and Usage.
package server

import (
	"io"
	"net/http"

	"github.com/tomoram/code-central/core"
	"github.com/tomoram/code-central/httpapi"
	"github.com/tomoram/code-central/mcp"
	"github.com/tomoram/code-central/ops/bind"
)

// Handler returns the service's handler. ui may be nil; log, when not
// nil, gets a line per MCP tool call; bindHosts are extra addresses (from
// --bind) the Guard accepts as Host and Origin.
func Handler(c *core.Core, ui http.Handler, log io.Writer, bindHosts ...string) http.Handler {
	mux := http.NewServeMux()
	s := bind.Serve(c)
	httpapi.Register(mux, Calls(s, c, log, Door{Name: "http"}))
	mcp.Register(mux, Calls(s, c, log, Door{Name: "mcp", Usage: true}))
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

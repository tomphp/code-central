package frontdoor

import (
	"net/http"

	"github.com/tomoram/code-central/core"
)

// VersionHeader carries the service's version on every response, so a
// CLI of another version can warn.
const VersionHeader = "X-Code-Central-Version"

// ClientVersionHeader carries the client's version on a request, so an
// unknown field's error can say whether the versions differ.
const ClientVersionHeader = "X-Code-Central-Client-Version"

// Versioned sets VersionHeader to version on every response from h, and
// gives every request's context the service's version and the client's
// from ClientVersionHeader, for core.Decode.
func Versioned(h http.Handler, version string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set(VersionHeader, version)
		ctx := core.WithVersions(r.Context(), version, r.Header.Get(ClientVersionHeader))
		h.ServeHTTP(w, r.WithContext(ctx))
	})
}

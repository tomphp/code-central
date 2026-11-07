package frontdoor

import "net/http"

// VersionHeader carries the service's version on every response, so a
// CLI of another version can warn.
const VersionHeader = "X-Code-Central-Version"

// Versioned sets VersionHeader to version on every response from h.
func Versioned(h http.Handler, version string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set(VersionHeader, version)
		h.ServeHTTP(w, r)
	})
}

package frontdoor

import (
	"net"
	"net/http"
	"net/url"
	"strings"
)

// LocalHosts are the host names the service answers to: loopback and the
// name a Docker container uses to reach its host.
var LocalHosts = []string{"localhost", "127.0.0.1", "::1", "host.docker.internal"}

// Guard is the one middleware in front of MCP and the HTTP API. It stops
// DNS rebinding and cross-site browser requests:
//   - the Host must be one of LocalHosts or an extra (--bind) address;
//   - a missing Origin is allowed (non-browser clients send none), but a
//     present one must name one of those hosts.
//
// Anything else gets 403. Guard never sends CORS headers, so a browser on
// another origin cannot read a response either.
func Guard(next http.Handler, extraHosts ...string) http.Handler {
	allowed := map[string]bool{}
	for _, h := range append(append([]string{}, LocalHosts...), extraHosts...) {
		allowed[strings.ToLower(strings.Trim(h, "[]"))] = true
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		host, ok := hostname(r.Host)
		if !ok || !allowed[host] {
			http.Error(w, "forbidden: Host must be localhost, 127.0.0.1, [::1] or host.docker.internal", http.StatusForbidden)
			return
		}
		if origins, present := r.Header["Origin"]; present {
			u, err := url.Parse(strings.Join(origins, ""))
			if err != nil || u.Host == "" || !allowed[strings.ToLower(u.Hostname())] {
				http.Error(w, "forbidden: cross-origin requests are not accepted", http.StatusForbidden)
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

// hostname strips an optional port and IPv6 brackets from a Host header.
// A bare IPv6 address without brackets is not a valid Host.
func hostname(hostport string) (string, bool) {
	if hostport == "" {
		return "", false
	}
	h := hostport
	if strings.Contains(hostport, "]") || strings.Count(hostport, ":") == 1 {
		if hh, _, err := net.SplitHostPort(hostport); err == nil {
			h = hh
		} else if strings.HasPrefix(hostport, "[") && strings.HasSuffix(hostport, "]") {
			h = hostport[1 : len(hostport)-1]
		} else {
			return "", false
		}
	} else if strings.Contains(hostport, ":") {
		return "", false
	}
	return strings.ToLower(h), true
}

package client

import (
	"fmt"
	"net"
	"net/url"
	"testing"
)

// realServicePort is the port of the human's own service, the one every
// default falls through to.
const realServicePort = "7733"

// refuseRealService panics when a test binary is about to send a request
// to the real service, or to anything off this machine, so a test that
// forgot CODE_CENTRAL_URL fails at once instead of writing to the human's
// Projects. Outside a test binary it allows every address.
func refuseRealService(baseURL string) {
	if !testing.Testing() {
		return
	}
	u, err := url.Parse(baseURL)
	if err != nil {
		panic(fmt.Sprintf("client: a test used the unparseable service URL %q", baseURL))
	}
	host := u.Hostname()
	if ip := net.ParseIP(host); host != "localhost" && (ip == nil || !ip.IsLoopback()) {
		panic(fmt.Sprintf("client: a test reached %s, which is not on this machine; start a test service and set CODE_CENTRAL_URL to it", baseURL))
	}
	port := u.Port()
	if port == "" && u.Scheme == "http" {
		port = "80"
	}
	if port == realServicePort {
		panic(fmt.Sprintf("client: a test reached %s, the real Code Central service; start a test service (an httptest server, or `code-central serve --store <scratch> --port <spare>`) and set CODE_CENTRAL_URL to it", baseURL))
	}
}

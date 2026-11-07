package cli

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"sync"

	"github.com/tomoram/code-central/client"
)

// Version is stamped by `make build` with `git describe --tags --always --dirty`.
var Version = "dev"

// version is this CLI's version: Env.Version (tests) else Version.
func (e Env) version() string {
	if e.Version != "" {
		return e.Version
	}
	return Version
}

// newClient is the client every command that talks to the service uses
// (except the hook commands, which stay silent). It warns once on stderr,
// without failing, when the service's version differs from this CLI's.
func newClient(e Env) *client.Client {
	c := client.New(serviceURL(e))
	var once sync.Once
	c.OnVersion = func(service string) {
		if service == e.version() {
			return
		}
		once.Do(func() { fmt.Fprintln(e.Stderr, versionWarning(e.version(), service)) })
	}
	return c
}

// versionWarning names the older side only when both are clean tags;
// otherwise versions are compared for equality only.
func versionWarning(cli, service string) string {
	c, cok := tagParts(cli)
	s, sok := tagParts(service)
	if cok && sok {
		switch compareParts(c, s) {
		case 1:
			return fmt.Sprintf("code-central: warning: the service (%s) is older than this CLI (%s); run `code-central service restart`", service, cli)
		case -1:
			return fmt.Sprintf("code-central: warning: this CLI is older than the service (CLI %s, service %s)", cli, service)
		}
	}
	return fmt.Sprintf("code-central: warning: this CLI's version %s differs from the service's %s", cli, service)
}

// cleanTag is what `git describe --tags --always --dirty` prints on a
// clean, tagged commit.
var cleanTag = regexp.MustCompile(`^v?\d+(\.\d+)*$`)

func tagParts(v string) ([]int, bool) {
	if !cleanTag.MatchString(v) {
		return nil, false
	}
	var out []int
	for _, p := range strings.Split(strings.TrimPrefix(v, "v"), ".") {
		n, err := strconv.Atoi(p)
		if err != nil {
			return nil, false
		}
		out = append(out, n)
	}
	return out, true
}

func compareParts(a, b []int) int {
	for i := 0; i < len(a) || i < len(b); i++ {
		var x, y int
		if i < len(a) {
			x = a[i]
		}
		if i < len(b) {
			y = b[i]
		}
		if x != y {
			if x > y {
				return 1
			}
			return -1
		}
	}
	return 0
}

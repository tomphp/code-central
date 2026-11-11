// Package azdourl parses Azure DevOps project URLs. It imports nothing of
// Code Central, so core and the Azure DevOps client can both use it.
package azdourl

import (
	"fmt"
	"net/url"
	"strings"
)

// ParseProjectURL turns what a human typed, an Azure DevOps project URL as
// their browser shows it, into the stored project URL: scheme, host and
// the path to the project, with no login, query, fragment or trailing
// slash. A path cut at its first segment starting with "_" (_build, _git,
// _settings) loses a deeper page's tail. The path must then be:
//
//   - dev.azure.com: /<organization>/<project>
//   - <organization>.visualstudio.com: /<project> or /DefaultCollection/<project>
//   - any other host, an Azure DevOps Server: /<collection>/<project>, or deeper
//
// Percent-encoding is kept as given, and a typed space is encoded.
func ParseProjectURL(text string) (string, error) {
	text = strings.TrimSpace(text)
	// shown is what errors echo: never a login, which may hold a secret.
	shown := text
	bad := func(why string) (string, error) {
		return "", fmt.Errorf("%q is not an Azure DevOps project URL: %s, such as https://dev.azure.com/acme/web", shown, why)
	}
	if text == "" {
		return bad("it is empty")
	}
	u, err := url.Parse(strings.ReplaceAll(text, " ", "%20"))
	if err != nil {
		shown = ""
		return bad("it can't be read as a URL")
	}
	if u.User != nil {
		u.User = nil
		shown = u.String()
	}
	if u.Scheme != "https" && u.Scheme != "http" {
		return bad("it must start with https://")
	}
	if u.Host == "" || u.Hostname() == "" {
		return bad("it has no host")
	}
	host := strings.ToLower(u.Host)

	var segs []string
	if path := strings.TrimSuffix(u.EscapedPath(), "/"); path != "" {
		segs = strings.Split(strings.TrimPrefix(path, "/"), "/")
	}
	for i, s := range segs {
		if strings.HasPrefix(s, "_") {
			segs = segs[:i]
			break
		}
	}
	for _, s := range segs {
		if s == "" {
			return bad("its path has an empty part")
		}
	}

	name := strings.ToLower(u.Hostname())
	switch {
	case name == "dev.azure.com":
		if len(segs) != 2 {
			return bad("on dev.azure.com it is /<organization>/<project>")
		}
	case strings.HasSuffix(name, ".visualstudio.com"):
		switch {
		case len(segs) == 1:
		case len(segs) == 2 && strings.EqualFold(segs[0], "DefaultCollection"):
		default:
			return bad("on visualstudio.com it is /<project> or /DefaultCollection/<project>")
		}
	default:
		if len(segs) < 2 {
			return bad("on Azure DevOps Server it is /<collection>/<project>")
		}
	}
	return u.Scheme + "://" + host + "/" + strings.Join(segs, "/"), nil
}

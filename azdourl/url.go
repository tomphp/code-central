// Package azdourl parses Azure DevOps project and saved query URLs. It
// imports nothing of Code Central, so core and the Azure DevOps client can
// both use it.
package azdourl

import (
	"fmt"
	"net/url"
	"regexp"
	"slices"
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

// ParseQueryURL turns what a human typed, the URL of a saved Azure DevOps
// query as their browser shows it, into the stored query URL: the project
// URL ParseProjectURL gives, then /_queries/query/<id>. Its path must
// carry the query's ID, a GUID, after _queries/query or
// _queries/query-edit, and nothing after it but a slash.
func ParseQueryURL(text string) (string, error) {
	bad := func(why string) (string, error) {
		return "", fmt.Errorf("%q is not the URL of a saved Azure DevOps query: %s, such as https://dev.azure.com/acme/web/_queries/query/<id>", withoutLogin(text), why)
	}
	if strings.TrimSpace(text) == "" {
		return bad("it is empty")
	}
	project, err := ParseProjectURL(text)
	if err != nil {
		return bad("it doesn't name an Azure DevOps project")
	}
	u, _ := url.Parse(strings.ReplaceAll(strings.TrimSpace(text), " ", "%20"))
	segs := strings.Split(strings.Trim(u.EscapedPath(), "/"), "/")
	var tail []string
	if i := slices.IndexFunc(segs, func(s string) bool { return strings.HasPrefix(s, "_") }); i >= 0 {
		tail = segs[i:]
	}
	if len(tail) != 3 || tail[0] != "_queries" || (tail[1] != "query" && tail[1] != "query-edit") || !guidPattern.MatchString(tail[2]) {
		return bad("its path must end in _queries/query/ and the query's ID")
	}
	return project + "/_queries/query/" + tail[2], nil
}

// guidPattern is a GUID, as Azure DevOps names a saved query.
var guidPattern = regexp.MustCompile(`^[0-9A-Fa-f]{8}-[0-9A-Fa-f]{4}-[0-9A-Fa-f]{4}-[0-9A-Fa-f]{4}-[0-9A-Fa-f]{12}$`)

// withoutLogin is text with any login taken out of it, for an error to
// echo: a login may hold a secret.
func withoutLogin(text string) string {
	text = strings.TrimSpace(text)
	u, err := url.Parse(strings.ReplaceAll(text, " ", "%20"))
	if err != nil {
		return ""
	}
	if u.User != nil {
		u.User = nil
		return u.String()
	}
	return text
}

// Names is the organization and project a project URL or query URL names,
// as ParseProjectURL or ParseQueryURL gave it, with the project's name
// unescaped: on Azure DevOps Server the organization is the collection.
// Both are empty for a URL that names no project.
func Names(stored string) (org, project string) {
	u, err := url.Parse(stored)
	if err != nil || u.Host == "" {
		return "", ""
	}
	var segs []string
	for _, s := range strings.Split(strings.Trim(u.EscapedPath(), "/"), "/") {
		if strings.HasPrefix(s, "_") {
			break
		}
		segs = append(segs, s)
	}
	if len(segs) == 0 || segs[0] == "" {
		return "", ""
	}
	name := strings.ToLower(u.Hostname())
	if org, ok := strings.CutSuffix(name, ".visualstudio.com"); ok {
		return org, unescape(segs[len(segs)-1])
	}
	if len(segs) < 2 {
		return "", ""
	}
	return unescape(segs[len(segs)-2]), unescape(segs[len(segs)-1])
}

// unescape is a path segment with its percent-encoding undone, or as it
// is when it can't be.
func unescape(seg string) string {
	if s, err := url.PathUnescape(seg); err == nil {
		return s
	}
	return seg
}

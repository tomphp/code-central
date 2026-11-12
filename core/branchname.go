package core

import (
	"errors"
	"strings"
)

// checkBranchName applies the rules `git check-ref-format --branch` does
// to a branch name, syntax only; an empty name is fine, it means "not
// set".
func checkBranchName(name string) error {
	if name == "" {
		return nil
	}
	bad := func(why string) error { return errors.New("branch name " + quote(name) + " " + why) }
	switch {
	case name == "@":
		return bad("can't be @")
	case strings.HasPrefix(name, "-"):
		return bad("can't start with -")
	case strings.HasPrefix(name, "/") || strings.HasSuffix(name, "/") || strings.Contains(name, "//"):
		return bad("can't start or end with /, or hold //")
	case strings.HasSuffix(name, ".") || strings.Contains(name, ".."):
		return bad("can't end with . or hold ..")
	case strings.Contains(name, "@{"):
		return bad("can't hold @{")
	}
	if strings.ContainsAny(name, " ~^:?*[\\\x7f") || strings.ContainsFunc(name, func(r rune) bool { return r < 0x20 }) {
		return bad("can't hold spaces, control characters or any of ~ ^ : ? * [ \\")
	}
	for _, part := range strings.Split(name, "/") {
		if strings.HasPrefix(part, ".") || strings.HasSuffix(part, ".lock") {
			return bad("can't have a part starting with . or ending in .lock")
		}
	}
	return nil
}

func quote(s string) string { return `"` + s + `"` }

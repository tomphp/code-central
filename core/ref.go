package core

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

var refPattern = regexp.MustCompile(`^(?:(.+)-)?([1-9][0-9]*)$`)

// refNumber resolves r within project key; field names the input for errors.
func refNumber(r Ref, key, field string) (int, error) {
	m := refPattern.FindStringSubmatch(strings.TrimSpace(string(r)))
	if m == nil {
		if strings.TrimSpace(string(r)) == "" {
			return 0, invalid(field, "%s is empty: give an ID as %s-<n> or <n>", field, key)
		}
		return 0, invalid(field, "%q is not an ID: use %s-<n> or <n>", r, key)
	}
	if m[1] != "" && m[1] != key {
		return 0, invalid(field, "%q belongs to Project %q, but this is Project %q", r, m[1], key)
	}
	n, _ := strconv.Atoi(m[2])
	return n, nil
}

// DisplayID is the ID shown everywhere and used as the branch name.
func DisplayID(key string, n int) string { return fmt.Sprintf("%s-%d", key, n) }

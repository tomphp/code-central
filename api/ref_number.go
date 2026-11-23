package api

import (
	"regexp"
	"strconv"
	"strings"
)

var refPattern = regexp.MustCompile(`^(?:(.+)-)?([1-9][0-9]*)$`)

// RefNumber resolves r within project key; field names the input for errors.
func RefNumber(r Ref, key, field string) (int, error) {
	m := refPattern.FindStringSubmatch(strings.TrimSpace(string(r)))
	if m == nil {
		if strings.TrimSpace(string(r)) == "" {
			return 0, InvalidField(field, "%s is empty: give an ID as %s-<n> or <n>", field, key)
		}
		return 0, InvalidField(field, "%q is not an ID: use %s-<n> or <n>", r, key)
	}
	if m[1] != "" && m[1] != key {
		return 0, InvalidField(field, "%q belongs to Project %q, but this is Project %q", r, m[1], key)
	}
	n, _ := strconv.Atoi(m[2])
	return n, nil
}

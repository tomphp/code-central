package core

import (
	"bytes"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// Ref names a Spec or Ticket within a Project: either "<key>-<n>" or a
// bare "<n>". In JSON it may be a string or a number.
type Ref string

// UnmarshalJSON accepts a string or a number.
func (r *Ref) UnmarshalJSON(b []byte) error {
	b = bytes.TrimSpace(b)
	if len(b) > 0 && b[0] != '"' {
		var n json.Number
		if err := json.Unmarshal(b, &n); err != nil {
			return err
		}
		*r = Ref(n.String())
		return nil
	}
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return err
	}
	*r = Ref(s)
	return nil
}

var refPattern = regexp.MustCompile(`^(?:(.+)-)?([1-9][0-9]*)$`)

// number resolves r within project key; field names the input for errors.
func (r Ref) number(key, field string) (int, error) {
	m := refPattern.FindStringSubmatch(strings.TrimSpace(string(r)))
	if m == nil {
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

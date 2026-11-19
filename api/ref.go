package api

import (
	"bytes"
	"encoding/json"
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

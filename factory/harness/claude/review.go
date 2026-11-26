// Review detection: whether a Turn ran the code-review skill.

package claude

import (
	"encoding/json"
	"strings"
)

// calledReview reports whether a tool call is the Skill tool running the
// code-review skill, under any plugin prefix (mattpocock-skills:code-review).
func calledReview(tool string, input json.RawMessage) bool {
	if tool != "Skill" {
		return false
	}
	var in struct {
		Skill string `json:"skill"`
	}
	if json.Unmarshal(input, &in) != nil {
		return false
	}
	name := in.Skill
	if i := strings.LastIndex(name, ":"); i >= 0 {
		name = name[i+1:]
	}
	return name == "code-review"
}

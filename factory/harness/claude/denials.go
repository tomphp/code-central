package claude

import (
	"encoding/json"
	"net/url"

	"github.com/tomoram/code-central/factory/harness"
)

type denialRules struct{}

// Denials is how claude's denied calls become --allowedTools rules.
func (*Harness) Denials() harness.DenialRules { return denialRules{} }

// Rule is the --allowedTools rule that allows the call: a Bash command
// exactly, a file tool on its path, WebFetch on its domain, and any other
// tool by name.
func (denialRules) Rule(d harness.Denial) string {
	var in struct {
		Command  string `json:"command"`
		FilePath string `json:"file_path"`
		URL      string `json:"url"`
	}
	_ = json.Unmarshal(d.Input, &in)
	switch {
	case d.Tool == "Bash" && in.Command != "":
		return "Bash(" + in.Command + ")"
	case in.FilePath != "":
		return d.Tool + "(" + in.FilePath + ")"
	case d.Tool == "WebFetch" && in.URL != "":
		if u, err := url.Parse(in.URL); err == nil && u.Host != "" {
			return "WebFetch(domain:" + u.Hostname() + ")"
		}
	}
	return d.Tool
}

func (denialRules) RuleName() string    { return "--allowedTools rule" }
func (denialRules) RuleExample() string { return "Bash(npm test:*)" }

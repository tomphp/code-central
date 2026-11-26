package claude_test

import (
	"encoding/json"
	"testing"

	"github.com/tomoram/code-central/factory/harness"
	"github.com/tomoram/code-central/factory/harness/claude"
)

func TestADeniedCallBecomesTheAllowedToolsRuleThatAllowsIt(t *testing.T) {
	t.Parallel()
	rules := claude.New(claude.Options{}).Denials()
	for _, c := range []struct{ tool, input, want string }{
		{"Bash", `{"command":"npm install"}`, "Bash(npm install)"},
		{"Edit", `{"file_path":"/a/b.go"}`, "Edit(/a/b.go)"},
		{"WebFetch", `{"url":"https://example.com/x?y=1"}`, "WebFetch(domain:example.com)"},
		{"WebFetch", `{"url":"not a url"}`, "WebFetch"},
		{"Bash", `{}`, "Bash"},
		{"mcp__code-central__finish", `{"ticket":"a-1"}`, "mcp__code-central__finish"},
	} {
		if got := rules.Rule(harness.Denial{Tool: c.tool, Input: json.RawMessage(c.input)}); got != c.want {
			t.Errorf("%s %s: rule %q, want %q", c.tool, c.input, got, c.want)
		}
	}
}

func TestClaudeNamesItsRuleAndGivesAnExample(t *testing.T) {
	t.Parallel()
	rules := claude.New(claude.Options{}).Denials()
	if rules.RuleName() != "--allowedTools rule" || rules.RuleExample() != "Bash(npm test:*)" {
		t.Errorf("rule named %q, example %q", rules.RuleName(), rules.RuleExample())
	}
}

func TestAStageSkillRunsAsASlashCommandWithTheTicketAsItsArguments(t *testing.T) {
	t.Parallel()
	if got := claude.New(claude.Options{}).SkillPrompt("implement", "acme-7"); got != "/implement acme-7" {
		t.Errorf("prompt %q", got)
	}
}

func TestClaudeTellsTheAgentToLoadTheDeferredToolsWithToolSearch(t *testing.T) {
	t.Parallel()
	want := "The code-central tools are deferred: load their schemas with ToolSearch (`select:mcp__code-central__get_ticket,...`) before you call them"
	if got := claude.New(claude.Options{}).ToolLoadingNote(); got != want {
		t.Errorf("note %q", got)
	}
}

package claude

import "fmt"

// SkillPrompt is the skill as a slash command, since claude runs a first
// prompt that starts with one.
func (*Harness) SkillPrompt(skill, args string) string {
	return fmt.Sprintf("/%s %s", skill, args)
}

// ToolLoadingNote tells the agent to load the deferred Code Central tools
// with ToolSearch.
func (*Harness) ToolLoadingNote() string {
	return "The code-central tools are deferred: load their schemas with ToolSearch (`select:mcp__code-central__get_ticket,...`) before you call them"
}

package factory

import (
	"strings"
	"testing"

	"github.com/tomoram/code-central/api"
	"github.com/tomoram/code-central/factory/harness"
)

type noNote struct{ harness.Prompts }

func (noNote) ToolLoadingNote() string { return "" }

func TestTheClaimIntroOnClaudeCodeEndsWithTheToolSearchNote(t *testing.T) {
	c := claim{Ticket: "acme-7", ClaimID: "C7"}
	want := "Your first call is `get_ticket` with `ticket: acme-7`. The code-central tools are deferred: load their schemas with ToolSearch (`select:mcp__code-central__get_ticket,...`) before you call them, and pass `ticket` on the writes that take it."
	if got := claimIntro(claudePrompts, c, api.ImplementationStage, ""); !strings.HasSuffix(got, want) {
		t.Errorf("intro %q does not end with %q", got, want)
	}
}

func TestTheClaimIntroOfAHarnessWithNoToolLoadingNoteJustSaysToPassTheTicket(t *testing.T) {
	c := claim{Ticket: "acme-7", ClaimID: "C7"}
	want := "Your first call is `get_ticket` with `ticket: acme-7`. Pass `ticket` on the writes that take it."
	if got := claimIntro(noNote{claudePrompts}, c, api.ImplementationStage, ""); !strings.HasSuffix(got, want) {
		t.Errorf("intro %q does not end with %q", got, want)
	}
}

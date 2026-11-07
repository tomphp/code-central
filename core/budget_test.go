package core_test

import (
	"regexp"
	"strings"
	"testing"

	"github.com/tomoram/code-central/core"
	"github.com/tomoram/code-central/mcp"
)

// The context budget for teaching agents. These are limits, not targets:
// text an agent pays for in every session must stay small.

func TestTheMCPInstructionsStayWithinAbout150Words(t *testing.T) {
	if n := len(strings.Fields(core.Instructions)); n > 150 {
		t.Fatalf("MCP instructions are %d words, over the 150 budget", n)
	}
}

func TestEveryToolDescriptionIsOneLine(t *testing.T) {
	for name, d := range mcp.Descriptions() {
		if strings.TrimSpace(d) == "" || strings.ContainsAny(d, "\r\n") {
			t.Errorf("tool %s: description %q is not one line", name, d)
		}
	}
}

func TestTheWorkflowProseStaysWithinAbout1500Tokens(t *testing.T) {
	if n := tokens(workflow(t).Prose); n > 1500 {
		t.Fatalf("get_workflow prose is about %d tokens, over the 1,500 budget", n)
	}
}

func TestEveryNextStepIsOneSentence(t *testing.T) {
	steps := core.NextSteps()
	if len(steps) == 0 {
		t.Fatal("no action has a next step")
	}
	for name, s := range steps {
		if !oneSentence(s) {
			t.Errorf("%s: next step %q is not one sentence", name, s)
		}
	}
}

func TestTheBudgetChecksCatchOverlongText(t *testing.T) {
	if oneSentence("End your turn now. Then run code-central wait.") {
		t.Error("two sentences passed as one")
	}
	if !oneSentence("End your turn now, after running `code-central wait acme-2` in the background if you can.") {
		t.Error("one sentence with a command in it was rejected")
	}
	if tokens(strings.Repeat("word ", 1200)) <= 1500 {
		t.Error("1,200 words should be over 1,500 tokens")
	}
}

// tokens estimates a text's token count the usual rough ways (four
// characters a token, or three words to four tokens) and takes the larger.
func tokens(s string) int {
	return max(len(s)/4, len(strings.Fields(s))*4/3)
}

// sentenceBreak is the end of one sentence followed by the start of
// another.
var sentenceBreak = regexp.MustCompile(`[.!?]\s+\S`)

func oneSentence(s string) bool {
	s = strings.TrimSpace(s)
	return s != "" && !strings.ContainsAny(s, "\r\n") && !sentenceBreak.MatchString(s)
}

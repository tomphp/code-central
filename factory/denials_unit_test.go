package factory

import (
	"encoding/json"
	"testing"

	"github.com/tomoram/code-central/factory/harness"
	"github.com/tomoram/code-central/factory/harness/claude"
)

type noDenialList struct{ *claude.Harness }

func (noDenialList) Denials() harness.DenialRules { return nil }

var oneDenial = harness.Outcome{Denials: []harness.Denial{{Tool: "Bash", Input: json.RawMessage(`{"command":"npm install"}`)}}}

func TestAHarnessWithNoDenialListHasNoDenialsToAskAbout(t *testing.T) {
	rules, ds := denialsOf(noDenialList{claude.New(claude.Options{})}, oneDenial)
	if rules != nil || len(ds) != 0 {
		t.Errorf("denials %v with rules %v, want none", ds, rules)
	}
}

func TestAHarnessWithADenialListHasItsTurnsDenialsToAskAbout(t *testing.T) {
	rules, ds := denialsOf(claude.New(claude.Options{}), oneDenial)
	if rules == nil || len(ds) != 1 {
		t.Fatalf("denials %v with rules %v, want the one call", ds, rules)
	}
	if got := rules.Rule(ds[0]); got != "Bash(npm install)" {
		t.Errorf("rule %q, want Bash(npm install)", got)
	}
}

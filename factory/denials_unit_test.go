package factory

import (
	"encoding/json"
	"io"
	"testing"

	"github.com/tomoram/code-central/factory/harness"
	"github.com/tomoram/code-central/factory/harness/claude"
)

type noDenialList struct{ *claude.Harness }

func (noDenialList) Denials() harness.DenialRules { return nil }

var oneDenial = harness.Outcome{Denials: []harness.Denial{{Tool: "Bash", Input: json.RawMessage(`{"command":"npm install"}`)}}}

func factoryOf(h harness.Harness) *factory {
	f := newFactory(Env{Stdout: io.Discard}, options{})
	f.runner = newTurnRunner(h, Env{}, false)
	return f
}

func TestAHarnessWithNoDenialListHasNoDenialsToAskAbout(t *testing.T) {
	f := factoryOf(noDenialList{claude.New(claude.Options{})})
	if rules, ds := f.denialsOf(oneDenial); rules != nil || len(ds) != 0 {
		t.Errorf("denials %v with rules %v, want none", ds, rules)
	}
}

func TestAHarnessWithADenialListHasItsTurnsDenialsToAskAbout(t *testing.T) {
	f := factoryOf(claude.New(claude.Options{}))
	rules, ds := f.denialsOf(oneDenial)
	if rules == nil || len(ds) != 1 {
		t.Fatalf("denials %v with rules %v, want the one call", ds, rules)
	}
	if got := rules.Rule(ds[0]); got != "Bash(npm install)" {
		t.Errorf("rule %q, want Bash(npm install)", got)
	}
}

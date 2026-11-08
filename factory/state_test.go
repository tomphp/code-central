package factory_test

import (
	"encoding/json"
	"fmt"
	"os"
	"testing"

	"github.com/tomoram/code-central/factory/factorytest"
	"github.com/tomoram/code-central/store"
)

func TestASecondFactoryOnTheSameProjectRefusesToStartNamingTheFirstsPid(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t)
	first := h.Start()
	first.WaitForOutput("base: main")

	refuses(t, h, h.Repo, fmt.Sprintf("a Factory is already running for acme (pid %d)", os.Getpid()))

	first.Stop()
	second := h.Start()
	second.WaitForOutput("base: main")
}

func TestTheStateFileHoldsEachClaimAfterItIsMade(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t)
	id := h.ReadyTicket("Work")
	h.Start()
	h.WaitForState(id, store.InReview)
	c := h.Calls()[0]

	b, err := os.ReadFile(h.StateFile())
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Claims []struct {
			Ticket       string   `json:"ticket"`
			ClaimID      string   `json:"claim_id"`
			SessionID    string   `json:"session_id"`
			AllowedTools []string `json:"allowed_tools"`
		} `json:"claims"`
	}
	if err := json.Unmarshal(b, &doc); err != nil {
		t.Fatalf("%v: %s", err, b)
	}
	if len(doc.Claims) != 1 {
		t.Fatalf("state file %s: want one Claim", b)
	}
	got := doc.Claims[0]
	if got.Ticket != id || got.ClaimID != c.Claim || got.ClaimID == "" || got.SessionID != c.Session || got.AllowedTools == nil || len(got.AllowedTools) != 0 {
		t.Errorf("state file %s; want %s under Claim %s and session %s, with no allowed tools", b, id, c.Claim, c.Session)
	}
}

func TestWithNoHOMETheStateFileIsUnderTheProfileFolder(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t)
	h.Setenv("HOME", "") // as on Windows: Home is then only the profile folder
	id := h.ReadyTicket("Work")
	h.Start()
	h.WaitForState(id, store.InReview)
	if _, err := os.Stat(h.StateFile()); err != nil {
		t.Fatalf("no state file under the profile folder: %v", err)
	}
}

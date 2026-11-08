package core_test

import (
	"testing"

	"github.com/tomoram/code-central/core"
	"github.com/tomoram/code-central/store"
)

// Resume Command and working directory on a Claim (ticket 69).

func str(s string) *string { return &s }

func wantResume(t *testing.T, c core.ClaimView, dir, command string) {
	t.Helper()
	if c.WorkingDir != dir || c.ResumeCommand != command {
		t.Fatalf("claim working_dir %q, resume_command %q; want %q, %q", c.WorkingDir, c.ResumeCommand, dir, command)
	}
}

func TestClaimRecordsTheWorkingDirAndResumeCommandAsGiven(t *testing.T) {
	f := newFixture(t)
	f.work(t, store.Ready)
	res := f.claim(t, core.ClaimInput{SessionID: "s1", WorkingDir: str("/work/acme-2"), ResumeCommand: str("codex resume s1 --yolo")})
	wantResume(t, res.Ticket.Claims[0], "/work/acme-2", "codex resume s1 --yolo")
}

func TestAClaimWithoutThemReadsBothEmptyNeverADefault(t *testing.T) {
	f := newFixture(t)
	f.work(t, store.Ready)
	res := f.claim(t, core.ClaimInput{SessionID: "s1"})
	wantResume(t, res.Ticket.Claims[0], "", "")
}

func TestSetSessionsLastWriteWinsAndAMissingFieldLeavesTheStoredOneAlone(t *testing.T) {
	f := newFixture(t)
	f.work(t, store.Ready)
	f.claim(t, core.ClaimInput{SessionID: "s1", WorkingDir: str("/a"), ResumeCommand: str("claude --resume s1")})

	v, err := f.core.SetSession(ctx, "acme", core.SetSessionInput{Ticket: "2", ClaimID: "claim-1", SessionID: "s2", ResumeCommand: str("claude --resume s2")})
	if err != nil {
		t.Fatal(err)
	}
	wantResume(t, v.Claims[0], "/a", "claude --resume s2")

	v, err = f.core.SetSession(ctx, "acme", core.SetSessionInput{Ticket: "2", ClaimID: "claim-1", SessionID: "s2", WorkingDir: str("/b")})
	if err != nil {
		t.Fatal(err)
	}
	wantResume(t, v.Claims[0], "/b", "claude --resume s2")

	// Given empty is a write too: it clears the value.
	v, err = f.core.SetSession(ctx, "acme", core.SetSessionInput{Ticket: "2", ClaimID: "claim-1", SessionID: "s2", ResumeCommand: str("")})
	if err != nil {
		t.Fatal(err)
	}
	wantResume(t, v.Claims[0], "/b", "")
}

func TestANewClaimStartsWithNoWorkingDirOrResumeCommand(t *testing.T) {
	f := newFixture(t)
	f.work(t, store.Ready)
	f.claim(t, core.ClaimInput{WorkingDir: str("/a"), ResumeCommand: str("run it")})
	if _, err := f.core.GiveUp(ctx, "acme", core.GiveUpInput{Ticket: "2", ClaimID: "claim-1", Note: "stuck"}); err != nil {
		t.Fatal(err)
	}
	res := f.claim(t, core.ClaimInput{Ticket: "2"})
	wantResume(t, res.Ticket.Claims[0], "/a", "run it")
	wantResume(t, res.Ticket.Claims[1], "", "")
}

func TestAFailureReportCopiesTheSessionWorkingDirAndResumeCommand(t *testing.T) {
	f := newFixture(t)
	f.work(t, store.Ready)
	f.claim(t, core.ClaimInput{SessionID: "s1", WorkingDir: str("/a"), ResumeCommand: str("claude --resume s1")})
	res, err := f.core.ReportFailure(ctx, "acme", report("2", "claim-1"))
	if err != nil {
		t.Fatal(err)
	}
	r := res.Ticket.FailureReports[0]
	if r.SessionID != "s1" || r.WorkingDir != "/a" || r.ResumeCommand != "claude --resume s1" {
		t.Fatalf("failure report %+v", r)
	}
}

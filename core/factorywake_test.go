package core_test

import (
	"context"
	"testing"
	"time"

	"github.com/tomoram/code-central/core"
	"github.com/tomoram/code-central/domain"
)

// wakeAsync opens Factory factory's next_factory_wake call on Project key
// from host box, and waits until it holds.
func (f fixture) wakeAsync(key, factory string) chan core.FactoryWake {
	out := make(chan core.FactoryWake, 1)
	n := f.clock.Waiters()
	go func() {
		w, err := f.core.NextFactoryWake(ctx, key, core.NextFactoryWakeInput{Factory: factory, Host: "box"})
		if err != nil {
			panic(err)
		}
		out <- w
	}()
	f.clock.BlockUntil(n + 1)
	return out
}

// ended is the call's result once WaitTimeout has passed, so a call no
// write woke ends empty instead of holding the test.
func (f fixture) ended(call chan core.FactoryWake) core.FactoryWake {
	f.clock.Advance(core.WaitTimeout)
	return <-call
}

func (f fixture) approve(t *testing.T, ref core.Ref) {
	t.Helper()
	if _, err := f.core.Approve(human, "acme", core.TicketInput{Ticket: ref}); err != nil {
		t.Fatalf("approve %s: %v", ref, err)
	}
}

// Every move into Ready, Answered, Accepted, Done or Cancelled wakes the
// Project's Factories: the Ticket may be claimable, to resume, to land,
// or the last Blocker of another.
func TestEveryMoveTheFactorysPollActsOnWakesIt(t *testing.T) {
	for _, tc := range []struct {
		name  string
		setup func(t *testing.T, f fixture)
		move  func(f fixture) error
	}{
		{"create a Ready Ticket", func(t *testing.T, f fixture) { f.work(t) }, func(f fixture) error {
			_, err := f.core.CreateTicket(ctx, "acme", core.CreateTicketInput{Spec: "1", Title: "Ta", State: domain.Ready})
			return err
		}},
		{"approve", func(t *testing.T, f fixture) { f.work(t, domain.Draft) }, func(f fixture) error {
			_, err := f.core.Approve(human, "acme", core.TicketInput{Ticket: "2"})
			return err
		}},
		{"send back", func(t *testing.T, f fixture) { f.failed(t) }, func(f fixture) error {
			_, err := f.core.SendBack(human, "acme", core.SendBackInput{Ticket: "2", Note: "Run migration 12 first."})
			return err
		}},
		{"reject", func(t *testing.T, f fixture) { f.inReview(t) }, func(f fixture) error {
			_, err := f.core.Reject(human, "acme", core.RejectInput{Ticket: "2", Comment: "Name the button."})
			return err
		}},
		{"release", func(t *testing.T, f fixture) {
			f.work(t, domain.Ready)
			f.claim(t, core.ClaimInput{})
		}, func(f fixture) error {
			_, err := f.core.Release(human, "acme", core.ReleaseInput{Ticket: "2"})
			return err
		}},
		{"give up", func(t *testing.T, f fixture) {
			f.work(t, domain.Ready)
			f.claim(t, core.ClaimInput{})
		}, func(f fixture) error {
			_, err := f.core.GiveUp(ctx, "acme", core.GiveUpInput{Ticket: "2", ClaimID: "claim-1", Note: "Out of ideas."})
			return err
		}},
		{"a Round's last answer", func(t *testing.T, f fixture) { f.asking(t, qDB) }, func(f fixture) error {
			_, err := f.core.Answer(human, "acme", core.AnswerInput{Ticket: "2", Question: 1, Text: "SQLite"})
			return err
		}},
		{"accept", func(t *testing.T, f fixture) { f.inReviewOnBranch(t) }, func(f fixture) error {
			_, err := f.core.Accept(human, "acme", core.TicketInput{Ticket: "2"})
			return err
		}},
		{"land", func(t *testing.T, f fixture) { f.accepted(t) }, func(f fixture) error {
			_, err := f.core.Land(ctx, "acme", core.LandInput{Ticket: "2", Commit: "def456"})
			return err
		}},
		{"implement", func(t *testing.T, f fixture) { f.specInReview(t) }, func(f fixture) error {
			_, err := f.core.Implement(human, "acme", core.TicketInput{Ticket: "4"})
			return err
		}},
		{"cancel", func(t *testing.T, f fixture) { f.work(t, domain.Draft) }, func(f fixture) error {
			_, err := f.core.Cancel(human, "acme", core.CancelInput{Ticket: "2"})
			return err
		}},
		{"a move by hand to Done", func(t *testing.T, f fixture) { f.inReview(t) }, func(f fixture) error {
			_, err := f.core.Move(human, "acme", core.MoveInput{Ticket: "2", ToState: domain.Done})
			return err
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newFixture(t)
			tc.setup(t, f)
			woken := f.wakeAsync("acme", "cc")
			if err := tc.move(f); err != nil {
				t.Fatal(err)
			}
			if w := f.ended(woken); !w.Woken {
				t.Fatalf("next_factory_wake = %+v, want woken", w)
			}
		})
	}
}

// Running agents write all the time; none of these is a change the
// Factory's poll acts on, so the call holds until WaitTimeout and ends
// empty.
func TestWritesTheFactorysPollDoesNotActOnLeaveItAsleepUntilTheTimeout(t *testing.T) {
	f := newFixture(t)
	f.work(t, domain.Ready, domain.Ready)
	woken := f.wakeAsync("acme", "cc")
	writes := []func() error{
		func() error {
			_, err := f.core.Claim(ctx, "acme", core.ClaimInput{SessionID: "s1"})
			return err
		},
		func() error {
			_, err := f.core.SetSession(ctx, "acme", core.SetSessionInput{Ticket: "2", ClaimID: "claim-1", SessionID: "s1"})
			return err
		},
		func() error {
			f.core.PostSessionEvents(ctx, "s1", []core.SessionEvent{{Time: f.clock.Now(), Kind: "tool_call", Text: "Bash: go test ./..."}})
			return nil
		},
		func() error {
			_, err := f.core.Comment(ctx, "acme", core.CommentInput{Ticket: "2", Body: "Halfway."})
			return err
		},
		func() error {
			_, err := f.core.Finish(ctx, "acme", core.FinishInput{Ticket: "2", ClaimID: "claim-1", Summary: "Done on acme-2."})
			return err
		},
		func() error {
			_, err := f.core.Claim(ctx, "acme", core.ClaimInput{SessionID: "s2"})
			return err
		},
		func() error {
			_, err := f.core.PostRound(ctx, "acme", core.PostRoundInput{Ticket: "3", ClaimID: "claim-2", Questions: []core.QuestionInput{qDB}})
			return err
		},
	}
	for i, write := range writes {
		if err := write(); err != nil {
			t.Fatalf("write %d: %v", i, err)
		}
	}
	f.clock.Advance(core.WaitTimeout - time.Second)
	select {
	case w := <-woken:
		t.Fatalf("returned before the timeout: %+v", w)
	default:
	}
	f.clock.Advance(time.Second)
	if w := <-woken; w.Woken {
		t.Fatalf("next_factory_wake = %+v, want empty", w)
	}
}

func TestAChangeInAnotherProjectDoesNotWakeTheFactory(t *testing.T) {
	f := newFixture(t)
	f.work(t, domain.Draft)
	f.project(t, "other")
	woken := f.wakeAsync("other", "cc")
	f.approve(t, "2")
	if w := f.ended(woken); w.Woken {
		t.Fatalf("other's next_factory_wake = %+v, want empty", w)
	}
}

func TestEveryFactoryOnTheProjectIsWoken(t *testing.T) {
	f := newFixture(t)
	f.work(t, domain.Draft)
	first, second := f.wakeAsync("acme", "cc"), f.wakeAsync("acme", "cc-2")
	f.approve(t, "2")
	f.clock.Advance(core.WaitTimeout)
	for _, woken := range []chan core.FactoryWake{first, second} {
		if w := <-woken; !w.Woken {
			t.Fatalf("next_factory_wake = %+v, want woken", w)
		}
	}
}

func TestANewerCallFromTheSameFactoryEndsTheOlderOneEmpty(t *testing.T) {
	f := newFixture(t)
	f.work(t, domain.Draft)
	older := f.wakeAsync("acme", "cc")
	newer := make(chan core.FactoryWake, 1)
	go func() {
		w, _ := f.core.NextFactoryWake(ctx, "acme", core.NextFactoryWakeInput{Factory: "cc", Host: "box"})
		newer <- w
	}()
	if w := <-older; w.Woken {
		t.Fatalf("older next_factory_wake = %+v, want empty", w)
	}
	f.clock.BlockUntil(2) // the newer call's timeout, beside the older's
	f.approve(t, "2")
	if w := f.ended(newer); !w.Woken {
		t.Fatalf("newer next_factory_wake = %+v, want woken", w)
	}
}

func TestNextFactoryWakeEndsEmptyWhenItsRequestIsCancelled(t *testing.T) {
	f := newFixture(t)
	f.project(t, "acme")
	cctx, cancel := context.WithCancel(ctx)
	done := make(chan core.FactoryWake, 1)
	go func() {
		w, _ := f.core.NextFactoryWake(cctx, "acme", core.NextFactoryWakeInput{Factory: "cc", Host: "box"})
		done <- w
	}()
	f.clock.BlockUntil(1)
	cancel()
	if w := <-done; w.Woken {
		t.Fatalf("next_factory_wake = %+v, want empty", w)
	}
}

// Like next_chat_work, it is a read: an archived Project still answers.
func TestNextFactoryWakeHoldsOnAnArchivedProject(t *testing.T) {
	f := newFixture(t)
	f.project(t, "acme")
	f.archive(t, "acme")
	if w := f.ended(f.wakeAsync("acme", "cc")); w.Woken {
		t.Fatalf("next_factory_wake = %+v, want empty", w)
	}
}

func TestNextFactoryWakeRejections(t *testing.T) {
	f := newFixture(t)
	f.project(t, "acme")
	_, err := f.core.NextFactoryWake(ctx, "acme", core.NextFactoryWakeInput{Host: "box"})
	wantCode(t, err, core.Invalid, "factory")
	_, err = f.core.NextFactoryWake(ctx, "acme", core.NextFactoryWakeInput{Factory: "cc", Host: " "})
	wantCode(t, err, core.Invalid, "host")
	_, err = f.core.NextFactoryWake(ctx, "nope", core.NextFactoryWakeInput{Factory: "cc", Host: "box"})
	wantCode(t, err, core.UnknownProject, "")
}

// WaitingFactoryWakes counts the calls a wake would end now: open, on the
// Project, and not yet woken.
func TestWaitingFactoryWakesCountsTheOpenCallsNotYetWoken(t *testing.T) {
	f := newFixture(t)
	f.work(t, domain.Draft)
	if n := f.core.WaitingFactoryWakes("acme"); n != 0 {
		t.Fatalf("with no call open, WaitingFactoryWakes = %d, want 0", n)
	}
	woken := f.wakeAsync("acme", "cc")
	if n := f.core.WaitingFactoryWakes("acme"); n != 1 {
		t.Fatalf("with one call open, WaitingFactoryWakes = %d, want 1", n)
	}
	if n := f.core.WaitingFactoryWakes("other"); n != 0 {
		t.Fatalf("on another Project, WaitingFactoryWakes = %d, want 0", n)
	}
	f.approve(t, "2")
	if w := f.ended(woken); !w.Woken {
		t.Fatalf("next_factory_wake = %+v, want woken", w)
	}
	if n := f.core.WaitingFactoryWakes("acme"); n != 0 {
		t.Fatalf("once the call is woken, WaitingFactoryWakes = %d, want 0", n)
	}
}

// wakes says whether write, made with a next_factory_wake call open on
// acme, woke it.
func (f fixture) wakes(t *testing.T, write func(f fixture) error) bool {
	t.Helper()
	woken := f.wakeAsync("acme", "cc")
	if err := write(f); err != nil {
		t.Fatal(err)
	}
	return f.ended(woken).Woken
}

// A live Claim ended by a move its holder didn't make frees a slot the
// Factory may be counting on, and stops a turn it should know about.
func TestALiveClaimEndedByAMoveItsHolderDidNotMakeWakesTheFactory(t *testing.T) {
	claimed := func(t *testing.T, f fixture) {
		f.work(t, domain.Ready)
		f.claim(t, core.ClaimInput{})
	}
	for _, tc := range []struct {
		name  string
		setup func(t *testing.T, f fixture)
		move  func(f fixture) error
	}{
		{"a move by hand to In Review", claimed, func(f fixture) error {
			_, err := f.core.Move(human, "acme", core.MoveInput{Ticket: "2", ToState: domain.InReview})
			return err
		}},
		{"a move by hand to Draft", claimed, func(f fixture) error {
			_, err := f.core.Move(human, "acme", core.MoveInput{Ticket: "2", ToState: domain.Draft})
			return err
		}},
		{"cancel", claimed, func(f fixture) error {
			_, err := f.core.Cancel(human, "acme", core.CancelInput{Ticket: "2"})
			return err
		}},
		{"release while Answered", func(t *testing.T, f fixture) {
			f.asking(t, qDB)
			if _, err := f.core.Answer(human, "acme", core.AnswerInput{Ticket: "2", Question: 1, Text: "SQLite"}); err != nil {
				t.Fatal(err)
			}
		}, func(f fixture) error {
			_, err := f.core.Release(human, "acme", core.ReleaseInput{Ticket: "2"})
			return err
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newFixture(t)
			tc.setup(t, f)
			if !f.wakes(t, tc.move) {
				t.Fatal("next_factory_wake ended empty, want woken")
			}
		})
	}
}

// The holder's own report_failure ends its Claim into Failed, which the
// Factory's poll doesn't act on.
func TestTheHoldersOwnReportFailureDoesNotWakeTheFactory(t *testing.T) {
	f := newFixture(t)
	f.work(t, domain.Ready)
	f.claim(t, core.ClaimInput{})
	if f.wakes(t, func(f fixture) error {
		_, err := f.core.ReportFailure(ctx, "acme", core.ReportFailureInput{Ticket: "2", ClaimID: "claim-1",
			WhatBroke: "The build.", Why: "A missing tool.", WhatToFix: "Install it.", WhereWorkStands: "Nothing done."})
		return err
	}) {
		t.Fatal("next_factory_wake woken, want empty")
	}
}

func TestRemovingABlockerWakesTheFactory(t *testing.T) {
	f := newFixture(t)
	f.work(t, domain.Ready, domain.Ready)
	if _, err := f.core.AddBlocker(ctx, "acme", core.BlockerInput{Ticket: "3", Blocker: "2"}); err != nil {
		t.Fatal(err)
	}
	if !f.wakes(t, func(f fixture) error {
		_, err := f.core.RemoveBlocker(ctx, "acme", core.BlockerInput{Ticket: "3", Blocker: "2"})
		return err
	}) {
		t.Fatal("next_factory_wake ended empty, want woken")
	}
}

func TestAddingABlockerDoesNotWakeTheFactory(t *testing.T) {
	f := newFixture(t)
	f.work(t, domain.Ready, domain.Ready)
	if f.wakes(t, func(f fixture) error {
		_, err := f.core.AddBlocker(ctx, "acme", core.BlockerInput{Ticket: "3", Blocker: "2"})
		return err
	}) {
		t.Fatal("next_factory_wake woken, want empty")
	}
}

// Each Project setting the Factory reads wakes it when it changes.
func TestChangingAProjectSettingTheFactoryReadsWakesIt(t *testing.T) {
	base := "trunk"
	for name, change := range map[string]func(f fixture) error{
		"the base": func(f fixture) error {
			_, err := f.core.SetLandingMethod(human, "acme", core.SetLandingMethodInput{Method: domain.LandingMerge, Base: &base})
			return err
		},
		"Landing": func(f fixture) error {
			_, err := f.core.SetLandingMethod(human, "acme", core.SetLandingMethodInput{Method: domain.LandingBranch})
			return err
		},
		"CI": func(f fixture) error {
			_, err := f.core.SetCITracking(human, "acme", core.SetCITrackingInput{Tracking: true, Repository: "acme/web"})
			return err
		},
		"Models": func(f fixture) error {
			_, err := f.core.SetModels(human, "acme", core.SetModelsInput{Default: core.ModelRowView{Model: "opus"}})
			return err
		},
		"Stage Guides' subagent exploration": func(f fixture) error {
			_, err := f.core.SetSubagentExploration(human, "acme", core.SetSubagentExplorationInput{On: false})
			return err
		},
		"Stage Guides' Model Assignment": func(f fixture) error {
			_, err := f.core.SetModelAssignment(human, "acme", core.SetModelAssignmentInput{On: true})
			return err
		},
		"Auto-accept": func(f fixture) error {
			_, err := f.core.SetAutoAccept(human, "acme", core.SetAutoAcceptInput{Kinds: map[domain.AutoAcceptKind]bool{domain.AutoAcceptImplementation: true}})
			return err
		},
		"Archived": func(f fixture) error {
			_, err := f.core.ArchiveProject(human, "acme")
			return err
		},
	} {
		t.Run(name, func(t *testing.T) {
			f := newFixture(t)
			f.project(t, "acme")
			if !f.wakes(t, change) {
				t.Fatal("next_factory_wake ended empty, want woken")
			}
		})
	}
}

func TestUnarchivingAProjectWakesTheFactory(t *testing.T) {
	f := newFixture(t)
	f.project(t, "acme")
	f.archive(t, "acme")
	if !f.wakes(t, func(f fixture) error {
		_, err := f.core.UnarchiveProject(human, "acme")
		return err
	}) {
		t.Fatal("next_factory_wake ended empty, want woken")
	}
}

func TestAnImportWakesTheFactory(t *testing.T) {
	f := newFixture(t)
	f.project(t, "acme")
	if !f.wakes(t, func(f fixture) error {
		_, err := f.core.Import(human, "acme", core.ImportInput{Payload: core.ImportPayload{
			Specs:   []core.ImportSpec{{Source: ".scratch/login/", Title: "Login"}},
			Tickets: []core.ImportTicket{{Source: ".scratch/login/issues/01-form.md", Spec: ".scratch/login/", Title: "Form", State: domain.Draft}},
		}})
		return err
	}) {
		t.Fatal("next_factory_wake ended empty, want woken")
	}
}

// guidedMap is a Map at acme-1 with its guided Tag set to guided.
func (f fixture) guidedMap(t *testing.T, guided string) {
	t.Helper()
	f.project(t, "acme")
	if _, err := f.core.SubmitMap(ctx, "acme", core.SubmitMapInput{Text: "Offline mode"}); err != nil {
		t.Fatal(err)
	}
	f.setGuided(t, guided)
}

func (f fixture) setGuided(t *testing.T, guided string) {
	t.Helper()
	if _, err := f.core.SetTags(ctx, "acme", core.SetTagsInput{Target: "1", Tags: map[string]string{core.GuidedTag: guided}}); err != nil {
		t.Fatal(err)
	}
}

func TestTurningAMapsGuidedTagOnWakesTheFactory(t *testing.T) {
	f := newFixture(t)
	f.guidedMap(t, "off")
	if !f.wakes(t, func(f fixture) error {
		_, err := f.core.SetTags(ctx, "acme", core.SetTagsInput{Target: "1", Tags: map[string]string{core.GuidedTag: core.GuidedOn}})
		return err
	}) {
		t.Fatal("next_factory_wake ended empty, want woken")
	}
}

func TestTurningAMapsGuidedTagOffOrRetaggingAGuidedMapDoesNotWakeTheFactory(t *testing.T) {
	for name, tags := range map[string]map[string]string{
		"off":     {core.GuidedTag: "off"},
		"another": {core.GuidedTag: core.GuidedOn, "type": "map"},
	} {
		t.Run(name, func(t *testing.T) {
			f := newFixture(t)
			f.guidedMap(t, core.GuidedOn)
			if f.wakes(t, func(f fixture) error {
				_, err := f.core.SetTags(ctx, "acme", core.SetTagsInput{Target: "1", Tags: tags})
				return err
			}) {
				t.Fatal("next_factory_wake woken, want empty")
			}
		})
	}
}

// The Factory's own reports, UI-only settings, Usage, CI Runs, Chat and
// Options aren't changes its poll acts on.
func TestProjectWritesTheFactorysPollDoesNotActOnLeaveItAsleep(t *testing.T) {
	for name, write := range map[string]func(f fixture) error{
		"report_models": func(f fixture) error {
			_, err := f.core.ReportModels(ctx, "acme", core.ReportModelsInput{EffortLevels: []string{"low"}, Aliases: []string{"opus"}})
			return err
		},
		"report_push_remote": func(f fixture) error {
			_, err := f.core.ReportPushRemote(ctx, "acme", core.ReportPushRemoteInput{URL: "git@github.com:acme/web.git"})
			return err
		},
		"Focus Kinds": func(f fixture) error {
			_, err := f.core.SetFocusKinds(human, "acme", core.SetFocusKindsInput{Off: []core.NeedsMeReason{core.ReasonAnswered}})
			return err
		},
		"Usage": func(f fixture) error {
			f.core.PostUsage(ctx, "s1", []core.UsageResponse{{Kind: "implementation", Model: "claude-opus-5-5"}})
			return nil
		},
		"CI Runs": func(f fixture) error {
			_, err := f.core.ReportCIRuns(ctx, "acme", core.ReportCIRunsInput{Runs: []core.CIRunView{{ID: 1, Attempt: 1, Workflow: "ci.yml", Commit: "abc123", Status: domain.CIQueued, Ticket: "acme-2"}}})
			return err
		},
		"Chat": func(f fixture) error {
			_, err := f.core.SendChatMessage(human, "acme", core.SendChatMessageInput{Text: "What is acme?"})
			return err
		},
		"Options": func(f fixture) error {
			focus := domain.FocusActivityDashboard
			_, err := f.core.SetOptions(human, core.SetOptionsInput{FocusClear: &focus})
			return err
		},
	} {
		t.Run(name, func(t *testing.T) {
			f := newFixture(t)
			f.work(t, domain.Ready)
			f.claim(t, core.ClaimInput{SessionID: "s1"})
			if _, err := f.core.SetCITracking(human, "acme", core.SetCITrackingInput{Tracking: true, Repository: "acme/web"}); err != nil {
				t.Fatal(err)
			}
			if _, err := f.core.ReportHeartbeat(ctx, "acme", core.ReportHeartbeatInput{Factory: "cc", Host: "box", State: core.HeartbeatOnline, IntervalSeconds: 10, Chat: true}); err != nil {
				t.Fatal(err)
			}
			if f.wakes(t, write) {
				t.Fatal("next_factory_wake woken, want empty")
			}
		})
	}
}

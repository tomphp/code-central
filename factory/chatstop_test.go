package factory_test

import (
	"slices"
	"testing"

	"github.com/tomoram/code-central/core"
	"github.com/tomoram/code-central/factory/factorytest"
)

// startLongReply starts the Factory and sends a Chat Message whose reply
// waits, once the fake claude is running it.
func startLongReply(t *testing.T, h *factorytest.Harness, args ...string) *factorytest.Running {
	t.Helper()
	h.Script(factorytest.Chat, factorytest.Say("Working"), factorytest.Wait(), factorytest.Say("All done."), factorytest.Result("success"))
	r := h.Start(args...)
	h.SendChat("Long question")
	h.WaitFor("the Chat reply to start", func() bool { return len(chatCalls(h)) == 1 })
	return r
}

func TestAStopEndsTheChatTurnAndTheNextMessageResumesTheSession(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t)
	startLongReply(t, h)

	if _, err := h.Human().StopChatReply(t.Context(), h.Project); err != nil {
		t.Fatal(err)
	}
	cv := h.WaitForReply()
	if r := reply(cv); r.Outcome != core.ChatStopped {
		t.Errorf("reply ended %q (%q), want stopped", r.Outcome, r.Error)
	}
	// Stopped as a Claim's turn is.
	if got, want := h.Signals(factorytest.Chat), stopSignals(); !slices.Equal(got, want) {
		t.Errorf("the reply's process got %q, want %q", got, want)
	}

	h.Script(factorytest.Chat, factorytest.Say("Back again."), factorytest.Result("success"))
	h.SendChat("Go on")
	cv2 := h.WaitForReply()
	calls := chatCalls(h)
	if len(calls) != 2 {
		t.Fatalf("Chat calls %+v, want two", calls)
	}
	if s, _ := calls[1].Flag("--resume"); s != cv.SessionID || cv.SessionID == "" {
		t.Errorf("args %q, want a resume of session %q", calls[1].Args, cv.SessionID)
	}
	if r := reply(cv2); r.Outcome != core.ChatDone {
		t.Errorf("second reply ended %q, want done", r.Outcome)
	}
}

func TestTheFirstCtrlCDrainsLettingTheChatReplyFinishAndTheConversationIsUnavailable(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t)
	r := startLongReply(t, h)

	r.Interrupt()
	r.WaitForOutput("draining")
	h.WaitFor("the Factory to report Draining", func() bool {
		return factories(t, h).Status == core.FactoryDraining
	})

	h.Continue(factorytest.Chat)
	if code := r.Exited(); code != 0 {
		t.Errorf("exit code %d, want 0", code)
	}
	cv := h.Chat()
	if m := reply(cv); m.Outcome != core.ChatDone {
		t.Errorf("reply ended %q (%q), want it left to finish: done", m.Outcome, m.Error)
	}
	if got := h.Signals(factorytest.Chat); len(got) != 0 {
		t.Errorf("the draining reply got %q, want no signals", got)
	}
	if cv.Availability != core.ChatUnavailable {
		t.Errorf("Conversation %+v, want it unavailable once the Factory has drained", cv)
	}
}

func TestStoppingStopsTheChatReplyPostsStoppedAndExits(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t)
	r := startLongReply(t, h)

	r.Interrupt()
	r.WaitForOutput("draining")
	r.Interrupt()
	if code := r.Exited(); code != 0 {
		t.Errorf("exit code %d, want 0", code)
	}
	if got, want := h.Signals(factorytest.Chat), stopSignals(); !slices.Equal(got, want) {
		t.Errorf("the reply's process got %q, want %q", got, want)
	}
	if m := reply(h.Chat()); m.Outcome != core.ChatStopped {
		t.Errorf("reply ended %q (%q), want stopped", m.Outcome, m.Error)
	}
}

func TestNoChatNeverPollsForChatWork(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t)
	// Every poll for Chat work would fail and be counted.
	h.SetFailing("/chat/next-work")
	h.Start("--no-chat")
	waitForFactory(t, h, core.FactoryOnline, "turns running")
	if fs := factories(t, h); fs.Factories[0].Chat {
		t.Errorf("Factory %+v, want no Chat support", fs.Factories[0])
	}
	if n := h.Refused("/api/p/" + h.Project + "/chat/next-work"); n != 0 {
		t.Errorf("%d polls for Chat work, want none", n)
	}
}

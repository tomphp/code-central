package core_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/tomoram/code-central/core"
	"github.com/tomoram/code-central/store"
)

// Chat on a Shared Store (Spec code-central-1107): Conversations stay in
// memory on the service a Factory with Chat support reports to, so only
// that service offers Chat, and another names its host.

// chatOn is the Project's Chat as f reads it.
func chatOn(t *testing.T, f fixture) core.ConversationView {
	t.Helper()
	v, err := f.core.GetChat(ctx, "acme", core.GetChatInput{})
	if err != nil {
		t.Fatal(err)
	}
	return v
}

// refusedChat is f's refusal of a Chat Message.
func refusedChat(t *testing.T, f fixture) *core.Error {
	t.Helper()
	_, err := f.core.SendChatMessage(human, "acme", core.SendChatMessageInput{Text: "What is acme?"})
	var e *core.Error
	if !errors.As(err, &e) || e.Code != core.ActionNotAllowed {
		t.Fatalf("send_chat_message = %v, want action_not_allowed", err)
	}
	return e
}

// chatBeat is an Online Heartbeat with Chat support from cc on box.
func chatBeat(f fixture, t *testing.T) {
	t.Helper()
	if _, err := f.core.ReportHeartbeat(ctx, "acme", core.ReportHeartbeatInput{Factory: "cc", Host: "box", State: core.HeartbeatOnline, IntervalSeconds: 10, Chat: true}); err != nil {
		t.Fatal(err)
	}
}

func TestChatIsOfferedOnlyWhereAChatFactoryReportsAndNamedElsewhere(t *testing.T) {
	laptop, desktop := twoServices(t, store.NewMemory())
	for _, f := range []fixture{laptop, desktop} {
		if err := f.core.Register(ctx); err != nil {
			t.Fatal(err)
		}
	}
	tick := watchFactories(t, desktop)
	chatBeat(laptop, t)
	tick()

	if v := chatOn(t, desktop); v.Availability != core.ChatNone || v.RunsOn != "laptop" {
		t.Fatalf("the desktop's Chat is %+v, want none, running on laptop", v)
	}
	if e := refusedChat(t, desktop); !strings.Contains(e.Message, "Chat runs on laptop") {
		t.Fatalf("the desktop refused with %q, want it to name laptop", e.Message)
	}
	// A Conversation started on the desktop can't be answered there either.
	if _, err := desktop.core.StartConversation(human, "acme", core.StartConversationInput{}); err != nil {
		t.Fatal(err)
	}
	if v := chatOn(t, desktop); v.Availability != core.ChatUnavailable || v.RunsOn != "laptop" {
		t.Fatalf("the desktop's new Conversation is %+v, want unavailable, running on laptop", v)
	}
	if e := refusedChat(t, desktop); !strings.Contains(e.Message, "Chat runs on laptop") {
		t.Fatalf("the desktop refused with %q, want it to name laptop", e.Message)
	}

	// On the laptop Chat works as it does on one service.
	if v := chatOn(t, laptop); v.RunsOn != "" {
		t.Fatalf("the laptop's Chat runs on %q, want here", v.RunsOn)
	}
	if v, err := laptop.core.SendChatMessage(human, "acme", core.SendChatMessageInput{Text: "What is acme?"}); err != nil || v.Availability != core.ChatAvailable || v.RunsOn != "" {
		t.Fatalf("the laptop's send_chat_message = %+v, %v; want it available here", v, err)
	}
	if w, err := laptop.core.NextChatWork(ctx, "acme", core.NextChatWorkInput{Factory: "cc", Host: "box"}); err != nil || w.Turn == nil {
		t.Fatalf("next_chat_work on the laptop = %+v, %v; want the turn", w, err)
	}
}

func TestChatNamesNoHostWhenNoLiveServiceHasAChatFactory(t *testing.T) {
	t.Run("no Factory with Chat support", func(t *testing.T) {
		laptop, desktop := twoServices(t, store.NewMemory())
		if err := laptop.core.Register(ctx); err != nil {
			t.Fatal(err)
		}
		tick := watchFactories(t, desktop)
		beat(laptop, t, "cc", "box", core.HeartbeatOnline, 10)
		tick()
		if v := chatOn(t, desktop); v.RunsOn != "" {
			t.Fatalf("the desktop's Chat runs on %q, want nowhere", v.RunsOn)
		}
		if e := refusedChat(t, desktop); strings.Contains(e.Message, "Chat runs on") {
			t.Fatalf("the desktop refused with %q, naming a host", e.Message)
		}
	})
	t.Run("its service no longer registered", func(t *testing.T) {
		laptop, desktop := twoServices(t, store.NewMemory())
		if err := laptop.core.Register(ctx); err != nil {
			t.Fatal(err)
		}
		tick := watchFactories(t, desktop)
		chatBeat(laptop, t)
		tick()
		if err := laptop.core.Deregister(ctx); err != nil {
			t.Fatal(err)
		}
		tick()
		if v := chatOn(t, desktop); v.RunsOn != "" {
			t.Fatalf("the desktop's Chat runs on %q, a service that stopped", v.RunsOn)
		}
		if e := refusedChat(t, desktop); strings.Contains(e.Message, "Chat runs on") {
			t.Fatalf("the desktop refused with %q, naming a service that stopped", e.Message)
		}
	})
}

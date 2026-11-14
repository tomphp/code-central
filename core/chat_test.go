package core_test

import (
	"testing"
	"time"

	"github.com/tomoram/code-central/core"
	"github.com/tomoram/code-central/events"
)

// The Factory Status watch settles Conversations too: a reply whose
// Factory goes Offline with no write ends failed on the tick, and the UI
// hears of it without asking. The Chat operations' own cases are in
// frontdoor/chat_doors_test.go.
func TestTheWatchFailsAReplyWhoseFactoryWentOfflineAndSaysSo(t *testing.T) {
	f := newFixture(t)
	f.project(t, "acme")
	tick := watchFactories(t, f)
	if _, err := f.core.ReportHeartbeat(ctx, "acme", core.ReportHeartbeatInput{Factory: "cc", Host: "box", State: core.HeartbeatOnline, IntervalSeconds: 10, Chat: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.core.SendChatMessage(human, "acme", core.SendChatMessageInput{Text: "What is acme?"}); err != nil {
		t.Fatal(err)
	}
	if w, err := f.core.NextChatWork(ctx, "acme", core.NextChatWorkInput{Factory: "cc", Host: "box"}); err != nil || w.Turn == nil {
		t.Fatalf("next_chat_work = %+v, %v", w, err)
	}
	tick()
	notices, stop := f.core.Events().Subscribe()
	defer stop()

	f.clock.Advance(90 * time.Second)
	tick()
	select {
	case e := <-notices:
		if e.Type != events.ChatChanged || e.Project != "acme" {
			t.Fatalf("notice %+v, want acme's chat.changed", e)
		}
	default:
		t.Fatal("the tick sent no chat.changed")
	}
	v, err := f.core.GetChat(ctx, "acme")
	if err != nil {
		t.Fatal(err)
	}
	if last := v.Messages[len(v.Messages)-1]; v.Running || last.Outcome != core.ChatFailed || last.Error != "the Factory stopped answering" {
		t.Fatalf("reply %+v, want failed", last)
	}
}

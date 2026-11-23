package factories_test

import (
	"testing"
	"time"

	"github.com/tomoram/code-central/api"
	"github.com/tomoram/code-central/events"
)

// The Factory Status watch settles Conversations too: a reply whose
// Factory goes Offline with no write ends failed on the tick, and the UI
// hears of it without asking. The Chat operations' own cases are in
// server/chat_doors_test.go.
func TestTheWatchFailsAReplyWhoseFactoryWentOfflineAndSaysSo(t *testing.T) {
	f := newFixture(t)
	f.Project(t, "acme")
	tick := f.WatchFactories(t)
	if _, err := f.Core.ReportHeartbeat(ctx, "acme", api.ReportHeartbeatInput{Factory: "cc", Host: "box", State: api.HeartbeatOnline, IntervalSeconds: 10, Chat: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.Core.Factories().SendChatMessage(human, "acme", api.SendChatMessageInput{Text: "What is acme?"}); err != nil {
		t.Fatal(err)
	}
	if w, err := f.Core.Factories().NextChatWork(ctx, "acme", api.NextChatWorkInput{Factory: "cc", Host: "box"}); err != nil || w.Turn == nil {
		t.Fatalf("next_chat_work = %+v, %v", w, err)
	}
	tick()
	notices, stop := f.Core.Events().Subscribe()
	defer stop()

	f.Clock.Advance(90 * time.Second)
	tick()
	select {
	case e := <-notices:
		if e.Type != events.ChatChanged || e.Project != "acme" {
			t.Fatalf("notice %+v, want acme's chat.changed", e)
		}
	default:
		t.Fatal("the tick sent no chat.changed")
	}
	v, err := f.Core.Factories().GetChat(ctx, "acme", api.GetChatInput{})
	if err != nil {
		t.Fatal(err)
	}
	if last := v.Messages[len(v.Messages)-1]; v.Running || last.Outcome != api.ChatFailed || last.Error != "the Factory stopped answering" {
		t.Fatalf("reply %+v, want failed", last)
	}
}

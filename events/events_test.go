package events_test

import (
	"testing"

	"github.com/tomoram/code-central/events"
)

func TestSubscribersReceiveEveryNoticePublishedWhileSubscribed(t *testing.T) {
	b := events.New()
	a, stopA := b.Subscribe()
	c, stopC := b.Subscribe()
	defer stopC()

	e := events.Event{Type: events.TicketChanged, Project: "acme", ID: "acme-2"}
	b.Publish(e)
	if got := <-a; got != e {
		t.Fatalf("a got %+v", got)
	}
	if got := <-c; got != e {
		t.Fatalf("c got %+v", got)
	}

	stopA()
	stopA() // stopping twice is harmless
	b.Publish(events.Event{Type: events.ProjectChanged, Project: "acme"})
	if got := <-c; got.Type != events.ProjectChanged {
		t.Fatalf("c got %+v", got)
	}
	if _, open := <-a; open {
		t.Fatal("a stopped subscriber's channel should be closed")
	}
}

func TestASlowSubscriberNeverBlocksPublish(t *testing.T) {
	b := events.New()
	_, stop := b.Subscribe()
	defer stop()
	for range 10 * events.Buffer {
		b.Publish(events.Event{Type: events.TicketChanged, Project: "acme", ID: "acme-2"})
	}
}

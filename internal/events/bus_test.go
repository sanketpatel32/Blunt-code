package events

import (
	"testing"
	"time"
)

func TestBusPublishes(t *testing.T) {
	b := New()
	ch, unsubscribe := b.Subscribe("scan")
	defer unsubscribe()
	b.Publish(Event{Type: "scan.started", ScanID: "scan"})
	select {
	case got := <-ch:
		if got.Type != "scan.started" {
			t.Fatal(got)
		}
	case <-time.After(time.Second):
		t.Fatal("event was not delivered")
	}
}

func TestBusReplaysRecentEventsToNewSubscriber(t *testing.T) {
	b := New()
	b.Publish(Event{Type: "scan.started", ScanID: "scan"})
	b.Publish(Event{Type: "analyzer.started", ScanID: "scan"})

	ch, unsubscribe := b.Subscribe("scan")
	defer unsubscribe()
	for _, want := range []string{"scan.started", "analyzer.started"} {
		select {
		case got := <-ch:
			if got.Type != want {
				t.Fatalf("event type = %q, want %q", got.Type, want)
			}
		case <-time.After(time.Second):
			t.Fatalf("timed out waiting for %q", want)
		}
	}
}

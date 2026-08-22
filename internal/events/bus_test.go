package events

import (
	"strconv"
	"sync"
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

func TestBusHistoryIsCappedAtLimit(t *testing.T) {
	b := New()
	for i := 0; i < historyLimit+8; i++ {
		b.Publish(Event{Type: "e" + strconv.Itoa(i), ScanID: "scan"})
	}
	ch, unsubscribe := b.Subscribe("scan")
	defer unsubscribe()
	events := make([]Event, 0, historyLimit)
	for len(events) < historyLimit {
		select {
		case got := <-ch:
			events = append(events, got)
		case <-time.After(time.Second):
			t.Fatalf("timed out after %d replayed events", len(events))
		}
	}
	select {
	case extra := <-ch:
		t.Fatalf("history replayed more than %d events: %#v", historyLimit, extra)
	default:
	}
	first, last := historyLimit+8-historyLimit, historyLimit+8-1
	if events[0].Type != "e"+strconv.Itoa(first) || events[len(events)-1].Type != "e"+strconv.Itoa(last) {
		t.Fatalf("history kept %#v .. %#v, want e%d .. e%d", events[0].Type, events[len(events)-1].Type, first, last)
	}
}

// TestSubscribeBridgesReplayToLiveWithoutLossOrDuplication pins the SSE gap
// guarantee: a subscriber that connects while events are being published must
// see a single ordered stream with no duplicates and no missing sequence
// numbers across the replay-history to live-subscription handoff. Events that
// already fell out of the bounded history are legitimately unavailable; the
// invariant is contiguity from the first replayed event to the final live one.
func TestSubscribeBridgesReplayToLiveWithoutLossOrDuplication(t *testing.T) {
	b := New()
	const total = 400
	var publisher sync.WaitGroup
	publisher.Add(1)
	go func() {
		defer publisher.Done()
		for i := 0; i < total; i++ {
			b.Publish(Event{Type: "seq", ScanID: "scan", Data: map[string]any{"n": i}})
			if i%8 == 0 {
				time.Sleep(time.Millisecond)
			}
		}
	}()
	time.Sleep(3 * time.Millisecond) // let some history accumulate first
	ch, unsubscribe := b.Subscribe("scan")
	defer unsubscribe()
	seen := map[int]bool{}
	first, previous, count := -1, -1, 0
	deadline := time.After(5 * time.Second)
	for previous != total-1 {
		select {
		case got := <-ch:
			n := got.Data.(map[string]any)["n"].(int)
			if seen[n] {
				t.Fatalf("event %d delivered twice", n)
			}
			if n <= previous {
				t.Fatalf("ordering violated: %d after %d", n, previous)
			}
			if first < 0 {
				first = n
			}
			previous = n
			seen[n] = true
			count++
		case <-deadline:
			t.Fatalf("stream stalled after %d events (last %d of %d); the replay/live handoff lost events", count, previous, total)
		}
	}
	publisher.Wait()
	if count != previous-first+1 {
		t.Fatalf("saw %d events from %d to %d; the stream has gaps", count, first, previous)
	}
	if count < 2 {
		t.Fatalf("test connected too late to exercise the replay/live boundary (only event %d seen)", first)
	}
}

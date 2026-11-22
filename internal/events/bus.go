// Package events provides bounded in-memory scan progress fan-out.
package events

import (
	"sync"
	"time"
)

const historyLimit = 32

// historyScanLimit bounds how many scans keep replay history, so a
// long-lived server that has seen thousands of scans holds bounded memory.
const historyScanLimit = 500

type Event struct {
	Type   string    `json:"type"`
	ScanID string    `json:"scan_id"`
	Data   any       `json:"data,omitempty"`
	At     time.Time `json:"at"`
}
type Bus struct {
	mu          sync.RWMutex
	subscribers map[string]map[chan Event]struct{}
	history     map[string][]Event
	// lastActive orders scans by their most recent publish (a monotonic
	// sequence, immune to clock changes) so eviction drops the oldest scan.
	lastActive map[string]uint64
	sequence   uint64
}

func New() *Bus {
	return &Bus{
		subscribers: make(map[string]map[chan Event]struct{}),
		history:     make(map[string][]Event),
		lastActive:  make(map[string]uint64),
	}
}
func (b *Bus) Subscribe(scanID string) (<-chan Event, func()) {
	ch := make(chan Event, 32)
	b.mu.Lock()
	for _, event := range b.history[scanID] {
		ch <- event
	}
	if b.subscribers[scanID] == nil {
		b.subscribers[scanID] = map[chan Event]struct{}{}
	}
	b.subscribers[scanID][ch] = struct{}{}
	b.mu.Unlock()
	return ch, func() {
		b.mu.Lock()
		if subs := b.subscribers[scanID]; subs != nil {
			delete(subs, ch)
			if len(subs) == 0 {
				delete(b.subscribers, scanID)
			}
		}
		b.mu.Unlock()
	}
}
func (b *Bus) Publish(event Event) {
	if event.At.IsZero() {
		event.At = time.Now().UTC()
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	history := append(b.history[event.ScanID], event)
	if len(history) > historyLimit {
		history = history[len(history)-historyLimit:]
	}
	b.history[event.ScanID] = history
	b.sequence++
	b.lastActive[event.ScanID] = b.sequence
	if len(b.history) > historyScanLimit {
		b.evictOldestIdle(event.ScanID)
	}
	for ch := range b.subscribers[event.ScanID] {
		select {
		case ch <- event:
		default:
		}
	}
}

// evictOldestIdle drops the replay history of the least recently active scan
// that nobody is watching. Scans with live subscribers are never evicted, so
// a busy server can never cut off the stream a user is watching; while every
// other scan is being watched, history may exceed historyScanLimit by at
// most the number of watched scans.
func (b *Bus) evictOldestIdle(current string) {
	oldest, oldestSeq := "", uint64(0)
	for scanID := range b.history {
		if scanID == current || len(b.subscribers[scanID]) > 0 {
			continue
		}
		if seq := b.lastActive[scanID]; oldest == "" || seq < oldestSeq {
			oldest, oldestSeq = scanID, seq
		}
	}
	if oldest != "" {
		delete(b.history, oldest)
		delete(b.lastActive, oldest)
	}
}

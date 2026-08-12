// Package events provides bounded in-memory scan progress fan-out.
package events

import (
	"sync"
	"time"
)

const historyLimit = 32

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
}

func New() *Bus {
	return &Bus{
		subscribers: make(map[string]map[chan Event]struct{}),
		history:     make(map[string][]Event),
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
	for ch := range b.subscribers[event.ScanID] {
		select {
		case ch <- event:
		default:
		}
	}
}

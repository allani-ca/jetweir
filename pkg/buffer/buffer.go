package buffer

import (
	"sync"
	"time"

	"github.com/allani-ca/jetweir/pkg/types"
)

// RollingBuffer maintains a time-based rolling buffer of recent events for replay.
type RollingBuffer struct {
	mu      sync.RWMutex
	events  []*types.EventWithMetadata
	maxSize int
	ttl     time.Duration
	nextSeq int64
}

// NewRollingBuffer creates a new rolling event buffer.
func NewRollingBuffer(maxSize int, ttl time.Duration) *RollingBuffer {
	return &RollingBuffer{
		events:  make([]*types.EventWithMetadata, 0, maxSize),
		maxSize: maxSize,
		ttl:     ttl,
		nextSeq: 1,
	}
}

// Add adds an event to the buffer, respecting size and TTL limits.
func (rb *RollingBuffer) Add(event *types.JetstreamEvent) {
	rb.mu.Lock()
	defer rb.mu.Unlock()

	now := time.Now()
	wrapped := &types.EventWithMetadata{
		Event:       *event,
		ReceivedAt:  now,
		SequenceNum: rb.nextSeq,
	}
	rb.nextSeq++

	// Remove expired events
	rb.expireOldEvents(now)

	// Add new event
	rb.events = append(rb.events, wrapped)

	// If we exceeded size, remove oldest
	if len(rb.events) > rb.maxSize {
		rb.events = rb.events[1:]
	}
}

// GetFromCursor retrieves events from the buffer starting at cursor.
// Returns events and whether cursor is valid in the buffer.
func (rb *RollingBuffer) GetFromCursor(cursor int64) ([]*types.EventWithMetadata, bool) {
	rb.mu.RLock()
	defer rb.mu.RUnlock()

	if len(rb.events) == 0 {
		return nil, false
	}

	// If cursor is -1, start from beginning
	if cursor == -1 {
		return rb.events, true
	}

	// Find the index where cursor matches
	for i, evt := range rb.events {
		if evt.SequenceNum > cursor {
			return rb.events[i:], true
		}
	}

	// Cursor is beyond buffer
	return nil, false
}

// GetLatestSequence returns the latest sequence number in the buffer.
func (rb *RollingBuffer) GetLatestSequence() int64 {
	rb.mu.RLock()
	defer rb.mu.RUnlock()

	if len(rb.events) == 0 {
		return 0
	}
	return rb.events[len(rb.events)-1].SequenceNum
}

// Size returns the current number of events in the buffer.
func (rb *RollingBuffer) Size() int {
	rb.mu.RLock()
	defer rb.mu.RUnlock()
	return len(rb.events)
}

// Utilization returns the percentage of buffer capacity used.
func (rb *RollingBuffer) Utilization() float64 {
	rb.mu.RLock()
	defer rb.mu.RUnlock()
	if rb.maxSize == 0 {
		return 0
	}
	return float64(len(rb.events)) / float64(rb.maxSize)
}

func (rb *RollingBuffer) expireOldEvents(now time.Time) {
	cutoff := now.Add(-rb.ttl)
	for len(rb.events) > 0 && rb.events[0].ReceivedAt.Before(cutoff) {
		rb.events = rb.events[1:]
	}
}

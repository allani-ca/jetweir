package types

import (
	"time"
)

// JetstreamEvent represents a Jetstream event from the upstream connection.
type JetstreamEvent struct {
	Did      string          `json:"did"`
	Seq      int64           `json:"seq"`
	Time     string          `json:"time"`
	Commit   string          `json:"commit"`
	Rev      string          `json:"rev"`
	Operation string          `json:"operation"`
	Blocks   map[string]string `json:"blocks,omitempty"`
	Ops      []Operation     `json:"ops,omitempty"`
}

// Operation represents a CAR operation
type Operation struct {
	Action string      `json:"action"`
	Path   string      `json:"path"`
	CID    string      `json:"cid,omitempty"`
}

// SubscriptionFilters represent the official Jetstream v2 subscription filters
type SubscriptionFilters struct {
	WantedCollections []string
	WantedDIDs        []string
	Cursor            int64
	MaxMessageSizeBytes int
	Compress          bool
}

// EventWithMetadata wraps an event with internal metadata
type EventWithMetadata struct {
	Event       JetstreamEvent
	ReceivedAt  time.Time
	SequenceNum int64
}

// ClientMessage represents a message sent to a downstream client
type ClientMessage struct {
	Event JetstreamEvent `json:"event"`
	Error *ErrorMessage  `json:"error,omitempty"`
}

type ErrorMessage struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// SubscriptionHandle represents a unique subscription
type SubscriptionHandle struct {
	ID      string
	Filters SubscriptionFilters
}

// ConnectionStats represents statistics about the system
type ConnectionStats struct {
	ActiveUpstreamConnections  int
	ConnectedDownstreamClients int
	SubscriptionReuseCount     map[string]int
	EventsReceived             int64
	EventsDistributed          int64
	UpstreamReconnects         int64
	BufferUtilization          float64
	ConsumerLag                map[string]int64
}

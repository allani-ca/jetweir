package downstream

import (
	"context"
	"log"
	"sync"
	"time"

	"github.com/gorilla/websocket"

	"github.com/allani-ca/jetweir/pkg/buffer"
	"github.com/allani-ca/jetweir/pkg/metrics"
	"github.com/allani-ca/jetweir/pkg/protocol"
	"github.com/allani-ca/jetweir/pkg/subscription"
	"github.com/allani-ca/jetweir/pkg/types"
)

// Handler manages a single downstream client connection.
type Handler struct {
	ID                   string
	ws                   *websocket.Conn
	filters              types.SubscriptionFilters
	eventBuffer          *buffer.RollingBuffer
	subscriptionManager  *subscription.Manager
	metrics              *metrics.Metrics
	eventChan            chan *types.JetstreamEvent
	eventMapping         map[string]<-chan *types.JetstreamEvent // subscriptionID -> event channel
	writeDeadline        time.Duration
	readDeadline         time.Duration
	slowConsumerTimeout  time.Duration
	bufferSize           int
	ctx                  context.Context
	cancel               context.CancelFunc
	mu                   sync.Mutex
	closed               bool
	currentCursor        int64
	lastEventTime        time.Time
}

// NewHandler creates a new downstream client handler.
func NewHandler(
	id string,
	ws *websocket.Conn,
	filters types.SubscriptionFilters,
	eventBuffer *buffer.RollingBuffer,
	subMgr *subscription.Manager,
	m *metrics.Metrics,
	writeDeadline, readDeadline, slowConsumerTimeout time.Duration,
	bufferSize int,
) *Handler {
	ctx, cancel := context.WithCancel(context.Background())

	h := &Handler{
		ID:                  id,
		ws:                  ws,
		filters:             filters,
		eventBuffer:         eventBuffer,
		subscriptionManager: subMgr,
		metrics:             m,
		eventChan:           make(chan *types.JetstreamEvent, bufferSize),
		eventMapping:        make(map[string]<-chan *types.JetstreamEvent),
		writeDeadline:       writeDeadline,
		readDeadline:        readDeadline,
		slowConsumerTimeout: slowConsumerTimeout,
		bufferSize:          bufferSize,
		ctx:                 ctx,
		cancel:              cancel,
		currentCursor:       filters.Cursor,
		lastEventTime:       time.Now(),
	}

	// Subscribe to upstream subscriptions
	h.subscribeToUpstream()

	return h
}

// subscribeToUpstream gets or creates subscriptions for the filters.
func (h *Handler) subscribeToUpstream() {
	// Try exact match first
	_ = h.subscriptionManager.GetOrCreateSubscription(h.ID, h.filters)
	// In a full implementation, we'd track upstream event channels
	// For now, we'll handle event distribution at the server level
}

// Start starts the handler's main loop.
func (h *Handler) Start() {
	go h.readLoop()
	go h.writeLoop()
}

// Close closes the handler.
func (h *Handler) Close() error {
	h.mu.Lock()
	if h.closed {
		h.mu.Unlock()
		return nil
	}
	h.closed = true
	h.mu.Unlock()

	h.cancel()
	close(h.eventChan)
	h.subscriptionManager.RemoveClient(h.ID)
	h.metrics.IncDownstreamDisconnects()

	return h.ws.Close()
}

// SendEvent sends an event to the client.
func (h *Handler) SendEvent(event *types.JetstreamEvent) bool {
	// Check if event matches filters
	if !protocol.FilterMatches(event, h.filters) {
		return true // Silently skip non-matching events
	}

	select {
	case h.eventChan <- event:
		return true
	case <-time.After(h.slowConsumerTimeout):
		log.Printf("Slow consumer timeout for client %s, disconnecting", h.ID)
		h.metrics.IncEventsDropped()
		h.Close()
		return false
	case <-h.ctx.Done():
		return false
	}
}

func (h *Handler) readLoop() {
	for {
		select {
		case <-h.ctx.Done():
			return
		default:
		}

		h.ws.SetReadDeadline(time.Now().Add(h.readDeadline))
		// Read any client messages (cursor updates, etc.)
		_, _, err := h.ws.ReadMessage()
		if err != nil {
			log.Printf("Error reading from client %s: %v", h.ID, err)
			h.Close()
			return
		}
	}
}

func (h *Handler) writeLoop() {
	for {
		select {
		case event, ok := <-h.eventChan:
			if !ok {
				return
			}

			h.mu.Lock()
			if h.closed {
				h.mu.Unlock()
				return
			}
			h.mu.Unlock()

			msg := types.ClientMessage{
				Event: *event,
			}

			h.ws.SetWriteDeadline(time.Now().Add(h.writeDeadline))
			if err := h.ws.WriteJSON(msg); err != nil {
				log.Printf("Error writing to client %s: %v", h.ID, err)
				h.Close()
				return
			}

			h.metrics.IncEventsDistributed()
			h.currentCursor = event.Seq
			h.lastEventTime = time.Now()

		case <-h.ctx.Done():
			return
		}
	}
}

// GetCurrentCursor returns the client's current cursor position.
func (h *Handler) GetCurrentCursor() int64 {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.currentCursor
}

// IsClosed returns whether the handler is closed.
func (h *Handler) IsClosed() bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.closed
}

// Wait blocks until the handler is closed.
func (h *Handler) Wait() {
	<-h.ctx.Done()
}

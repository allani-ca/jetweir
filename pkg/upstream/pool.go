package upstream

import (
	"context"
	"fmt"
	"log"
	"net/url"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"

	"github.com/allani-ca/jetweir/pkg/buffer"
	"github.com/allani-ca/jetweir/pkg/metrics"
	"github.com/allani-ca/jetweir/pkg/types"
)

// ConnectionPool manages upstream WebSocket connections with reconnection logic.
type ConnectionPool struct {
	mu                sync.RWMutex
	baseURL            string
	connections        map[string]*Connection
	eventBuffer        *buffer.RollingBuffer
	metrics            *metrics.Metrics
	reconnectBase      time.Duration
	reconnectMax       time.Duration
	readTimeout        time.Duration
	writeTimeout       time.Duration
	ctx                context.Context
	cancel             context.CancelFunc
}

// Connection represents a single upstream WebSocket connection.
type Connection struct {
	ID               string
	Filters          types.SubscriptionFilters
	ws               *websocket.Conn
	eventChan        chan *types.JetstreamEvent
	reconnectCount   int64
	lastError        error
	lastConnectedAt  time.Time
	isClosed         bool
	mu               sync.Mutex
}

// NewConnectionPool creates a new connection pool.
func NewConnectionPool(baseURL string, eventBuffer *buffer.RollingBuffer, m *metrics.Metrics, reconnectBase, reconnectMax, readTimeout, writeTimeout time.Duration) *ConnectionPool {
	ctx, cancel := context.WithCancel(context.Background())
	return &ConnectionPool{
		baseURL:       baseURL,
		connections:   make(map[string]*Connection),
		eventBuffer:   eventBuffer,
		metrics:       m,
		reconnectBase: reconnectBase,
		reconnectMax:  reconnectMax,
		readTimeout:   readTimeout,
		writeTimeout:  writeTimeout,
		ctx:           ctx,
		cancel:        cancel,
	}
}

// GetOrCreateConnection gets or creates an upstream connection for the given filters.
func (cp *ConnectionPool) GetOrCreateConnection(id string, filters types.SubscriptionFilters) (*Connection, error) {
	cp.mu.Lock()
	defer cp.mu.Unlock()

	if conn, exists := cp.connections[id]; exists {
		return conn, nil
	}

	conn := &Connection{
		ID:        id,
		Filters:   filters,
		eventChan: make(chan *types.JetstreamEvent, 100),
	}

	if err := cp.connectWebSocket(conn); err != nil {
		return nil, err
	}

	cp.connections[id] = conn
	cp.metrics.SetActiveUpstreamConnections(len(cp.connections))

	go cp.readLoop(conn)

	return conn, nil
}

// GetEventChannel returns the event channel for a connection.
func (c *Connection) GetEventChannel() <-chan *types.JetstreamEvent {
	return c.eventChan
}

// Close closes a connection.
func (cp *ConnectionPool) CloseConnection(id string) error {
	cp.mu.Lock()
	defer cp.mu.Unlock()

	conn, exists := cp.connections[id]
	if !exists {
		return nil
	}

	conn.mu.Lock()
	defer conn.mu.Unlock()

	conn.isClosed = true
	close(conn.eventChan)
	if conn.ws != nil {
		conn.ws.Close()
	}

	delete(cp.connections, id)
	cp.metrics.SetActiveUpstreamConnections(len(cp.connections))

	return nil
}

// GetStats returns connection pool statistics.
func (cp *ConnectionPool) GetStats() int {
	cp.mu.RLock()
	defer cp.mu.RUnlock()
	return len(cp.connections)
}

// Shutdown shuts down the connection pool.
func (cp *ConnectionPool) Shutdown(ctx context.Context) error {
	cp.cancel()

	cp.mu.Lock()
	ids := make([]string, 0, len(cp.connections))
	for id := range cp.connections {
		ids = append(ids, id)
	}
	cp.mu.Unlock()

	for _, id := range ids {
		cp.CloseConnection(id)
	}

	return nil
}

func (cp *ConnectionPool) connectWebSocket(conn *Connection) error {
	wsURL := cp.buildWebSocketURL(conn.Filters)
	dialer := websocket.Dialer{
		HandshakeTimeout: 10 * time.Second,
	}

	ws, _, err := dialer.Dial(wsURL, nil)
	if err != nil {
		return err
	}

	conn.ws = ws
	conn.lastConnectedAt = time.Now()
	atomic.AddInt64(&conn.reconnectCount, 0) // Reset
	return nil
}

func (cp *ConnectionPool) buildWebSocketURL(filters types.SubscriptionFilters) string {
	params := url.Values{}

	if len(filters.WantedCollections) > 0 {
		for _, c := range filters.WantedCollections {
			params.Add("wantedCollections", c)
		}
	}

	if len(filters.WantedDIDs) > 0 {
		for _, d := range filters.WantedDIDs {
			params.Add("wantedDids", d)
		}
	}

	if filters.Cursor >= 0 {
		params.Set("cursor", fmt.Sprintf("%d", filters.Cursor))
	}

	params.Set("maxMessageSizeBytes", fmt.Sprintf("%d", filters.MaxMessageSizeBytes))

	if filters.Compress {
		params.Set("compress", "true")
	}

	return cp.baseURL + "?" + params.Encode()
}

func (cp *ConnectionPool) readLoop(conn *Connection) {
	backoff := cp.reconnectBase
	for {
		select {
		case <-cp.ctx.Done():
			return
		default:
		}

		if conn.ws == nil {
			select {
			case <-time.After(backoff):
				if err := cp.connectWebSocket(conn); err != nil {
					cp.metrics.IncUpstreamConnectionErrors()
					log.Printf("Failed to reconnect upstream: %v", err)
					backoff = cp.exponentialBackoff(backoff)
					continue
				}
				cp.metrics.IncUpstreamReconnects()
				backoff = cp.reconnectBase
			case <-cp.ctx.Done():
				return
			}
		}

		conn.mu.Lock()
		if conn.isClosed {
			conn.mu.Unlock()
			return
		}
		ws := conn.ws
		conn.mu.Unlock()

		if ws == nil {
			continue
		}

		ws.SetReadDeadline(time.Now().Add(cp.readTimeout))
		var event types.JetstreamEvent
		err := ws.ReadJSON(&event)
		if err != nil {
			cp.metrics.IncUpstreamConnectionErrors()
			conn.lastError = err
			log.Printf("Error reading from upstream: %v", err)
			conn.mu.Lock()
			conn.ws = nil
			conn.mu.Unlock()
			backoff = cp.exponentialBackoff(backoff)
			continue
		}

		cp.eventBuffer.Add(&event)
		cp.metrics.IncEventsReceived()

		select {
		case conn.eventChan <- &event:
		case <-cp.ctx.Done():
			return
		}
	}
}

func (cp *ConnectionPool) exponentialBackoff(current time.Duration) time.Duration {
	next := time.Duration(float64(current) * 1.5)
	if next > cp.reconnectMax {
		return cp.reconnectMax
	}
	return next
}

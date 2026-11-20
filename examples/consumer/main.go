package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gorilla/websocket"
	"github.com/prometheus/client_golang/prometheus"
	promhttp "github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	jetweir         = flag.String("jetweir", "ws://localhost:8080/subscribe", "Jetweir subscription endpoint")
	listenAddr      = flag.String("listen", ":8081", "Prometheus metrics listen address")
	wantedDIDs      = flag.String("dids", "", "Comma-separated DIDs to subscribe to")
	wantedCollections = flag.String("collections", "", "Comma-separated collections to subscribe to")
	bufferSize      = flag.Int("buffer", 100, "Event buffer size")
)

type Metrics struct {
	eventsReceived prometheus.Counter
	eventsSkipped  prometheus.Counter
	connectionStatus prometheus.Gauge
	lastEventTime  prometheus.Gauge
}

func main() {
	flag.Parse()

	// Initialize metrics
	m := &Metrics{
		eventsReceived: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "jetweir_consumer_events_received_total",
			Help: "Total events received from Jetweir",
		}),
		eventsSkipped: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "jetweir_consumer_events_skipped_total",
			Help: "Total events skipped (non-matching or errors)",
		}),
		connectionStatus: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "jetweir_consumer_connection_status",
			Help: "Connection status (1 = connected, 0 = disconnected)",
		}),
		lastEventTime: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "jetweir_consumer_last_event_timestamp",
			Help: "Unix timestamp of last received event",
		}),
	}

	prometheus.MustRegister(m.eventsReceived, m.eventsSkipped, m.connectionStatus, m.lastEventTime)

	// Setup metrics HTTP server
	http.Handle("/metrics", promhttp.Handler())
	go func() {
		log.Printf("Starting metrics server on %s", *listenAddr)
		if err := http.ListenAndServe(*listenAddr, nil); err != nil {
			log.Fatalf("Metrics server error: %v", err)
		}
	}()

	// Setup signal handling for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigChan
		log.Println("Shutdown signal received")
		cancel()
	}()

	// Connect to Jetweir with reconnection logic
	connectToJetweir(ctx, m)

	log.Println("Consumer shutdown complete")
}

func connectToJetweir(ctx context.Context, m *Metrics) {
	reconnectDelay := time.Second

	for {
		select {
		case <-ctx.Done():
			m.connectionStatus.Set(0)
			return
		default:
		}

		log.Printf("Connecting to Jetweir at %s", *jetweir)
		if err := consumeJetweir(ctx, m); err != nil {
			log.Printf("Consumer error: %v, reconnecting in %v", err, reconnectDelay)
			m.connectionStatus.Set(0)

			select {
			case <-time.After(reconnectDelay):
				// Exponential backoff up to 30 seconds
				reconnectDelay *= 2
				if reconnectDelay > 30*time.Second {
					reconnectDelay = 30 * time.Second
				}
			case <-ctx.Done():
				return
			}
		}
	}
}

func consumeJetweir(ctx context.Context, m *Metrics) error {
	// Build URL with filters
	u, err := url.Parse(*jetweir)
	if err != nil {
		return fmt.Errorf("invalid jetweir URL: %w", err)
	}

	q := u.Query()
	if *wantedDIDs != "" {
		q.Set("wantedDids", *wantedDIDs)
	}
	if *wantedCollections != "" {
		q.Set("wantedCollections", *wantedCollections)
	}
	u.RawQuery = q.Encode()

	// Connect WebSocket
	dialer := websocket.Dialer{
		HandshakeTimeout: 10 * time.Second,
	}
	ws, _, err := dialer.DialContext(ctx, u.String(), nil)
	if err != nil {
		return fmt.Errorf("websocket dial failed: %w", err)
	}
	defer ws.Close()

	log.Println("Connected to Jetweir")
	m.connectionStatus.Set(1)

	eventChan := make(chan json.RawMessage, *bufferSize)
	errChan := make(chan error, 1)

	// Read loop
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			default:
			}

			var msg map[string]json.RawMessage
			ws.SetReadDeadline(time.Now().Add(60 * time.Second))
			if err := ws.ReadJSON(&msg); err != nil {
				errChan <- fmt.Errorf("read error: %w", err)
				return
			}

			if eventRaw, ok := msg["event"]; ok {
				select {
				case eventChan <- eventRaw:
				case <-ctx.Done():
					return
				}
			}
		}
	}()

	// Process events
	for {
		select {
		case <-ctx.Done():
			return nil

		case err := <-errChan:
			return err

		case eventRaw := <-eventChan:
			var event map[string]interface{}
			if err := json.Unmarshal(eventRaw, &event); err != nil {
				m.eventsSkipped.Inc()
				log.Printf("Failed to unmarshal event: %v", err)
				continue
			}

			m.eventsReceived.Inc()
			m.lastEventTime.Set(float64(time.Now().Unix()))

			// Log event summary
			if did, ok := event["did"].(string); ok {
				log.Printf("Event from %s", did)
			}
		}
	}
}

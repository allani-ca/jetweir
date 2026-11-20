package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/gorilla/websocket"
	"github.com/prometheus/client_golang/prometheus"
	promhttp "github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/allani-ca/jetweir/config"
	"github.com/allani-ca/jetweir/pkg/buffer"
	"github.com/allani-ca/jetweir/pkg/downstream"
	"github.com/allani-ca/jetweir/pkg/metrics"
	"github.com/allani-ca/jetweir/pkg/protocol"
	"github.com/allani-ca/jetweir/pkg/subscription"
	"github.com/allani-ca/jetweir/pkg/upstream"
)

type Server struct {
	config               *config.Config
	subscriptionManager  *subscription.Manager
	upstreamPool         *upstream.ConnectionPool
	eventBuffer          *buffer.RollingBuffer
	metrics              *metrics.Metrics
	wsUpgrader           websocket.Upgrader
	httpServer           *http.Server
	metricsServer        *http.Server
	activeHandlers       map[string]*downstream.Handler
	activeHandlersMu     sync.RWMutex
	clientIDCounter      int64
	ctx                  context.Context
	cancel               context.CancelFunc
}

func main() {
	cfg := config.Load()

	// Initialize metrics
	m := metrics.New(prometheus.DefaultRegisterer)

	// Initialize event buffer
	buf := buffer.NewRollingBuffer(cfg.EventBufferSize, cfg.EventBufferTTL)

	// Initialize subscription manager
	subMgr := subscription.NewManager()

	// Initialize upstream pool
	upstreamPool := upstream.NewConnectionPool(
		cfg.UpstreamURL,
		buf,
		m,
		cfg.ReconnectBaseBackoff,
		cfg.ReconnectMaxBackoff,
		cfg.ReadTimeout,
		cfg.WriteTimeout,
	)

	ctx, cancel := context.WithCancel(context.Background())

	server := &Server{
		config:              cfg,
		subscriptionManager: subMgr,
		upstreamPool:        upstreamPool,
		eventBuffer:         buf,
		metrics:             m,
		wsUpgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool {
				return true // Allow all origins for LAN use
			},
		},
		activeHandlers: make(map[string]*downstream.Handler),
		ctx:            ctx,
		cancel:         cancel,
	}

	// Setup HTTP routes
	mux := http.NewServeMux()
	mux.HandleFunc("/subscribe", server.handleSubscribe)
	mux.HandleFunc("/xrpc/network.bsky.jetstream.subscribeEvents", server.handleSubscribe)
	mux.HandleFunc("/health", server.handleHealth)
	mux.HandleFunc("/status", server.handleStatus)
	mux.HandleFunc("/metrics", promhttp.Handler().ServeHTTP)

	server.httpServer = &http.Server{
		Addr:           fmt.Sprintf("%s:%d", cfg.ListenAddr, cfg.ListenPort),
		Handler:        mux,
		ReadTimeout:    cfg.ReadTimeout,
		WriteTimeout:   cfg.WriteTimeout,
		MaxHeaderBytes: 1 << 20,
	}

	// Setup metrics server if enabled
	if cfg.EnableMetrics {
		metricsMux := http.NewServeMux()
		metricsMux.Handle("/metrics", promhttp.Handler())
		server.metricsServer = &http.Server{
			Addr:    fmt.Sprintf(":%d", cfg.MetricsPort),
			Handler: metricsMux,
		}
	}

	// Start servers
	go func() {
		log.Printf("Starting Jetweir service on %s", server.httpServer.Addr)
		if err := server.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("HTTP server error: %v", err)
		}
	}()

	if server.metricsServer != nil {
		go func() {
			log.Printf("Starting metrics server on %s", server.metricsServer.Addr)
			if err := server.metricsServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				log.Fatalf("Metrics server error: %v", err)
			}
		}()
	}

	// Start metrics update loop
	go server.metricsUpdateLoop()

	// Handle graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	log.Println("Shutting down gracefully...")
	server.shutdown(ctx)
}

func (s *Server) handleSubscribe(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Parse subscription parameters
	filters := protocol.ParseSubscriptionParams(r.URL.Query())

	// Upgrade to WebSocket
	conn, err := s.wsUpgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("WebSocket upgrade error: %v", err)
		return
	}

	clientID := fmt.Sprintf("client-%d", s.generateClientID())
	log.Printf("Client %s connected with filters: %+v", clientID, filters)

	// Create downstream handler
	handler := downstream.NewHandler(
		clientID,
		conn,
		filters,
		s.eventBuffer,
		s.subscriptionManager,
		s.metrics,
		s.config.WriteTimeout,
		s.config.ReadTimeout,
		s.config.SlowConsumerTimeout,
		s.config.DownstreamBufferSize,
	)

	s.activeHandlersMu.Lock()
	s.activeHandlers[clientID] = handler
	s.activeHandlersMu.Unlock()

	handler.Start()

	// Wait for handler to close
	handler.Wait()

	s.activeHandlersMu.Lock()
	delete(s.activeHandlers, clientID)
	s.activeHandlersMu.Unlock()

	log.Printf("Client %s disconnected", clientID)
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status": "healthy",
		"time":   time.Now().Unix(),
	})
}

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	s.activeHandlersMu.RLock()
	clientCount := len(s.activeHandlers)
	s.activeHandlersMu.RUnlock()

	subCount, reuseMap := s.subscriptionManager.GetStats()
	upstreamCount := s.upstreamPool.GetStats()

	var totalReuse int
	for _, count := range reuseMap {
		totalReuse += count
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"active_upstream_connections":  upstreamCount,
		"connected_downstream_clients": clientCount,
		"active_subscriptions":         subCount,
		"subscription_reuse_total":     totalReuse,
		"buffer_size":                  s.eventBuffer.Size(),
		"buffer_utilization":           s.eventBuffer.Utilization(),
		"timestamp":                    time.Now().Unix(),
	})
}

func (s *Server) metricsUpdateLoop() {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			s.activeHandlersMu.RLock()
			clientCount := len(s.activeHandlers)
			s.activeHandlersMu.RUnlock()

			subCount, _ := s.subscriptionManager.GetStats()
			upstreamCount := s.upstreamPool.GetStats()

			s.metrics.SetConnectedDownstreamClients(clientCount)
			s.metrics.SetActiveSubscriptions(subCount)
			s.metrics.SetActiveUpstreamConnections(upstreamCount)
			s.metrics.SetBufferUtilization(s.eventBuffer.Utilization())

		case <-s.ctx.Done():
			return
		}
	}
}

func (s *Server) shutdown(ctx context.Context) {
	s.cancel()

	// Close all active handlers
	s.activeHandlersMu.Lock()
	for _, handler := range s.activeHandlers {
		handler.Close()
	}
	s.activeHandlersMu.Unlock()

	// Shutdown upstream pool
	s.upstreamPool.Shutdown(ctx)

	// Shutdown HTTP servers
	shutdownCtx, cancel := context.WithTimeout(ctx, s.config.ShutdownTimeout)
	defer cancel()

	if err := s.httpServer.Shutdown(shutdownCtx); err != nil {
		log.Printf("HTTP server shutdown error: %v", err)
	}

	if s.metricsServer != nil {
		if err := s.metricsServer.Shutdown(shutdownCtx); err != nil {
			log.Printf("Metrics server shutdown error: %v", err)
		}
	}

	log.Println("Server shutdown complete")
}

func (s *Server) generateClientID() int64 {
	return time.Now().UnixNano()
}
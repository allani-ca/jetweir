# Jetweir

A lightweight, self-hosted Go service that multiplexes [Bluesky Jetstream v2](https://github.com/bluesky-social/jetstream) WebSocket subscriptions. LAN applications connect to a single Jetweir endpoint using official Jetstream-compatible filters. Identical subscriptions automatically share upstream connections.

## Features

- **Subscription Pooling**: Multiple clients with identical subscriptions share one upstream connection
- **Jetstream v2 Compatible**: Supports official Jetstream API and parameters
- **Automatic Reconnection**: Exponential backoff reconnection with configurable limits
- **Event Buffering**: In-memory rolling buffer for event replay and cursor handling
- **Independent Consumers**: Each downstream client has its own cursor; backpressure doesn't affect others
- **Prometheus Metrics**: Built-in observability for connection counts, event throughput, and buffer utilization
- **Lightweight**: ~1 vCPU, 512 MB–1 GB RAM, 4 GB storage
- **LAN Optimized**: Designed for local network deployments

## Quick Start

### Prerequisites

- Go 1.23 or later
- Linux/macOS/Windows

### Installation

```bash
git clone https://github.com/allani-ca/jetweir.git
cd jetweir
go mod download
go build -o jetweir ./cmd/jetweir
```

### Running the Service

```bash
# Use defaults (connects to public Jetstream, listens on 127.0.0.1:8080)
./jetweir

# Or configure via environment variables
export JETWEIR_LISTEN_ADDR=0.0.0.0
export JETWEIR_LISTEN_PORT=8080
export JETWEIR_UPSTREAM_URL=wss://jetstream2.us-west.firehose.bluesky.net/subscribe
export JETWEIR_BUFFER_SIZE=10000
./jetweir
```

## API

### Subscribe Endpoint

Connect a WebSocket client to either:
- `/subscribe`
- `/xrpc/network.bsky.jetstream.subscribeEvents` (official Jetstream compatibility)

### Query Parameters

All parameters are optional and match [Jetstream v2 specification](https://github.com/bluesky-social/jetstream):

| Parameter | Type | Default | Description |
|-----------|------|---------|-------------|
| `wantedCollections` | string | (none) | Comma-separated list of collections (e.g., `app.bsky.feed.post,app.bsky.feed.like`) |
| `wantedDids` | string | (none) | Comma-separated list of DIDs to monitor |
| `cursor` | int64 | -1 | Starting cursor position (-1 = latest) |
| `maxMessageSizeBytes` | int | 102400 | Maximum event size in bytes |
| `compress` | bool | false | Enable compression (future) |

### Example

```bash
# Subscribe to all posts
wscat -c 'ws://localhost:8080/subscribe?wantedCollections=app.bsky.feed.post'

# Subscribe to specific DID
wscat -c 'ws://localhost:8080/subscribe?wantedDids=did:plc:xxx'

# Replay events from cursor position
wscat -c 'ws://localhost:8080/subscribe?cursor=12345'
```

## Admin Endpoints

### Health Check

```bash
curl http://localhost:8080/health
# {"status":"healthy","time":1700000000}
```

### Status Summary

```bash
curl http://localhost:8080/status
```

Returns:
```json
{
  "active_upstream_connections": 2,
  "connected_downstream_clients": 5,
  "active_subscriptions": 2,
  "subscription_reuse_total": 5,
  "buffer_size": 1234,
  "buffer_utilization": 0.123,
  "timestamp": 1700000000
}
```

### Prometheus Metrics

```bash
curl http://localhost:9090/metrics
```

## Configuration

Configure via environment variables:

```bash
# Server
JETWEIR_LISTEN_ADDR=127.0.0.1          # Default: 127.0.0.1
JETWEIR_LISTEN_PORT=8080               # Default: 8080
JETWEIR_UPSTREAM_URL=wss://...         # Jetstream endpoint

# Timeouts
JETWEIR_READ_TIMEOUT=30s               # Default: 30s
JETWEIR_WRITE_TIMEOUT=30s              # Default: 30s
JETWEIR_SHUTDOWN_TIMEOUT=10s           # Default: 10s

# Buffering
JETWEIR_BUFFER_SIZE=10000              # Default: 10000
JETWEIR_BUFFER_TTL=5m                  # Default: 5m
JETWEIR_DOWNSTREAM_BUFFER=100          # Default: 100
JETWEIR_SLOW_CONSUMER_TIMEOUT=5s       # Default: 5s

# Reconnection
JETWEIR_RECONNECT_BASE=1s              # Default: 1s
JETWEIR_RECONNECT_MAX=30s              # Default: 30s

# Metrics
JETWEIR_METRICS_ENABLED=true           # Default: true
JETWEIR_METRICS_PORT=9090              # Default: 9090
```

## Example Consumer

A Prometheus-instrumented example consumer is included in `examples/consumer/`.

### Running the Example

```bash
# Terminal 1: Start Jetweir
./jetweir

# Terminal 2: Run the consumer
cd examples/consumer
go run main.go \
  -jetweir=ws://localhost:8080/subscribe \
  -listen=:8081 \
  -collections=app.bsky.feed.post
```

The consumer exposes metrics at `http://localhost:8081/metrics`:
- `jetweir_consumer_events_received_total`: Total events processed
- `jetweir_consumer_events_skipped_total`: Skipped events (errors, non-matching)
- `jetweir_consumer_connection_status`: Connection state (1 = connected, 0 = disconnected)
- `jetweir_consumer_last_event_timestamp`: Last event reception time

## Architecture

### Components

```
Jetweir Service
├── Downstream Handlers (one per client)
├── Subscription Manager (deduplication)
├── Upstream Connection Pool
├── Event Buffer (rolling window)
└── Metrics (Prometheus)
```

### Subscription Flow

1. Client connects with filters (e.g., `wantedCollections=app.bsky.feed.post`)
2. Subscription Manager checks if identical subscription exists
3. If yes, client joins existing upstream connection (subscription reuse)
4. If no, new upstream connection created
5. Events from upstream flow to all matching downstream clients independently
6. Slow clients are disconnected; fast clients aren't affected

### Event Delivery

- **At-least-once**: Events are delivered at least once to each client
- **Independent buffering**: Each client has its own buffer; disconnection doesn't affect others
- **Cursor-based replay**: Clients can replay from cursor position using in-memory buffer
- **Backpressure isolation**: Slow clients disconnected after timeout; others continue

## Deployment

### Docker

```dockerfile
FROM golang:1.23-alpine AS builder
WORKDIR /app
COPY . .
RUN go build -o jetweir ./

FROM alpine:latest
RUN apk add --no-cache ca-certificates
COPY --from=builder /app/jetweir /usr/local/bin/
EXPOSE 8080 9090
CMD ["jetweir"]
```

### Kubernetes

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: jetweir
spec:
  replicas: 1
  selector:
    matchLabels:
      app: jetweir
  template:
    metadata:
      labels:
        app: jetweir
    spec:
      containers:
      - name: jetweir
        image: jetweir:latest
        ports:
        - containerPort: 8080
        - containerPort: 9090
        env:
        - name: JETWEIR_LISTEN_ADDR
          value: "0.0.0.0"
        - name: JETWEIR_UPSTREAM_URL
          value: "wss://jetstream2.us-west.firehose.bluesky.net/subscribe"
        livenessProbe:
          httpGet:
            path: /health
            port: 8080
          initialDelaySeconds: 5
          periodSeconds: 10
        readinessProbe:
          httpGet:
            path: /health
            port: 8080
          initialDelaySeconds: 5
          periodSeconds: 10
        resources:
          requests:
            memory: "512Mi"
            cpu: "500m"
          limits:
            memory: "1Gi"
            cpu: "1000m"
---
apiVersion: v1
kind: Service
metadata:
  name: jetweir
spec:
  ports:
  - name: api
    port: 8080
    targetPort: 8080
  - name: metrics
    port: 9090
    targetPort: 9090
  selector:
    app: jetweir
  type: ClusterIP
```

### LXC

```bash
# Create container
lxc launch ubuntu:22.04 jetweir

# Install dependencies
lxc exec jetweir -- apt-get update
lxc exec jetweir -- apt-get install -y golang-1.21

# Copy and build
lxc file push jetweir jetweir/root/
lxc exec jetweir -- bash -c 'cd /root/jetweir && /usr/lib/go-1.21/bin/go build -o jetweir'

# Run
lxc exec jetweir -- /root/jetweir/jetweir
```

## Metrics Reference

### Gauge Metrics

- `jetweir_active_upstream_connections`: Number of active upstream WebSocket connections
- `jetweir_connected_downstream_clients`: Number of connected downstream clients
- `jetweir_active_subscriptions`: Number of unique subscriptions
- `jetweir_buffer_utilization_ratio`: Event buffer utilization (0-1)
- `jetweir_connection_pool_size`: Size of the upstream connection pool

### Counter Metrics

- `jetweir_upstream_reconnects_total`: Total upstream reconnection attempts
- `jetweir_upstream_connection_errors_total`: Total upstream connection errors
- `jetweir_downstream_disconnects_total`: Total downstream client disconnections
- `jetweir_events_received_total`: Total events received from upstream
- `jetweir_events_distributed_total`: Total events sent to downstream clients
- `jetweir_events_dropped_total`: Total events dropped (slow consumers)
- `jetweir_buffer_evictions_total`: Total events evicted from buffer

### Histogram Metrics

- `jetweir_event_processing_seconds`: Event processing latency distribution
- `jetweir_subscription_reuse_ratio`: Clients per upstream subscription
- `jetweir_consumer_lag_events`: Event lag distribution
- `jetweir_consumer_connection_errors_total`: Consumer connection error count

## Limitations & Constraints

- **No persistent archive**: Events only buffered in memory (configurable TTL)
- **No database**: All data ephemeral; suitable for event streaming only
- **No external broker**: Jetweir is standalone; use multiple instances for HA
- **Cursor semantics**: Cursors are instance-specific and may not work across restarts
- **Regional failover**: Cursors invalid on regional Jetstream failover; use timestamp-based recovery
- **At-least-once delivery**: Duplicate handling is application-level responsibility

## Performance Characteristics

- **Memory**: ~50-100 MB baseline + event buffer (10K events ≈ 10-50 MB depending on size)
- **CPU**: <10% per 100 concurrent clients (varies by event rate)
- **Throughput**: Tested with 10K+ events/second per upstream connection
- **Latency**: <100ms event propagation under normal load

## Development

### Running Tests

```bash
go test ./...
```

### Code Structure

```
jetweir/
├── config/           # Configuration management
├── pkg/
│   ├── types/        # Core data types
│   ├── protocol/     # Jetstream protocol handling
│   ├── buffer/       # Event buffer implementation
│   ├── subscription/ # Subscription pooling logic
│   ├── upstream/     # Upstream connection management
│   ├── downstream/   # Downstream client handlers
│   └── metrics/      # Prometheus metrics
├── examples/
│   └── consumer/     # Example Prometheus consumer
└── main.go           # Main application
```

## Troubleshooting

### High Memory Usage

Reduce buffer size:
```bash
JETWEIR_BUFFER_SIZE=5000 ./jetweir
```

### Frequent Disconnections

Increase timeouts:
```bash
JETWEIR_READ_TIMEOUT=60s JETWEIR_SLOW_CONSUMER_TIMEOUT=10s ./jetweir
```

### Reconnection Loops

Check upstream URL and network connectivity:
```bash
# Test upstream endpoint
curl -i -N -H "Connection: Upgrade" -H "Upgrade: websocket" \
  wss://jetstream2.us-west.firehose.bluesky.net/subscribe
```

### High CPU Usage

Reduce event buffer size or disconnect slow consumers:
```bash
JETWEIR_BUFFER_SIZE=5000 JETWEIR_SLOW_CONSUMER_TIMEOUT=3s ./jetweir
```

## License

This project is provided as-is for public use. Consult the official [Bluesky Jetstream documentation](https://github.com/bluesky-social/jetstream) for protocol details and usage restrictions.

## Support

For issues and questions:
1. Check this README
2. Review the example consumer
3. Enable debug logging: `RUST_LOG=debug`
4. Open an issue on GitHub

## Contributing

Contributions welcome! Please ensure:
- Code passes `go fmt` and `go vet`
- Tests pass: `go test ./...`
- Changes are well-documented
- Commits follow conventional commit style

---

**Jetweir** — Bluesky Jetstream multiplexing for local networks.

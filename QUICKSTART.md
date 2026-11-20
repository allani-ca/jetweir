# Jetweir Quick Start Guide

This guide gets you up and running with Jetweir in minutes.

## Prerequisites

- Go 1.23+ (or use Docker)
- `git` and `curl` for testing
- A terminal/command prompt

## Option 1: Native Go

### Build

```bash
# Clone or download the project
cd jetweir

# Download dependencies
go mod download

# Build
go build -o jetweir ./
```

### Run

```bash
# Default configuration (connects to us-west Jetstream instance)
./jetweir

# Custom upstream (e.g., us-east instance)
export JETWEIR_UPSTREAM_URL=wss://jetstream2.us-east.firehose.bluesky.net/subscribe
./jetweir

# Listen on all interfaces (for LAN access)
export JETWEIR_LISTEN_ADDR=0.0.0.0
./jetweir
```

The service starts on `127.0.0.1:8080` by default.

### Test the Service

In another terminal:

```bash
# Health check
curl http://localhost:8080/health

# Status endpoint
curl http://localhost:8080/status

# Prometheus metrics
curl http://localhost:9090/metrics
```

## Option 2: Docker

### Build & Run

```bash
# Build the image
docker build -t jetweir:latest .

# Run the container
docker run -d \
  --name jetweir \
  -p 8080:8080 \
  -p 9090:9090 \
  -e JETWEIR_LISTEN_ADDR=0.0.0.0 \
  jetweir:latest
```

### View Logs

```bash
docker logs -f jetweir
```

## Option 3: Docker Compose

```bash
# Start Jetweir + Prometheus
docker-compose up -d

# View logs
docker-compose logs -f jetweir

# Stop
docker-compose down
```

Prometheus will be available at `http://localhost:9091`.

## Connect a Client

### Using WebSocket (wscat)

```bash
# Install wscat (if not already installed)
npm install -g wscat

# Subscribe to all events
wscat -c ws://localhost:8080/subscribe

# Subscribe to posts only
wscat -c 'ws://localhost:8080/subscribe?wantedCollections=app.bsky.feed.post'

# Subscribe to specific DID
wscat -c 'ws://localhost:8080/subscribe?wantedDids=did:plc:xxx'
```

### Using the Example Consumer

```bash
# Terminal 1: Start Jetweir (if not already running)
./jetweir

# Terminal 2: Run the example consumer
cd examples/consumer
go run main.go \
  -jetweir=ws://localhost:8080/subscribe \
  -listen=:8081 \
  -collections=app.bsky.feed.post

# Terminal 3: Check consumer metrics
curl http://localhost:8081/metrics | grep jetweir_consumer
```

The consumer exposes metrics at `:8081/metrics`.

## Configuration

All configuration is via environment variables. Common ones:

```bash
# Server
JETWEIR_LISTEN_ADDR=0.0.0.0           # Listen address
JETWEIR_LISTEN_PORT=8080              # HTTP port
JETWEIR_UPSTREAM_URL=wss://...        # Jetstream endpoint

# Buffering
JETWEIR_BUFFER_SIZE=10000             # Event buffer size
JETWEIR_BUFFER_TTL=5m                 # Event buffer TTL

# Backpressure
JETWEIR_SLOW_CONSUMER_TIMEOUT=5s      # Disconnect slow clients after this

# Metrics
JETWEIR_METRICS_ENABLED=true          # Enable Prometheus metrics
JETWEIR_METRICS_PORT=9090             # Metrics port
```

See [README.md](README.md) for all configuration options.

## Monitoring

### Health Endpoint

```bash
# Returns healthy status
curl http://localhost:8080/health
```

### Status Summary

```bash
# Returns active connections, subscriptions, buffer stats
curl http://localhost:8080/status
```

### Prometheus Metrics

```bash
# Full metrics
curl http://localhost:9090/metrics

# Useful queries for Prometheus:
# - jetweir_active_upstream_connections
# - jetweir_connected_downstream_clients
# - jetweir_events_received_total
# - jetweir_events_distributed_total
# - jetweir_buffer_utilization_ratio
```

## Troubleshooting

### Service won't start

```bash
# Check if port is in use
# macOS/Linux:
lsof -i :8080

# Windows:
netstat -ano | findstr :8080
```

### High memory usage

```bash
# Reduce buffer size
JETWEIR_BUFFER_SIZE=5000 ./jetweir
```

### Frequent disconnections

```bash
# Increase timeouts
JETWEIR_READ_TIMEOUT=60s JETWEIR_SLOW_CONSUMER_TIMEOUT=10s ./jetweir
```

### Check logs

```bash
# Docker
docker logs jetweir

# Docker Compose
docker-compose logs jetweir

# Native (stdout)
./jetweir 2>&1 | tee jetweir.log
```

## Next Steps

1. **Review the [README.md](README.md)** for detailed documentation
2. **Explore the example consumer** in `examples/consumer/main.go`
3. **Set up Prometheus monitoring** using `docker-compose.yml`
4. **Deploy to production** using Docker or Kubernetes manifests from README

## Common Patterns

### Subscribe to multiple collections

```bash
wscat -c 'ws://localhost:8080/subscribe?wantedCollections=app.bsky.feed.post,app.bsky.feed.like'
```

### Replay events from a cursor

```bash
# Start from cursor position 12345
wscat -c 'ws://localhost:8080/subscribe?cursor=12345'

# -1 means latest events
wscat -c 'ws://localhost:8080/subscribe?cursor=-1'
```

### Monitor subscription reuse

```bash
# Check how many clients share subscriptions
curl http://localhost:8080/status | grep subscription_reuse_total
```

## Support

- 📖 Read the [README.md](README.md)
- 🔍 Check the example consumer implementation
- 🐛 Enable debug logging (check service logs)
- 🌐 Test upstream connectivity (documented in README)

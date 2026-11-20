package metrics

import (
	"sync"
	"sync/atomic"

	"github.com/prometheus/client_golang/prometheus"
)

// Metrics holds all Prometheus metrics for the service.
type Metrics struct {
	// Upstream connection metrics
	activeUpstreamConnections prometheus.Gauge
	upstreamReconnects        prometheus.Counter
	upstreamConnectionErrors  prometheus.Counter

	// Downstream connection metrics
	connectedDownstreamClients prometheus.Gauge
	downstreamDisconnects      prometheus.Counter

	// Event metrics
	eventsReceived      prometheus.Counter
	eventsDistributed   prometheus.Counter
	eventsDropped       prometheus.Counter
	eventProcessingTime prometheus.Histogram

	// Subscription metrics
	activeSubscriptions prometheus.Gauge
	subscriptionReuse   prometheus.Histogram

	// Buffer metrics
	bufferUtilization prometheus.Gauge
	bufferEvictions   prometheus.Counter

	// Consumer lag metrics
	consumerLag prometheus.Histogram

	// Connection pool metrics
	connectionPoolSize prometheus.Gauge

	// Internal counters (for thread-safe increments)
	eventsReceivedCounter      int64
	eventsDistributedCounter   int64
	eventsDroppedCounter       int64
	upstreamReconnectsCounter  int64
	downstreamDisconnectsCounter int64
}

var instance *Metrics
var once sync.Once

// New creates or returns the singleton metrics instance.
func New(registerer prometheus.Registerer) *Metrics {
	once.Do(func() {
		instance = &Metrics{
			activeUpstreamConnections: prometheus.NewGauge(prometheus.GaugeOpts{
				Name: "jetweir_active_upstream_connections",
				Help: "Number of active upstream WebSocket connections",
			}),
			upstreamReconnects: prometheus.NewCounter(prometheus.CounterOpts{
				Name: "jetweir_upstream_reconnects_total",
				Help: "Total number of upstream connection reconnects",
			}),
			upstreamConnectionErrors: prometheus.NewCounter(prometheus.CounterOpts{
				Name: "jetweir_upstream_connection_errors_total",
				Help: "Total number of upstream connection errors",
			}),
			connectedDownstreamClients: prometheus.NewGauge(prometheus.GaugeOpts{
				Name: "jetweir_connected_downstream_clients",
				Help: "Number of connected downstream clients",
			}),
			downstreamDisconnects: prometheus.NewCounter(prometheus.CounterOpts{
				Name: "jetweir_downstream_disconnects_total",
				Help: "Total number of downstream client disconnections",
			}),
			eventsReceived: prometheus.NewCounter(prometheus.CounterOpts{
				Name: "jetweir_events_received_total",
				Help: "Total number of events received from upstream",
			}),
			eventsDistributed: prometheus.NewCounter(prometheus.CounterOpts{
				Name: "jetweir_events_distributed_total",
				Help: "Total number of events distributed to downstream clients",
			}),
			eventsDropped: prometheus.NewCounter(prometheus.CounterOpts{
				Name: "jetweir_events_dropped_total",
				Help: "Total number of events dropped due to slow consumers",
			}),
			eventProcessingTime: prometheus.NewHistogram(prometheus.HistogramOpts{
				Name:    "jetweir_event_processing_seconds",
				Help:    "Event processing time in seconds",
				Buckets: prometheus.DefBuckets,
			}),
			activeSubscriptions: prometheus.NewGauge(prometheus.GaugeOpts{
				Name: "jetweir_active_subscriptions",
				Help: "Number of active unique subscriptions",
			}),
			subscriptionReuse: prometheus.NewHistogram(prometheus.HistogramOpts{
				Name:    "jetweir_subscription_reuse_ratio",
				Help:    "Subscription reuse ratio (downstream clients per upstream subscription)",
				Buckets: []float64{1, 2, 5, 10, 25, 50, 100},
			}),
			bufferUtilization: prometheus.NewGauge(prometheus.GaugeOpts{
				Name: "jetweir_buffer_utilization_ratio",
				Help: "Event buffer utilization as a fraction (0-1)",
			}),
			bufferEvictions: prometheus.NewCounter(prometheus.CounterOpts{
				Name: "jetweir_buffer_evictions_total",
				Help: "Total number of events evicted from buffer due to size limit",
			}),
			consumerLag: prometheus.NewHistogram(prometheus.HistogramOpts{
				Name:    "jetweir_consumer_lag_events",
				Help:    "Consumer lag in number of events",
				Buckets: []float64{1, 10, 100, 1000, 10000},
			}),
			connectionPoolSize: prometheus.NewGauge(prometheus.GaugeOpts{
				Name: "jetweir_connection_pool_size",
				Help: "Size of the upstream connection pool",
			}),
		}

		if registerer != nil {
			registerer.MustRegister(
				instance.activeUpstreamConnections,
				instance.upstreamReconnects,
				instance.upstreamConnectionErrors,
				instance.connectedDownstreamClients,
				instance.downstreamDisconnects,
				instance.eventsReceived,
				instance.eventsDistributed,
				instance.eventsDropped,
				instance.eventProcessingTime,
				instance.activeSubscriptions,
				instance.subscriptionReuse,
				instance.bufferUtilization,
				instance.bufferEvictions,
				instance.consumerLag,
				instance.connectionPoolSize,
			)
		}
	})
	return instance
}

// SetActiveUpstreamConnections sets the number of active upstream connections.
func (m *Metrics) SetActiveUpstreamConnections(count int) {
	m.activeUpstreamConnections.Set(float64(count))
}

// IncUpstreamReconnects increments the upstream reconnects counter.
func (m *Metrics) IncUpstreamReconnects() {
	m.upstreamReconnects.Inc()
	atomic.AddInt64(&m.upstreamReconnectsCounter, 1)
}

// IncUpstreamConnectionErrors increments the upstream connection errors counter.
func (m *Metrics) IncUpstreamConnectionErrors() {
	m.upstreamConnectionErrors.Inc()
}

// SetConnectedDownstreamClients sets the number of connected downstream clients.
func (m *Metrics) SetConnectedDownstreamClients(count int) {
	m.connectedDownstreamClients.Set(float64(count))
}

// IncDownstreamDisconnects increments the downstream disconnects counter.
func (m *Metrics) IncDownstreamDisconnects() {
	m.downstreamDisconnects.Inc()
	atomic.AddInt64(&m.downstreamDisconnectsCounter, 1)
}

// IncEventsReceived increments the events received counter.
func (m *Metrics) IncEventsReceived() {
	m.eventsReceived.Inc()
	atomic.AddInt64(&m.eventsReceivedCounter, 1)
}

// IncEventsDistributed increments the events distributed counter.
func (m *Metrics) IncEventsDistributed() {
	m.eventsDistributed.Inc()
	atomic.AddInt64(&m.eventsDistributedCounter, 1)
}

// IncEventsDropped increments the events dropped counter.
func (m *Metrics) IncEventsDropped() {
	m.eventsDropped.Inc()
	atomic.AddInt64(&m.eventsDroppedCounter, 1)
}

// ObserveEventProcessingTime observes event processing time in seconds.
func (m *Metrics) ObserveEventProcessingTime(seconds float64) {
	m.eventProcessingTime.Observe(seconds)
}

// SetActiveSubscriptions sets the number of active subscriptions.
func (m *Metrics) SetActiveSubscriptions(count int) {
	m.activeSubscriptions.Set(float64(count))
}

// ObserveSubscriptionReuse observes the reuse ratio.
func (m *Metrics) ObserveSubscriptionReuse(ratio float64) {
	m.subscriptionReuse.Observe(ratio)
}

// SetBufferUtilization sets the buffer utilization ratio.
func (m *Metrics) SetBufferUtilization(ratio float64) {
	m.bufferUtilization.Set(ratio)
}

// IncBufferEvictions increments the buffer evictions counter.
func (m *Metrics) IncBufferEvictions() {
	m.bufferEvictions.Inc()
}

// ObserveConsumerLag observes consumer lag in events.
func (m *Metrics) ObserveConsumerLag(lag float64) {
	m.consumerLag.Observe(lag)
}

// SetConnectionPoolSize sets the connection pool size.
func (m *Metrics) SetConnectionPoolSize(size int) {
	m.connectionPoolSize.Set(float64(size))
}

// GetCounters returns current counter values.
func (m *Metrics) GetCounters() (eventsRx, eventsTx, eventsDropped, reconnects, disconnects int64) {
	eventsRx = atomic.LoadInt64(&m.eventsReceivedCounter)
	eventsTx = atomic.LoadInt64(&m.eventsDistributedCounter)
	eventsDropped = atomic.LoadInt64(&m.eventsDroppedCounter)
	reconnects = atomic.LoadInt64(&m.upstreamReconnectsCounter)
	disconnects = atomic.LoadInt64(&m.downstreamDisconnectsCounter)
	return
}

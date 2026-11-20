package config

import (
	"os"
	"strconv"
	"time"
)

type Config struct {
	// Server configuration
	ListenAddr              string
	ListenPort              int
	UpstreamURL             string
	ReadTimeout             time.Duration
	WriteTimeout            time.Duration
	ShutdownTimeout         time.Duration
	
	// Subscription configuration
	MaxMessageSizeBytes     int
	
	// Buffer and replay configuration
	EventBufferSize         int
	EventBufferTTL          time.Duration
	
	// Backpressure configuration
	DownstreamBufferSize    int
	SlowConsumerTimeout     time.Duration
	
	// Reconnection configuration
	ReconnectBaseBackoff    time.Duration
	ReconnectMaxBackoff     time.Duration
	
	// Metrics configuration
	EnableMetrics           bool
	MetricsPort             int
}

func Load() *Config {
	return &Config{
		ListenAddr:          getEnv("JETWEIR_LISTEN_ADDR", "127.0.0.1"),
		ListenPort:          getEnvInt("JETWEIR_LISTEN_PORT", 8080),
		UpstreamURL:         getEnv("JETWEIR_UPSTREAM_URL", "wss://jetstream2.us-west.firehose.bluesky.net/subscribe"),
		ReadTimeout:         getEnvDuration("JETWEIR_READ_TIMEOUT", 30*time.Second),
		WriteTimeout:        getEnvDuration("JETWEIR_WRITE_TIMEOUT", 30*time.Second),
		ShutdownTimeout:     getEnvDuration("JETWEIR_SHUTDOWN_TIMEOUT", 10*time.Second),
		
		MaxMessageSizeBytes: getEnvInt("JETWEIR_MAX_MESSAGE_SIZE", 100*1024), // 100 KB default
		
		EventBufferSize:     getEnvInt("JETWEIR_BUFFER_SIZE", 10000),
		EventBufferTTL:      getEnvDuration("JETWEIR_BUFFER_TTL", 5*time.Minute),
		
		DownstreamBufferSize: getEnvInt("JETWEIR_DOWNSTREAM_BUFFER", 100),
		SlowConsumerTimeout:  getEnvDuration("JETWEIR_SLOW_CONSUMER_TIMEOUT", 5*time.Second),
		
		ReconnectBaseBackoff: getEnvDuration("JETWEIR_RECONNECT_BASE", 1*time.Second),
		ReconnectMaxBackoff:  getEnvDuration("JETWEIR_RECONNECT_MAX", 30*time.Second),
		
		EnableMetrics:        getEnvBool("JETWEIR_METRICS_ENABLED", true),
		MetricsPort:          getEnvInt("JETWEIR_METRICS_PORT", 9090),
	}
}

func getEnv(key, defaultVal string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return defaultVal
}

func getEnvInt(key string, defaultVal int) int {
	if val := os.Getenv(key); val != "" {
		if i, err := strconv.Atoi(val); err == nil {
			return i
		}
	}
	return defaultVal
}

func getEnvBool(key string, defaultVal bool) bool {
	if val := os.Getenv(key); val != "" {
		return val == "true" || val == "1" || val == "yes"
	}
	return defaultVal
}

func getEnvDuration(key string, defaultVal time.Duration) time.Duration {
	if val := os.Getenv(key); val != "" {
		if d, err := time.ParseDuration(val); err == nil {
			return d
		}
	}
	return defaultVal
}

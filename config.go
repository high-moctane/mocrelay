package mocrelay

import (
	"fmt"
	"os"
	"strconv"
	"time"

	"gopkg.in/yaml.v3"
)

// Config holds all configuration loaded from NIP-11 YAML + environment variables
type Config struct {
	// Server settings
	ServerAddr         string
	ServerSendTimeout  time.Duration
	ServerPingDuration time.Duration

	// Relay settings
	RelayMaxMessageLength   int64
	RelayRecvRateLimitRate  float64
	RelayRecvRateLimitBurst int

	// Handler settings
	CacheSize  int
	RouterSize int

	// SQLite plugin settings
	SQLiteEnabled            bool
	SQLitePath               string
	SQLiteBulkInsertNum      int
	SQLiteBulkInsertDuration time.Duration
	SQLiteMaxLimit           int

	// Prometheus plugin settings
	PrometheusEnabled bool

	// NIP-11
	NIP11 *NIP11
}

// LoadConfig loads configuration from:
// 1. Default values
// 2. NIP-11 YAML file (if exists)
// 3. Environment variables (override)
func LoadConfig() (*Config, error) {
	// Start with defaults
	config := &Config{
		// Server settings
		ServerAddr:         getEnv("MOCRELAY_SERVER_ADDR", "0.0.0.0:8234"),
		ServerSendTimeout:  getEnvDuration("MOCRELAY_SERVER_SEND_TIMEOUT", 10*time.Second),
		ServerPingDuration: getEnvDuration("MOCRELAY_SERVER_PING_DURATION", 1*time.Minute),

		// Relay settings
		RelayMaxMessageLength:   getEnvInt64("MOCRELAY_RELAY_MAX_MESSAGE_LENGTH", 100000),
		RelayRecvRateLimitRate:  getEnvFloat64("MOCRELAY_RELAY_RECV_RATE_LIMIT_RATE", 10.0),
		RelayRecvRateLimitBurst: getEnvInt("MOCRELAY_RELAY_RECV_RATE_LIMIT_BURST", 10),

		// Handler settings
		CacheSize:  getEnvInt("MOCRELAY_CACHE_SIZE", 100),
		RouterSize: getEnvInt("MOCRELAY_ROUTER_SIZE", 100),

		// SQLite plugin settings
		SQLiteEnabled:            getEnvBool("MOCRELAY_SQLITE_ENABLED", true),
		SQLitePath:               getEnv("MOCRELAY_SQLITE_PATH", "./mocrelay.db"),
		SQLiteBulkInsertNum:      getEnvInt("MOCRELAY_SQLITE_BULK_INSERT_NUM", 1000),
		SQLiteBulkInsertDuration: getEnvDuration("MOCRELAY_SQLITE_BULK_INSERT_DURATION", 2*time.Minute),
		SQLiteMaxLimit:           getEnvInt("MOCRELAY_SQLITE_MAX_LIMIT", 5000),

		// Prometheus plugin settings
		PrometheusEnabled: getEnvBool("MOCRELAY_PROMETHEUS_ENABLED", true),

		// Default NIP-11
		NIP11: &NIP11{
			Name:          "mocrelay",
			Description:   "moctane's nostr relay",
			Software:      "https://github.com/high-moctane/mocrelay",
			Version:       "0.1.0",
			SupportedNIPs: []int{1, 11, 42, 45},
		},
	}

	// Load NIP-11 from YAML file if exists
	nip11File := getEnv("MOCRELAY_NIP11_FILE", "./nip11.yaml")
	if _, err := os.Stat(nip11File); err == nil {
		nip11, err := LoadNIP11FromYAML(nip11File)
		if err != nil {
			return nil, fmt.Errorf("failed to load NIP-11 from YAML: %w", err)
		}
		config.NIP11 = nip11
	}

	// Override NIP-11 limitation values from environment variables
	if config.NIP11.Limitation == nil {
		config.NIP11.Limitation = &NIP11Limitation{}
	}

	// Apply environment variable overrides for limitations
	if envVal := os.Getenv("MOCRELAY_LIMITATION_MAX_MESSAGE_LENGTH"); envVal != "" {
		if val, err := strconv.Atoi(envVal); err == nil {
			config.NIP11.Limitation.MaxMessageLength = val
		}
	}
	if envVal := os.Getenv("MOCRELAY_LIMITATION_MAX_SUBSCRIPTIONS"); envVal != "" {
		if val, err := strconv.Atoi(envVal); err == nil {
			config.NIP11.Limitation.MaxSubscriptions = val
		}
	}
	if envVal := os.Getenv("MOCRELAY_LIMITATION_MAX_FILTERS"); envVal != "" {
		if val, err := strconv.Atoi(envVal); err == nil {
			config.NIP11.Limitation.MaxFilters = val
		}
	}
	if envVal := os.Getenv("MOCRELAY_LIMITATION_MAX_LIMIT"); envVal != "" {
		if val, err := strconv.Atoi(envVal); err == nil {
			config.NIP11.Limitation.MaxLimit = val
		}
	}
	if envVal := os.Getenv("MOCRELAY_LIMITATION_MAX_SUBID_LENGTH"); envVal != "" {
		if val, err := strconv.Atoi(envVal); err == nil {
			config.NIP11.Limitation.MaxSubIDLength = val
		}
	}
	if envVal := os.Getenv("MOCRELAY_LIMITATION_MAX_EVENT_TAGS"); envVal != "" {
		if val, err := strconv.Atoi(envVal); err == nil {
			config.NIP11.Limitation.MaxEventTags = val
		}
	}
	if envVal := os.Getenv("MOCRELAY_LIMITATION_MAX_CONTENT_LENGTH"); envVal != "" {
		if val, err := strconv.Atoi(envVal); err == nil {
			config.NIP11.Limitation.MaxContentLength = val
		}
	}
	if envVal := os.Getenv("MOCRELAY_LIMITATION_MIN_POW_DIFFICULTY"); envVal != "" {
		if val, err := strconv.Atoi(envVal); err == nil {
			config.NIP11.Limitation.MinPoWDifficulty = val
		}
	}
	if envVal := os.Getenv("MOCRELAY_LIMITATION_AUTH_REQUIRED"); envVal != "" {
		if val, err := strconv.ParseBool(envVal); err == nil {
			config.NIP11.Limitation.AuthRequired = val
		}
	}
	if envVal := os.Getenv("MOCRELAY_LIMITATION_PAYMENT_REQUIRED"); envVal != "" {
		if val, err := strconv.ParseBool(envVal); err == nil {
			config.NIP11.Limitation.PaymentRequired = val
		}
	}
	if envVal := os.Getenv("MOCRELAY_LIMITATION_CREATED_AT_LOWER_LIMIT"); envVal != "" {
		if val, err := strconv.ParseInt(envVal, 10, 64); err == nil {
			config.NIP11.Limitation.CreatedAtLowerLimit = val
		}
	}
	if envVal := os.Getenv("MOCRELAY_LIMITATION_CREATED_AT_UPPER_LIMIT"); envVal != "" {
		if val, err := strconv.ParseInt(envVal, 10, 64); err == nil {
			config.NIP11.Limitation.CreatedAtUpperLimit = val
		}
	}

	return config, nil
}

// LoadNIP11FromYAML loads NIP-11 information from YAML file
func LoadNIP11FromYAML(path string) (*NIP11, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read NIP-11 YAML file: %w", err)
	}

	var nip11 NIP11
	if err := yaml.Unmarshal(data, &nip11); err != nil {
		return nil, fmt.Errorf("failed to unmarshal NIP-11 YAML: %w", err)
	}

	return &nip11, nil
}

// Helper functions to get environment variables with defaults

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getEnvInt(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if i, err := strconv.Atoi(value); err == nil {
			return i
		}
	}
	return defaultValue
}

func getEnvInt64(key string, defaultValue int64) int64 {
	if value := os.Getenv(key); value != "" {
		if i, err := strconv.ParseInt(value, 10, 64); err == nil {
			return i
		}
	}
	return defaultValue
}

func getEnvFloat64(key string, defaultValue float64) float64 {
	if value := os.Getenv(key); value != "" {
		if f, err := strconv.ParseFloat(value, 64); err == nil {
			return f
		}
	}
	return defaultValue
}

func getEnvBool(key string, defaultValue bool) bool {
	if value := os.Getenv(key); value != "" {
		if b, err := strconv.ParseBool(value); err == nil {
			return b
		}
	}
	return defaultValue
}

func getEnvDuration(key string, defaultValue time.Duration) time.Duration {
	if value := os.Getenv(key); value != "" {
		if d, err := time.ParseDuration(value); err == nil {
			return d
		}
	}
	return defaultValue
}

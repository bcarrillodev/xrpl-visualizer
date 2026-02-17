package config

import (
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
)

type Config struct {
	// Rippled Configuration
	RippledJSONRPCURL   string
	RippledWebSocketURL string
	Network             string

	// Server Configuration
	ListenPort         int
	ListenAddr         string
	CORSAllowedOrigins []string

	// Validator Fetcher Configuration
	ValidatorRefreshInterval      int // seconds
	ValidatorListSites            []string
	SecondaryValidatorRegistryURL string

	// Transaction Configuration
	MinPaymentDrops int64

	// Logging Configuration
	LogLevel string
}

// NewConfig creates a new config from environment variables or defaults
func NewConfig() *Config {
	corsOrigins := getEnv("CORS_ALLOWED_ORIGINS", "http://localhost:3000,http://127.0.0.1:3000")
	validatorListSites := getEnv("VALIDATOR_LIST_SITES", "https://vl.ripple.com,https://unl.xrplf.org")
	cfg := &Config{
		RippledJSONRPCURL:             getEnv("RIPPLED_JSON_RPC_URL", "http://localhost:5005"),
		RippledWebSocketURL:           getEnv("RIPPLED_WEBSOCKET_URL", "ws://localhost:6006"),
		Network:                       strings.ToLower(getEnv("XRPL_NETWORK", "mainnet")),
		ListenPort:                    getEnvInt("LISTEN_PORT", 8080),
		ListenAddr:                    getEnv("LISTEN_ADDR", "0.0.0.0"),
		CORSAllowedOrigins:            splitCSV(corsOrigins),
		ValidatorRefreshInterval:      getEnvInt("VALIDATOR_REFRESH_INTERVAL", 300), // 5 minutes
		ValidatorListSites:            splitCSV(validatorListSites),
		SecondaryValidatorRegistryURL: getEnv("SECONDARY_VALIDATOR_REGISTRY_URL", "https://api.xrpscan.com/api/v1/validatorregistry"),
		MinPaymentDrops:               getEnvInt64("MIN_PAYMENT_DROPS", 1000000000), // 1000 XRP
		LogLevel:                      getEnv("LOG_LEVEL", "info"),
	}
	return cfg
}

func getEnv(key, defaultVal string) string {
	if value, exists := os.LookupEnv(key); exists {
		return value
	}
	return defaultVal
}

func getEnvInt(key string, defaultVal int) int {
	if value, exists := os.LookupEnv(key); exists {
		if intVal, err := strconv.Atoi(value); err == nil {
			return intVal
		}
	}
	return defaultVal
}

func getEnvInt64(key string, defaultVal int64) int64 {
	if value, exists := os.LookupEnv(key); exists {
		if intVal, err := strconv.ParseInt(value, 10, 64); err == nil {
			return intVal
		}
	}
	return defaultVal
}

func splitCSV(value string) []string {
	parts := strings.Split(value, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed != "" {
			out = append(out, trimmed)
		}
	}
	sort.Strings(out)
	return out
}

// Validate checks the configuration for validity
func (c *Config) Validate() error {
	if c.ListenPort <= 0 || c.ListenPort > 65535 {
		return fmt.Errorf("invalid listen port: %d", c.ListenPort)
	}
	if c.ListenAddr == "" {
		return fmt.Errorf("listen address cannot be empty")
	}
	if c.RippledJSONRPCURL == "" {
		return fmt.Errorf("rippled JSON RPC URL cannot be empty")
	}
	if c.RippledWebSocketURL == "" {
		return fmt.Errorf("rippled WebSocket URL cannot be empty")
	}
	if c.Network == "" {
		return fmt.Errorf("network cannot be empty")
	}
	if c.ValidatorRefreshInterval <= 0 {
		return fmt.Errorf("validator refresh interval must be positive: %d", c.ValidatorRefreshInterval)
	}
	if len(c.ValidatorListSites) == 0 {
		return fmt.Errorf("at least one validator list site must be specified")
	}
	if c.SecondaryValidatorRegistryURL == "" {
		return fmt.Errorf("secondary validator registry URL cannot be empty")
	}
	if c.MinPaymentDrops <= 0 {
		return fmt.Errorf("minimum payment drops must be positive: %d", c.MinPaymentDrops)
	}
	if len(c.CORSAllowedOrigins) == 0 {
		return fmt.Errorf("at least one CORS allowed origin must be specified")
	}
	return nil
}

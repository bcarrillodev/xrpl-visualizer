package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

type Config struct {
	// Rippled Configuration
	RippledJSONRPCURL   string
	RippledWebSocketURL string

	// Server Configuration
	ListenPort         int
	ListenAddr         string
	CORSAllowedOrigins []string

	// Validator Fetcher Configuration
	ValidatorRefreshInterval int // seconds

	// Logging Configuration
	LogLevel string
}

// NewConfig creates a new config from environment variables or defaults
func NewConfig() *Config {
	corsOrigins := getEnv("CORS_ALLOWED_ORIGINS", "http://localhost:3000")
	cfg := &Config{
		RippledJSONRPCURL:        getEnv("RIPPLED_JSON_RPC_URL", "http://localhost:5005"),
		RippledWebSocketURL:      getEnv("RIPPLED_WEBSOCKET_URL", "ws://localhost:6006"),
		ListenPort:               getEnvInt("LISTEN_PORT", 8080),
		ListenAddr:               getEnv("LISTEN_ADDR", "0.0.0.0"),
		CORSAllowedOrigins:       strings.Split(corsOrigins, ","),
		ValidatorRefreshInterval: getEnvInt("VALIDATOR_REFRESH_INTERVAL", 300), // 5 minutes
		LogLevel:                 getEnv("LOG_LEVEL", "info"),
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
	if c.ValidatorRefreshInterval <= 0 {
		return fmt.Errorf("validator refresh interval must be positive: %d", c.ValidatorRefreshInterval)
	}
	if len(c.CORSAllowedOrigins) == 0 {
		return fmt.Errorf("at least one CORS allowed origin must be specified")
	}
	return nil
}

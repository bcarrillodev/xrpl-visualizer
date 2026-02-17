package config

import (
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
)

type Config struct {
	// Source Selection
	SourceMode string // local | public | hybrid
	Network    string

	// Local Rippled Configuration
	RippledJSONRPCURL   string
	RippledWebSocketURL string

	// Public Rippled Configuration
	PublicRippledJSONRPCURL   string
	PublicRippledWebSocketURL string

	// Server Configuration
	ListenPort         int
	ListenAddr         string
	CORSAllowedOrigins []string

	// Validator Fetcher Configuration
	ValidatorRefreshInterval      int // seconds
	ValidatorListSites            []string
	SecondaryValidatorRegistryURL string
	ValidatorMetadataCachePath    string
	GeoCachePath                  string
	GeoLookupMinIntervalMS        int
	GeoRateLimitCooldownSeconds   int

	// Transaction Configuration
	MinPaymentDrops int64

	// Logging Configuration
	LogLevel string
}

// NewConfig creates a new config from environment variables or defaults
func NewConfig() *Config {
	corsOrigins := getEnv("CORS_ALLOWED_ORIGINS", "http://localhost:3000,http://127.0.0.1:3000,http://localhost:5173,http://127.0.0.1:5173")
	validatorListSites := getEnv("VALIDATOR_LIST_SITES", "https://vl.ripple.com,https://unl.xrplf.org")
	cfg := &Config{
		SourceMode:                    strings.ToLower(getEnv("XRPL_SOURCE_MODE", "hybrid")),
		RippledJSONRPCURL:             getEnv("RIPPLED_JSON_RPC_URL", "http://localhost:5005"),
		RippledWebSocketURL:           getEnv("RIPPLED_WEBSOCKET_URL", "ws://localhost:6006"),
		PublicRippledJSONRPCURL:       getEnv("PUBLIC_RIPPLED_JSON_RPC_URL", "https://xrplcluster.com"),
		PublicRippledWebSocketURL:     getEnv("PUBLIC_RIPPLED_WEBSOCKET_URL", "wss://xrplcluster.com"),
		Network:                       strings.ToLower(getEnv("XRPL_NETWORK", "mainnet")),
		ListenPort:                    getEnvInt("LISTEN_PORT", 8080),
		ListenAddr:                    getEnv("LISTEN_ADDR", "0.0.0.0"),
		CORSAllowedOrigins:            splitCSV(corsOrigins),
		ValidatorRefreshInterval:      getEnvInt("VALIDATOR_REFRESH_INTERVAL", 300), // 5 minutes
		ValidatorListSites:            splitCSV(validatorListSites),
		SecondaryValidatorRegistryURL: getEnv("SECONDARY_VALIDATOR_REGISTRY_URL", "https://api.xrpscan.com/api/v1/validatorregistry"),
		ValidatorMetadataCachePath:    getEnv("VALIDATOR_METADATA_CACHE_PATH", "data/validator-metadata-cache.json"),
		GeoCachePath:                  getEnv("GEO_CACHE_PATH", "data/geolocation-cache.json"),
		GeoLookupMinIntervalMS:        getEnvInt("GEO_LOOKUP_MIN_INTERVAL_MS", 1200),
		GeoRateLimitCooldownSeconds:   getEnvInt("GEO_RATE_LIMIT_COOLDOWN_SECONDS", 900),
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
	if c.PublicRippledJSONRPCURL == "" {
		return fmt.Errorf("public rippled JSON RPC URL cannot be empty")
	}
	if c.PublicRippledWebSocketURL == "" {
		return fmt.Errorf("public rippled WebSocket URL cannot be empty")
	}
	switch c.SourceMode {
	case "local", "public", "hybrid":
	default:
		return fmt.Errorf("invalid source mode: %s", c.SourceMode)
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
	if strings.TrimSpace(c.ValidatorMetadataCachePath) == "" {
		return fmt.Errorf("validator metadata cache path cannot be empty")
	}
	if strings.TrimSpace(c.GeoCachePath) == "" {
		return fmt.Errorf("geo cache path cannot be empty")
	}
	if c.GeoLookupMinIntervalMS <= 0 {
		return fmt.Errorf("geo lookup min interval must be positive: %d", c.GeoLookupMinIntervalMS)
	}
	if c.GeoRateLimitCooldownSeconds <= 0 {
		return fmt.Errorf("geo rate limit cooldown must be positive: %d", c.GeoRateLimitCooldownSeconds)
	}
	if c.MinPaymentDrops <= 0 {
		return fmt.Errorf("minimum payment drops must be positive: %d", c.MinPaymentDrops)
	}
	if len(c.CORSAllowedOrigins) == 0 {
		return fmt.Errorf("at least one CORS allowed origin must be specified")
	}
	return nil
}

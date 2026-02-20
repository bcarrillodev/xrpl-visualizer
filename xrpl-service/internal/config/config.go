package config

import (
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
)

type Config struct {
	// External XRPL source configuration
	PublicXRPLJSONRPCURL   string
	PublicXRPLWebSocketURL string

	// Transaction Stream Source (external by default)
	TransactionJSONRPCURL   string
	TransactionWebSocketURL string

	Network string

	// Server Configuration
	ListenPort         int
	ListenAddr         string
	CORSAllowedOrigins []string

	// Validator Fetcher Configuration
	ValidatorRefreshInterval      int // seconds
	ValidatorListSites            []string
	SecondaryValidatorRegistryURL string
	ValidatorMetadataCachePath    string
	NetworkHealthJSONRPCURLs      []string
	NetworkHealthRetries          int
	GeoCachePath                  string
	GeoLiteDBPath                 string
	GeoLiteDownloadURL            string
	GeoLiteAutoDownload           bool

	// Transaction Configuration
	MinPaymentDrops       int64
	TransactionBufferSize int
	GeoEnrichmentQSize    int
	GeoEnrichmentWorkers  int
	MaxGeoCandidates      int
	BroadcastBufferSize   int
	WSClientBufferSize    int

	// Logging Configuration
	LogLevel string
}

// NewConfig creates a new config from environment variables or defaults
func NewConfig() *Config {
	corsOrigins := getEnv("CORS_ALLOWED_ORIGINS", "http://localhost:3000,http://127.0.0.1:3000,http://localhost:5173,http://127.0.0.1:5173")
	validatorListSites := getEnv("VALIDATOR_LIST_SITES", "https://vl.ripple.com,https://unl.xrplf.org")
	publicJSONRPCURL := getEnv("PUBLIC_XRPL_JSON_RPC_URL", "https://xrplcluster.com")
	publicWebSocketURL := getEnv("PUBLIC_XRPL_WEBSOCKET_URL", "wss://xrplcluster.com")
	networkHealthJSONRPCURLs := getEnv("NETWORK_HEALTH_JSON_RPC_URLS", publicJSONRPCURL+",https://s2.ripple.com:51234")
	cfg := &Config{
		PublicXRPLJSONRPCURL:          publicJSONRPCURL,
		PublicXRPLWebSocketURL:        publicWebSocketURL,
		TransactionJSONRPCURL:         getEnv("TRANSACTION_JSON_RPC_URL", publicJSONRPCURL),
		TransactionWebSocketURL:       getEnv("TRANSACTION_WEBSOCKET_URL", publicWebSocketURL),
		Network:                       strings.ToLower(getEnv("XRPL_NETWORK", "mainnet")),
		ListenPort:                    getEnvInt("LISTEN_PORT", 8080),
		ListenAddr:                    getEnv("LISTEN_ADDR", "0.0.0.0"),
		CORSAllowedOrigins:            splitCSV(corsOrigins),
		ValidatorRefreshInterval:      getEnvInt("VALIDATOR_REFRESH_INTERVAL", 300), // 5 minutes
		ValidatorListSites:            splitCSV(validatorListSites),
		SecondaryValidatorRegistryURL: getEnv("SECONDARY_VALIDATOR_REGISTRY_URL", "https://api.xrpscan.com/api/v1/validatorregistry"),
		ValidatorMetadataCachePath:    getEnv("VALIDATOR_METADATA_CACHE_PATH", "data/validator-metadata-cache.json"),
		NetworkHealthJSONRPCURLs:      splitCSVPreserveOrder(networkHealthJSONRPCURLs),
		NetworkHealthRetries:          getEnvInt("NETWORK_HEALTH_RETRIES", 2),
		GeoCachePath:                  getEnv("GEO_CACHE_PATH", "data/geolocation-cache.json"),
		GeoLiteDBPath:                 getEnv("GEOLITE_DB_PATH", "data/GeoLite2-City.mmdb"),
		GeoLiteDownloadURL:            getEnv("GEOLITE_DOWNLOAD_URL", "https://github.com/P3TERX/GeoLite.mmdb/raw/download/GeoLite2-City.mmdb"),
		GeoLiteAutoDownload:           getEnvBool("GEOLITE_AUTO_DOWNLOAD", true),
		MinPaymentDrops:               getEnvInt64("MIN_PAYMENT_DROPS", 1000000), // 1 XRP
		TransactionBufferSize:         getEnvInt("TRANSACTION_BUFFER_SIZE", 2048),
		GeoEnrichmentQSize:            getEnvInt("GEO_ENRICHMENT_QUEUE_SIZE", 2048),
		GeoEnrichmentWorkers:          getEnvInt("GEO_ENRICHMENT_WORKERS", 8),
		MaxGeoCandidates:              getEnvInt("MAX_GEO_CANDIDATES", 6),
		BroadcastBufferSize:           getEnvInt("BROADCAST_BUFFER_SIZE", 2048),
		WSClientBufferSize:            getEnvInt("WS_CLIENT_BUFFER_SIZE", 512),
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

func getEnvBool(key string, defaultVal bool) bool {
	if value, exists := os.LookupEnv(key); exists {
		parsed, err := strconv.ParseBool(strings.TrimSpace(value))
		if err == nil {
			return parsed
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

func splitCSVPreserveOrder(value string) []string {
	parts := strings.Split(value, ",")
	out := make([]string, 0, len(parts))
	seen := make(map[string]struct{}, len(parts))
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed == "" {
			continue
		}
		if _, exists := seen[trimmed]; exists {
			continue
		}
		seen[trimmed] = struct{}{}
		out = append(out, trimmed)
	}
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
	if c.PublicXRPLJSONRPCURL == "" {
		return fmt.Errorf("public XRPL JSON RPC URL cannot be empty")
	}
	if c.PublicXRPLWebSocketURL == "" {
		return fmt.Errorf("public XRPL WebSocket URL cannot be empty")
	}
	if c.TransactionJSONRPCURL == "" {
		return fmt.Errorf("transaction JSON RPC URL cannot be empty")
	}
	if c.TransactionWebSocketURL == "" {
		return fmt.Errorf("transaction WebSocket URL cannot be empty")
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
	if len(c.NetworkHealthJSONRPCURLs) == 0 {
		return fmt.Errorf("at least one network health JSON RPC URL must be specified")
	}
	if c.NetworkHealthRetries <= 0 {
		return fmt.Errorf("network health retries must be positive: %d", c.NetworkHealthRetries)
	}
	if strings.TrimSpace(c.GeoCachePath) == "" {
		return fmt.Errorf("geo cache path cannot be empty")
	}
	if strings.TrimSpace(c.GeoLiteDBPath) == "" {
		return fmt.Errorf("GeoLite DB path cannot be empty")
	}
	if c.GeoLiteAutoDownload && strings.TrimSpace(c.GeoLiteDownloadURL) == "" {
		return fmt.Errorf("GeoLite download URL cannot be empty when auto-download is enabled")
	}
	if c.MinPaymentDrops <= 0 {
		return fmt.Errorf("minimum payment drops must be positive: %d", c.MinPaymentDrops)
	}
	if c.TransactionBufferSize <= 0 {
		return fmt.Errorf("transaction buffer size must be positive: %d", c.TransactionBufferSize)
	}
	if c.GeoEnrichmentQSize <= 0 {
		return fmt.Errorf("geo enrichment queue size must be positive: %d", c.GeoEnrichmentQSize)
	}
	if c.GeoEnrichmentWorkers <= 0 {
		return fmt.Errorf("geo enrichment workers must be positive: %d", c.GeoEnrichmentWorkers)
	}
	if c.MaxGeoCandidates <= 0 {
		return fmt.Errorf("max geo candidates must be positive: %d", c.MaxGeoCandidates)
	}
	if c.BroadcastBufferSize <= 0 {
		return fmt.Errorf("broadcast buffer size must be positive: %d", c.BroadcastBufferSize)
	}
	if c.WSClientBufferSize <= 0 {
		return fmt.Errorf("websocket client buffer size must be positive: %d", c.WSClientBufferSize)
	}
	if len(c.CORSAllowedOrigins) == 0 {
		return fmt.Errorf("at least one CORS allowed origin must be specified")
	}
	return nil
}

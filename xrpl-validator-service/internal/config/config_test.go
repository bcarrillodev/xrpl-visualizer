package config

import (
	"os"
	"testing"
)

func TestNewConfig(t *testing.T) {
	cfg := NewConfig()

	if cfg.SourceMode != "hybrid" {
		t.Errorf("Expected SourceMode 'hybrid', got %s", cfg.SourceMode)
	}
	if cfg.ListenPort != 8080 {
		t.Errorf("Expected ListenPort 8080, got %d", cfg.ListenPort)
	}
	if cfg.ListenAddr != "0.0.0.0" {
		t.Errorf("Expected ListenAddr '0.0.0.0', got %s", cfg.ListenAddr)
	}
	if cfg.RippledJSONRPCURL != "http://localhost:5005" {
		t.Errorf("Expected RippledJSONRPCURL 'http://localhost:5005', got %s", cfg.RippledJSONRPCURL)
	}
	if cfg.RippledWebSocketURL != "ws://localhost:6006" {
		t.Errorf("Expected RippledWebSocketURL 'ws://localhost:6006', got %s", cfg.RippledWebSocketURL)
	}
	if cfg.PublicRippledJSONRPCURL != "https://xrplcluster.com" {
		t.Errorf("Expected PublicRippledJSONRPCURL 'https://xrplcluster.com', got %s", cfg.PublicRippledJSONRPCURL)
	}
	if cfg.PublicRippledWebSocketURL != "wss://xrplcluster.com" {
		t.Errorf("Expected PublicRippledWebSocketURL 'wss://xrplcluster.com', got %s", cfg.PublicRippledWebSocketURL)
	}
	if cfg.Network != "mainnet" {
		t.Errorf("Expected Network 'mainnet', got %s", cfg.Network)
	}
	if cfg.ValidatorRefreshInterval != 300 {
		t.Errorf("Expected ValidatorRefreshInterval 300, got %d", cfg.ValidatorRefreshInterval)
	}
	if cfg.MinPaymentDrops != 10000000000 {
		t.Errorf("Expected MinPaymentDrops 10000000000, got %d", cfg.MinPaymentDrops)
	}
	expectedSites := []string{"https://unl.xrplf.org", "https://vl.ripple.com"}
	if len(cfg.ValidatorListSites) != len(expectedSites) {
		t.Errorf("Expected ValidatorListSites length %d, got %d", len(expectedSites), len(cfg.ValidatorListSites))
	}
	for i, site := range expectedSites {
		if cfg.ValidatorListSites[i] != site {
			t.Errorf("Expected ValidatorListSites[%d] '%s', got '%s'", i, site, cfg.ValidatorListSites[i])
		}
	}
	if cfg.SecondaryValidatorRegistryURL != "https://api.xrpscan.com/api/v1/validatorregistry" {
		t.Errorf("Expected SecondaryValidatorRegistryURL default to XRPSCAN API, got %s", cfg.SecondaryValidatorRegistryURL)
	}
	if cfg.ValidatorMetadataCachePath != "data/validator-metadata-cache.json" {
		t.Errorf("Expected ValidatorMetadataCachePath default, got %s", cfg.ValidatorMetadataCachePath)
	}
	if cfg.GeoCachePath != "data/geolocation-cache.json" {
		t.Errorf("Expected GeoCachePath default, got %s", cfg.GeoCachePath)
	}
	if cfg.GeoLookupMinIntervalMS != 1200 {
		t.Errorf("Expected GeoLookupMinIntervalMS 1200, got %d", cfg.GeoLookupMinIntervalMS)
	}
	if cfg.GeoRateLimitCooldownSeconds != 900 {
		t.Errorf("Expected GeoRateLimitCooldownSeconds 900, got %d", cfg.GeoRateLimitCooldownSeconds)
	}

	expectedDefaultCORS := []string{
		"http://127.0.0.1:3000",
		"http://127.0.0.1:5173",
		"http://localhost:3000",
		"http://localhost:5173",
	}
	if len(cfg.CORSAllowedOrigins) != len(expectedDefaultCORS) {
		t.Errorf("Expected CORSAllowedOrigins length %d, got %d", len(expectedDefaultCORS), len(cfg.CORSAllowedOrigins))
	}
	for i, origin := range expectedDefaultCORS {
		if cfg.CORSAllowedOrigins[i] != origin {
			t.Errorf("Expected CORSAllowedOrigins[%d] '%s', got '%s'", i, origin, cfg.CORSAllowedOrigins[i])
		}
	}
}

func TestNewConfigWithEnvVars(t *testing.T) {
	os.Setenv("XRPL_SOURCE_MODE", "public")
	os.Setenv("LISTEN_PORT", "9090")
	os.Setenv("LISTEN_ADDR", "127.0.0.1")
	os.Setenv("RIPPLED_JSON_RPC_URL", "http://local:5005")
	os.Setenv("RIPPLED_WEBSOCKET_URL", "ws://local:6006")
	os.Setenv("PUBLIC_RIPPLED_JSON_RPC_URL", "https://public.example")
	os.Setenv("PUBLIC_RIPPLED_WEBSOCKET_URL", "wss://public.example")
	os.Setenv("XRPL_NETWORK", "testnet")
	os.Setenv("VALIDATOR_REFRESH_INTERVAL", "600")
	os.Setenv("VALIDATOR_LIST_SITES", "https://example.com/vl1,https://example.com/vl2")
	os.Setenv("SECONDARY_VALIDATOR_REGISTRY_URL", "https://example.com/registry")
	os.Setenv("VALIDATOR_METADATA_CACHE_PATH", "/tmp/validator-meta-cache.json")
	os.Setenv("GEO_CACHE_PATH", "/tmp/geo-cache.json")
	os.Setenv("GEO_LOOKUP_MIN_INTERVAL_MS", "2500")
	os.Setenv("GEO_RATE_LIMIT_COOLDOWN_SECONDS", "1800")
	os.Setenv("MIN_PAYMENT_DROPS", "2500000000")
	os.Setenv("LOG_LEVEL", "debug")
	os.Setenv("CORS_ALLOWED_ORIGINS", "http://example.com,http://test.com")

	defer func() {
		os.Unsetenv("XRPL_SOURCE_MODE")
		os.Unsetenv("LISTEN_PORT")
		os.Unsetenv("LISTEN_ADDR")
		os.Unsetenv("RIPPLED_JSON_RPC_URL")
		os.Unsetenv("RIPPLED_WEBSOCKET_URL")
		os.Unsetenv("PUBLIC_RIPPLED_JSON_RPC_URL")
		os.Unsetenv("PUBLIC_RIPPLED_WEBSOCKET_URL")
		os.Unsetenv("XRPL_NETWORK")
		os.Unsetenv("VALIDATOR_REFRESH_INTERVAL")
		os.Unsetenv("VALIDATOR_LIST_SITES")
		os.Unsetenv("SECONDARY_VALIDATOR_REGISTRY_URL")
		os.Unsetenv("VALIDATOR_METADATA_CACHE_PATH")
		os.Unsetenv("GEO_CACHE_PATH")
		os.Unsetenv("GEO_LOOKUP_MIN_INTERVAL_MS")
		os.Unsetenv("GEO_RATE_LIMIT_COOLDOWN_SECONDS")
		os.Unsetenv("MIN_PAYMENT_DROPS")
		os.Unsetenv("LOG_LEVEL")
		os.Unsetenv("CORS_ALLOWED_ORIGINS")
	}()

	cfg := NewConfig()
	if cfg.SourceMode != "public" {
		t.Errorf("Expected SourceMode 'public', got %s", cfg.SourceMode)
	}
	if cfg.ListenPort != 9090 {
		t.Errorf("Expected ListenPort 9090, got %d", cfg.ListenPort)
	}
	if cfg.ListenAddr != "127.0.0.1" {
		t.Errorf("Expected ListenAddr '127.0.0.1', got %s", cfg.ListenAddr)
	}
	if cfg.RippledJSONRPCURL != "http://local:5005" {
		t.Errorf("Expected RippledJSONRPCURL 'http://local:5005', got %s", cfg.RippledJSONRPCURL)
	}
	if cfg.RippledWebSocketURL != "ws://local:6006" {
		t.Errorf("Expected RippledWebSocketURL 'ws://local:6006', got %s", cfg.RippledWebSocketURL)
	}
	if cfg.PublicRippledJSONRPCURL != "https://public.example" {
		t.Errorf("Expected PublicRippledJSONRPCURL 'https://public.example', got %s", cfg.PublicRippledJSONRPCURL)
	}
	if cfg.PublicRippledWebSocketURL != "wss://public.example" {
		t.Errorf("Expected PublicRippledWebSocketURL 'wss://public.example', got %s", cfg.PublicRippledWebSocketURL)
	}
	if cfg.SecondaryValidatorRegistryURL != "https://example.com/registry" {
		t.Errorf("Expected SecondaryValidatorRegistryURL 'https://example.com/registry', got %s", cfg.SecondaryValidatorRegistryURL)
	}
	if cfg.ValidatorMetadataCachePath != "/tmp/validator-meta-cache.json" {
		t.Errorf("Expected ValidatorMetadataCachePath '/tmp/validator-meta-cache.json', got %s", cfg.ValidatorMetadataCachePath)
	}
	if cfg.GeoCachePath != "/tmp/geo-cache.json" {
		t.Errorf("Expected GeoCachePath '/tmp/geo-cache.json', got %s", cfg.GeoCachePath)
	}
	if cfg.GeoLookupMinIntervalMS != 2500 {
		t.Errorf("Expected GeoLookupMinIntervalMS 2500, got %d", cfg.GeoLookupMinIntervalMS)
	}
	if cfg.GeoRateLimitCooldownSeconds != 1800 {
		t.Errorf("Expected GeoRateLimitCooldownSeconds 1800, got %d", cfg.GeoRateLimitCooldownSeconds)
	}
}

func validConfig() *Config {
	return &Config{
		SourceMode:                    "hybrid",
		ListenPort:                    8080,
		ListenAddr:                    "0.0.0.0",
		RippledJSONRPCURL:             "http://localhost:5005",
		RippledWebSocketURL:           "ws://localhost:6006",
		PublicRippledJSONRPCURL:       "https://xrplcluster.com",
		PublicRippledWebSocketURL:     "wss://xrplcluster.com",
		Network:                       "mainnet",
		ValidatorRefreshInterval:      300,
		ValidatorListSites:            []string{"https://vl.ripple.com"},
		SecondaryValidatorRegistryURL: "https://api.xrpscan.com/api/v1/validatorregistry",
		ValidatorMetadataCachePath:    "data/validator-metadata-cache.json",
		GeoCachePath:                  "data/geolocation-cache.json",
		GeoLookupMinIntervalMS:        1200,
		GeoRateLimitCooldownSeconds:   900,
		MinPaymentDrops:               10000000000,
		CORSAllowedOrigins:            []string{"http://localhost:3000"},
	}
}

func TestConfigValidate(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*Config)
		wantErr bool
	}{
		{name: "valid config", mutate: func(*Config) {}, wantErr: false},
		{name: "invalid source mode", mutate: func(c *Config) { c.SourceMode = "invalid" }, wantErr: true},
		{name: "empty public rpc", mutate: func(c *Config) { c.PublicRippledJSONRPCURL = "" }, wantErr: true},
		{name: "empty public ws", mutate: func(c *Config) { c.PublicRippledWebSocketURL = "" }, wantErr: true},
		{name: "empty local rpc", mutate: func(c *Config) { c.RippledJSONRPCURL = "" }, wantErr: true},
		{name: "empty local ws", mutate: func(c *Config) { c.RippledWebSocketURL = "" }, wantErr: true},
		{name: "empty network", mutate: func(c *Config) { c.Network = "" }, wantErr: true},
		{name: "empty validator sites", mutate: func(c *Config) { c.ValidatorListSites = []string{} }, wantErr: true},
		{name: "empty secondary registry", mutate: func(c *Config) { c.SecondaryValidatorRegistryURL = "" }, wantErr: true},
		{name: "empty validator metadata cache path", mutate: func(c *Config) { c.ValidatorMetadataCachePath = "" }, wantErr: true},
		{name: "empty geo cache path", mutate: func(c *Config) { c.GeoCachePath = "" }, wantErr: true},
		{name: "zero geo min interval", mutate: func(c *Config) { c.GeoLookupMinIntervalMS = 0 }, wantErr: true},
		{name: "zero geo cooldown", mutate: func(c *Config) { c.GeoRateLimitCooldownSeconds = 0 }, wantErr: true},
		{name: "zero min payment", mutate: func(c *Config) { c.MinPaymentDrops = 0 }, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := validConfig()
			tt.mutate(cfg)
			err := cfg.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Config.Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

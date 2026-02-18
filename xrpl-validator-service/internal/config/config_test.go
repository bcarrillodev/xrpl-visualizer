package config

import (
	"os"
	"testing"
)

func TestNewConfig(t *testing.T) {
	cfg := NewConfig()

	if cfg.ListenPort != 8080 {
		t.Errorf("Expected ListenPort 8080, got %d", cfg.ListenPort)
	}
	if cfg.ListenAddr != "0.0.0.0" {
		t.Errorf("Expected ListenAddr '0.0.0.0', got %s", cfg.ListenAddr)
	}
	if cfg.PublicRippledJSONRPCURL != "https://xrplcluster.com" {
		t.Errorf("Expected PublicRippledJSONRPCURL 'https://xrplcluster.com', got %s", cfg.PublicRippledJSONRPCURL)
	}
	if cfg.PublicRippledWebSocketURL != "wss://xrplcluster.com" {
		t.Errorf("Expected PublicRippledWebSocketURL 'wss://xrplcluster.com', got %s", cfg.PublicRippledWebSocketURL)
	}
	if cfg.TransactionJSONRPCURL != "https://xrplcluster.com" {
		t.Errorf("Expected TransactionJSONRPCURL 'https://xrplcluster.com', got %s", cfg.TransactionJSONRPCURL)
	}
	if cfg.TransactionWebSocketURL != "wss://xrplcluster.com" {
		t.Errorf("Expected TransactionWebSocketURL 'wss://xrplcluster.com', got %s", cfg.TransactionWebSocketURL)
	}
	if cfg.Network != "mainnet" {
		t.Errorf("Expected Network 'mainnet', got %s", cfg.Network)
	}
	if cfg.ValidatorRefreshInterval != 300 {
		t.Errorf("Expected ValidatorRefreshInterval 300, got %d", cfg.ValidatorRefreshInterval)
	}
	if cfg.MinPaymentDrops != 1000000 {
		t.Errorf("Expected MinPaymentDrops 1000000, got %d", cfg.MinPaymentDrops)
	}
	if cfg.TransactionBufferSize != 2048 {
		t.Errorf("Expected TransactionBufferSize 2048, got %d", cfg.TransactionBufferSize)
	}
	if cfg.GeoEnrichmentQSize != 2048 {
		t.Errorf("Expected GeoEnrichmentQSize 2048, got %d", cfg.GeoEnrichmentQSize)
	}
	if cfg.GeoEnrichmentWorkers != 8 {
		t.Errorf("Expected GeoEnrichmentWorkers 8, got %d", cfg.GeoEnrichmentWorkers)
	}
	if cfg.MaxGeoCandidates != 6 {
		t.Errorf("Expected MaxGeoCandidates 6, got %d", cfg.MaxGeoCandidates)
	}
	if cfg.BroadcastBufferSize != 2048 {
		t.Errorf("Expected BroadcastBufferSize 2048, got %d", cfg.BroadcastBufferSize)
	}
	if cfg.WSClientBufferSize != 512 {
		t.Errorf("Expected WSClientBufferSize 512, got %d", cfg.WSClientBufferSize)
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
	expectedHealthRPCURLs := []string{"https://xrplcluster.com", "https://s2.ripple.com:51234"}
	if len(cfg.NetworkHealthJSONRPCURLs) != len(expectedHealthRPCURLs) {
		t.Errorf("Expected NetworkHealthJSONRPCURLs length %d, got %d", len(expectedHealthRPCURLs), len(cfg.NetworkHealthJSONRPCURLs))
	}
	for i, endpoint := range expectedHealthRPCURLs {
		if cfg.NetworkHealthJSONRPCURLs[i] != endpoint {
			t.Errorf("Expected NetworkHealthJSONRPCURLs[%d] '%s', got '%s'", i, endpoint, cfg.NetworkHealthJSONRPCURLs[i])
		}
	}
	if cfg.NetworkHealthRetries != 2 {
		t.Errorf("Expected NetworkHealthRetries 2, got %d", cfg.NetworkHealthRetries)
	}
	if cfg.ValidatorMetadataCachePath != "data/validator-metadata-cache.json" {
		t.Errorf("Expected ValidatorMetadataCachePath default, got %s", cfg.ValidatorMetadataCachePath)
	}
	if cfg.GeoCachePath != "data/geolocation-cache.json" {
		t.Errorf("Expected GeoCachePath default, got %s", cfg.GeoCachePath)
	}
	if cfg.GeoLiteDBPath != "data/GeoLite2-City.mmdb" {
		t.Errorf("Expected GeoLiteDBPath default, got %s", cfg.GeoLiteDBPath)
	}
	if cfg.GeoLiteDownloadURL != "https://github.com/P3TERX/GeoLite.mmdb/raw/download/GeoLite2-City.mmdb" {
		t.Errorf("Expected GeoLiteDownloadURL default, got %s", cfg.GeoLiteDownloadURL)
	}
	if !cfg.GeoLiteAutoDownload {
		t.Errorf("Expected GeoLiteAutoDownload default true")
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
	os.Setenv("LISTEN_PORT", "9090")
	os.Setenv("LISTEN_ADDR", "127.0.0.1")
	os.Setenv("PUBLIC_RIPPLED_JSON_RPC_URL", "https://public.example")
	os.Setenv("PUBLIC_RIPPLED_WEBSOCKET_URL", "wss://public.example")
	os.Setenv("TRANSACTION_JSON_RPC_URL", "https://txrpc.example")
	os.Setenv("TRANSACTION_WEBSOCKET_URL", "wss://txws.example")
	os.Setenv("XRPL_NETWORK", "testnet")
	os.Setenv("VALIDATOR_REFRESH_INTERVAL", "600")
	os.Setenv("VALIDATOR_LIST_SITES", "https://example.com/vl1,https://example.com/vl2")
	os.Setenv("SECONDARY_VALIDATOR_REGISTRY_URL", "https://example.com/registry")
	os.Setenv("VALIDATOR_METADATA_CACHE_PATH", "/tmp/validator-meta-cache.json")
	os.Setenv("NETWORK_HEALTH_JSON_RPC_URLS", "https://health-1.example,https://health-2.example")
	os.Setenv("NETWORK_HEALTH_RETRIES", "4")
	os.Setenv("GEO_CACHE_PATH", "/tmp/geo-cache.json")
	os.Setenv("GEOLITE_DB_PATH", "/tmp/GeoLite2-City.mmdb")
	os.Setenv("GEOLITE_DOWNLOAD_URL", "https://example.com/geolite.mmdb")
	os.Setenv("GEOLITE_AUTO_DOWNLOAD", "false")
	os.Setenv("MIN_PAYMENT_DROPS", "2500000000")
	os.Setenv("TRANSACTION_BUFFER_SIZE", "4096")
	os.Setenv("GEO_ENRICHMENT_QUEUE_SIZE", "4096")
	os.Setenv("GEO_ENRICHMENT_WORKERS", "16")
	os.Setenv("MAX_GEO_CANDIDATES", "10")
	os.Setenv("BROADCAST_BUFFER_SIZE", "3000")
	os.Setenv("WS_CLIENT_BUFFER_SIZE", "700")
	os.Setenv("LOG_LEVEL", "debug")
	os.Setenv("CORS_ALLOWED_ORIGINS", "http://example.com,http://test.com")

	defer func() {
		os.Unsetenv("LISTEN_PORT")
		os.Unsetenv("LISTEN_ADDR")
		os.Unsetenv("PUBLIC_RIPPLED_JSON_RPC_URL")
		os.Unsetenv("PUBLIC_RIPPLED_WEBSOCKET_URL")
		os.Unsetenv("TRANSACTION_JSON_RPC_URL")
		os.Unsetenv("TRANSACTION_WEBSOCKET_URL")
		os.Unsetenv("XRPL_NETWORK")
		os.Unsetenv("VALIDATOR_REFRESH_INTERVAL")
		os.Unsetenv("VALIDATOR_LIST_SITES")
		os.Unsetenv("SECONDARY_VALIDATOR_REGISTRY_URL")
		os.Unsetenv("VALIDATOR_METADATA_CACHE_PATH")
		os.Unsetenv("NETWORK_HEALTH_JSON_RPC_URLS")
		os.Unsetenv("NETWORK_HEALTH_RETRIES")
		os.Unsetenv("GEO_CACHE_PATH")
		os.Unsetenv("GEOLITE_DB_PATH")
		os.Unsetenv("GEOLITE_DOWNLOAD_URL")
		os.Unsetenv("GEOLITE_AUTO_DOWNLOAD")
		os.Unsetenv("MIN_PAYMENT_DROPS")
		os.Unsetenv("TRANSACTION_BUFFER_SIZE")
		os.Unsetenv("GEO_ENRICHMENT_QUEUE_SIZE")
		os.Unsetenv("GEO_ENRICHMENT_WORKERS")
		os.Unsetenv("MAX_GEO_CANDIDATES")
		os.Unsetenv("BROADCAST_BUFFER_SIZE")
		os.Unsetenv("WS_CLIENT_BUFFER_SIZE")
		os.Unsetenv("LOG_LEVEL")
		os.Unsetenv("CORS_ALLOWED_ORIGINS")
	}()

	cfg := NewConfig()
	if cfg.ListenPort != 9090 {
		t.Errorf("Expected ListenPort 9090, got %d", cfg.ListenPort)
	}
	if cfg.ListenAddr != "127.0.0.1" {
		t.Errorf("Expected ListenAddr '127.0.0.1', got %s", cfg.ListenAddr)
	}
	if cfg.PublicRippledJSONRPCURL != "https://public.example" {
		t.Errorf("Expected PublicRippledJSONRPCURL 'https://public.example', got %s", cfg.PublicRippledJSONRPCURL)
	}
	if cfg.PublicRippledWebSocketURL != "wss://public.example" {
		t.Errorf("Expected PublicRippledWebSocketURL 'wss://public.example', got %s", cfg.PublicRippledWebSocketURL)
	}
	if cfg.TransactionJSONRPCURL != "https://txrpc.example" {
		t.Errorf("Expected TransactionJSONRPCURL 'https://txrpc.example', got %s", cfg.TransactionJSONRPCURL)
	}
	if cfg.TransactionWebSocketURL != "wss://txws.example" {
		t.Errorf("Expected TransactionWebSocketURL 'wss://txws.example', got %s", cfg.TransactionWebSocketURL)
	}
	if cfg.SecondaryValidatorRegistryURL != "https://example.com/registry" {
		t.Errorf("Expected SecondaryValidatorRegistryURL 'https://example.com/registry', got %s", cfg.SecondaryValidatorRegistryURL)
	}
	if cfg.ValidatorMetadataCachePath != "/tmp/validator-meta-cache.json" {
		t.Errorf("Expected ValidatorMetadataCachePath '/tmp/validator-meta-cache.json', got %s", cfg.ValidatorMetadataCachePath)
	}
	expectedConfiguredHealthRPCURLs := []string{"https://health-1.example", "https://health-2.example"}
	if len(cfg.NetworkHealthJSONRPCURLs) != len(expectedConfiguredHealthRPCURLs) {
		t.Errorf("Expected NetworkHealthJSONRPCURLs length %d, got %d", len(expectedConfiguredHealthRPCURLs), len(cfg.NetworkHealthJSONRPCURLs))
	}
	for i, endpoint := range expectedConfiguredHealthRPCURLs {
		if cfg.NetworkHealthJSONRPCURLs[i] != endpoint {
			t.Errorf("Expected NetworkHealthJSONRPCURLs[%d] '%s', got '%s'", i, endpoint, cfg.NetworkHealthJSONRPCURLs[i])
		}
	}
	if cfg.NetworkHealthRetries != 4 {
		t.Errorf("Expected NetworkHealthRetries 4, got %d", cfg.NetworkHealthRetries)
	}
	if cfg.GeoCachePath != "/tmp/geo-cache.json" {
		t.Errorf("Expected GeoCachePath '/tmp/geo-cache.json', got %s", cfg.GeoCachePath)
	}
	if cfg.GeoLiteDBPath != "/tmp/GeoLite2-City.mmdb" {
		t.Errorf("Expected GeoLiteDBPath '/tmp/GeoLite2-City.mmdb', got %s", cfg.GeoLiteDBPath)
	}
	if cfg.GeoLiteDownloadURL != "https://example.com/geolite.mmdb" {
		t.Errorf("Expected GeoLiteDownloadURL 'https://example.com/geolite.mmdb', got %s", cfg.GeoLiteDownloadURL)
	}
	if cfg.GeoLiteAutoDownload {
		t.Errorf("Expected GeoLiteAutoDownload false")
	}
	if cfg.TransactionBufferSize != 4096 {
		t.Errorf("Expected TransactionBufferSize 4096, got %d", cfg.TransactionBufferSize)
	}
	if cfg.GeoEnrichmentQSize != 4096 {
		t.Errorf("Expected GeoEnrichmentQSize 4096, got %d", cfg.GeoEnrichmentQSize)
	}
	if cfg.GeoEnrichmentWorkers != 16 {
		t.Errorf("Expected GeoEnrichmentWorkers 16, got %d", cfg.GeoEnrichmentWorkers)
	}
	if cfg.MaxGeoCandidates != 10 {
		t.Errorf("Expected MaxGeoCandidates 10, got %d", cfg.MaxGeoCandidates)
	}
	if cfg.BroadcastBufferSize != 3000 {
		t.Errorf("Expected BroadcastBufferSize 3000, got %d", cfg.BroadcastBufferSize)
	}
	if cfg.WSClientBufferSize != 700 {
		t.Errorf("Expected WSClientBufferSize 700, got %d", cfg.WSClientBufferSize)
	}
}

func validConfig() *Config {
	return &Config{
		ListenPort:                    8080,
		ListenAddr:                    "0.0.0.0",
		PublicRippledJSONRPCURL:       "https://xrplcluster.com",
		PublicRippledWebSocketURL:     "wss://xrplcluster.com",
		TransactionJSONRPCURL:         "https://xrplcluster.com",
		TransactionWebSocketURL:       "wss://xrplcluster.com",
		Network:                       "mainnet",
		ValidatorRefreshInterval:      300,
		ValidatorListSites:            []string{"https://vl.ripple.com"},
		SecondaryValidatorRegistryURL: "https://api.xrpscan.com/api/v1/validatorregistry",
		ValidatorMetadataCachePath:    "data/validator-metadata-cache.json",
		NetworkHealthJSONRPCURLs:      []string{"https://xrplcluster.com", "https://s2.ripple.com:51234"},
		NetworkHealthRetries:          2,
		GeoCachePath:                  "data/geolocation-cache.json",
		GeoLiteDBPath:                 "data/GeoLite2-City.mmdb",
		GeoLiteDownloadURL:            "https://github.com/P3TERX/GeoLite.mmdb/raw/download/GeoLite2-City.mmdb",
		GeoLiteAutoDownload:           true,
		MinPaymentDrops:               1000000,
		TransactionBufferSize:         2048,
		GeoEnrichmentQSize:            2048,
		GeoEnrichmentWorkers:          8,
		MaxGeoCandidates:              6,
		BroadcastBufferSize:           2048,
		WSClientBufferSize:            512,
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
		{name: "empty public rpc", mutate: func(c *Config) { c.PublicRippledJSONRPCURL = "" }, wantErr: true},
		{name: "empty public ws", mutate: func(c *Config) { c.PublicRippledWebSocketURL = "" }, wantErr: true},
		{name: "empty transaction rpc", mutate: func(c *Config) { c.TransactionJSONRPCURL = "" }, wantErr: true},
		{name: "empty transaction ws", mutate: func(c *Config) { c.TransactionWebSocketURL = "" }, wantErr: true},
		{name: "empty network", mutate: func(c *Config) { c.Network = "" }, wantErr: true},
		{name: "empty validator sites", mutate: func(c *Config) { c.ValidatorListSites = []string{} }, wantErr: true},
		{name: "empty secondary registry", mutate: func(c *Config) { c.SecondaryValidatorRegistryURL = "" }, wantErr: true},
		{name: "empty validator metadata cache path", mutate: func(c *Config) { c.ValidatorMetadataCachePath = "" }, wantErr: true},
		{name: "empty network health rpc urls", mutate: func(c *Config) { c.NetworkHealthJSONRPCURLs = []string{} }, wantErr: true},
		{name: "zero network health retries", mutate: func(c *Config) { c.NetworkHealthRetries = 0 }, wantErr: true},
		{name: "empty geo cache path", mutate: func(c *Config) { c.GeoCachePath = "" }, wantErr: true},
		{name: "empty geolite db path", mutate: func(c *Config) { c.GeoLiteDBPath = "" }, wantErr: true},
		{name: "empty geolite download when auto enabled", mutate: func(c *Config) { c.GeoLiteDownloadURL = "" }, wantErr: true},
		{name: "empty geolite download when auto disabled", mutate: func(c *Config) { c.GeoLiteAutoDownload = false; c.GeoLiteDownloadURL = "" }, wantErr: false},
		{name: "zero min payment", mutate: func(c *Config) { c.MinPaymentDrops = 0 }, wantErr: true},
		{name: "zero transaction buffer size", mutate: func(c *Config) { c.TransactionBufferSize = 0 }, wantErr: true},
		{name: "zero geo enrichment queue size", mutate: func(c *Config) { c.GeoEnrichmentQSize = 0 }, wantErr: true},
		{name: "zero geo enrichment workers", mutate: func(c *Config) { c.GeoEnrichmentWorkers = 0 }, wantErr: true},
		{name: "zero max geo candidates", mutate: func(c *Config) { c.MaxGeoCandidates = 0 }, wantErr: true},
		{name: "zero broadcast buffer size", mutate: func(c *Config) { c.BroadcastBufferSize = 0 }, wantErr: true},
		{name: "zero ws client buffer size", mutate: func(c *Config) { c.WSClientBufferSize = 0 }, wantErr: true},
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

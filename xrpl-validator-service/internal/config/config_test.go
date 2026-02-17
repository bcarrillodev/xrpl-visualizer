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
	if cfg.RippledJSONRPCURL != "http://localhost:5005" {
		t.Errorf("Expected RippledJSONRPCURL 'http://localhost:5005', got %s", cfg.RippledJSONRPCURL)
	}
	if cfg.RippledWebSocketURL != "ws://localhost:6006" {
		t.Errorf("Expected RippledWebSocketURL 'ws://localhost:6006', got %s", cfg.RippledWebSocketURL)
	}
	if cfg.Network != "mainnet" {
		t.Errorf("Expected Network 'mainnet', got %s", cfg.Network)
	}
	if cfg.ValidatorRefreshInterval != 300 {
		t.Errorf("Expected ValidatorRefreshInterval 300, got %d", cfg.ValidatorRefreshInterval)
	}
	if cfg.MinPaymentDrops != 1000000000 {
		t.Errorf("Expected MinPaymentDrops 1000000000, got %d", cfg.MinPaymentDrops)
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
	if cfg.LogLevel != "info" {
		t.Errorf("Expected LogLevel 'info', got %s", cfg.LogLevel)
	}
	expectedDefaultCORS := []string{"http://127.0.0.1:3000", "http://localhost:3000"}
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
	os.Setenv("RIPPLED_JSON_RPC_URL", "http://test:5005")
	os.Setenv("RIPPLED_WEBSOCKET_URL", "ws://test:6006")
	os.Setenv("XRPL_NETWORK", "testnet")
	os.Setenv("VALIDATOR_REFRESH_INTERVAL", "600")
	os.Setenv("VALIDATOR_LIST_SITES", "https://example.com/vl1,https://example.com/vl2")
	os.Setenv("SECONDARY_VALIDATOR_REGISTRY_URL", "https://example.com/registry")
	os.Setenv("MIN_PAYMENT_DROPS", "2500000000")
	os.Setenv("LOG_LEVEL", "debug")
	os.Setenv("CORS_ALLOWED_ORIGINS", "http://example.com,http://test.com")

	defer func() {
		os.Unsetenv("LISTEN_PORT")
		os.Unsetenv("LISTEN_ADDR")
		os.Unsetenv("RIPPLED_JSON_RPC_URL")
		os.Unsetenv("RIPPLED_WEBSOCKET_URL")
		os.Unsetenv("XRPL_NETWORK")
		os.Unsetenv("VALIDATOR_REFRESH_INTERVAL")
		os.Unsetenv("VALIDATOR_LIST_SITES")
		os.Unsetenv("SECONDARY_VALIDATOR_REGISTRY_URL")
		os.Unsetenv("MIN_PAYMENT_DROPS")
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
	if cfg.RippledJSONRPCURL != "http://test:5005" {
		t.Errorf("Expected RippledJSONRPCURL 'http://test:5005', got %s", cfg.RippledJSONRPCURL)
	}
	if cfg.RippledWebSocketURL != "ws://test:6006" {
		t.Errorf("Expected RippledWebSocketURL 'ws://test:6006', got %s", cfg.RippledWebSocketURL)
	}
	if cfg.Network != "testnet" {
		t.Errorf("Expected Network 'testnet', got %s", cfg.Network)
	}
	if cfg.ValidatorRefreshInterval != 600 {
		t.Errorf("Expected ValidatorRefreshInterval 600, got %d", cfg.ValidatorRefreshInterval)
	}
	if cfg.MinPaymentDrops != 2500000000 {
		t.Errorf("Expected MinPaymentDrops 2500000000, got %d", cfg.MinPaymentDrops)
	}
	expectedSites := []string{"https://example.com/vl1", "https://example.com/vl2"}
	if len(cfg.ValidatorListSites) != len(expectedSites) {
		t.Errorf("Expected ValidatorListSites length %d, got %d", len(expectedSites), len(cfg.ValidatorListSites))
	}
	for i, site := range expectedSites {
		if cfg.ValidatorListSites[i] != site {
			t.Errorf("Expected ValidatorListSites[%d] '%s', got '%s'", i, site, cfg.ValidatorListSites[i])
		}
	}
	if cfg.SecondaryValidatorRegistryURL != "https://example.com/registry" {
		t.Errorf("Expected SecondaryValidatorRegistryURL 'https://example.com/registry', got %s", cfg.SecondaryValidatorRegistryURL)
	}
	if cfg.LogLevel != "debug" {
		t.Errorf("Expected LogLevel 'debug', got %s", cfg.LogLevel)
	}
	expectedCORS := []string{"http://example.com", "http://test.com"}
	if len(cfg.CORSAllowedOrigins) != len(expectedCORS) {
		t.Errorf("Expected CORSAllowedOrigins length %d, got %d", len(expectedCORS), len(cfg.CORSAllowedOrigins))
	}
	for i, origin := range expectedCORS {
		if cfg.CORSAllowedOrigins[i] != origin {
			t.Errorf("Expected CORSAllowedOrigins[%d] '%s', got '%s'", i, origin, cfg.CORSAllowedOrigins[i])
		}
	}
}

func TestConfigValidate(t *testing.T) {
	tests := []struct {
		name    string
		config  *Config
		wantErr bool
	}{
		{
			name: "valid config",
			config: &Config{
				ListenPort:                    8080,
				ListenAddr:                    "0.0.0.0",
				RippledJSONRPCURL:             "http://localhost:5005",
				RippledWebSocketURL:           "ws://localhost:6006",
				Network:                       "mainnet",
				ValidatorRefreshInterval:      300,
				ValidatorListSites:            []string{"https://vl.ripple.com"},
				SecondaryValidatorRegistryURL: "https://api.xrpscan.com/api/v1/validatorregistry",
				MinPaymentDrops:               1000000000,
				CORSAllowedOrigins:            []string{"http://localhost:3000"},
			},
			wantErr: false,
		},
		{
			name: "invalid listen port - zero",
			config: &Config{
				ListenPort:                    0,
				ListenAddr:                    "0.0.0.0",
				RippledJSONRPCURL:             "http://localhost:5005",
				RippledWebSocketURL:           "ws://localhost:6006",
				Network:                       "mainnet",
				ValidatorRefreshInterval:      300,
				ValidatorListSites:            []string{"https://vl.ripple.com"},
				SecondaryValidatorRegistryURL: "https://api.xrpscan.com/api/v1/validatorregistry",
				MinPaymentDrops:               1000000000,
				CORSAllowedOrigins:            []string{"http://localhost:3000"},
			},
			wantErr: true,
		},
		{
			name: "invalid listen port - too high",
			config: &Config{
				ListenPort:                    70000,
				ListenAddr:                    "0.0.0.0",
				RippledJSONRPCURL:             "http://localhost:5005",
				RippledWebSocketURL:           "ws://localhost:6006",
				Network:                       "mainnet",
				ValidatorRefreshInterval:      300,
				ValidatorListSites:            []string{"https://vl.ripple.com"},
				SecondaryValidatorRegistryURL: "https://api.xrpscan.com/api/v1/validatorregistry",
				MinPaymentDrops:               1000000000,
				CORSAllowedOrigins:            []string{"http://localhost:3000"},
			},
			wantErr: true,
		},
		{
			name: "empty listen addr",
			config: &Config{
				ListenPort:                    8080,
				ListenAddr:                    "",
				RippledJSONRPCURL:             "http://localhost:5005",
				RippledWebSocketURL:           "ws://localhost:6006",
				Network:                       "mainnet",
				ValidatorRefreshInterval:      300,
				ValidatorListSites:            []string{"https://vl.ripple.com"},
				SecondaryValidatorRegistryURL: "https://api.xrpscan.com/api/v1/validatorregistry",
				MinPaymentDrops:               1000000000,
				CORSAllowedOrigins:            []string{"http://localhost:3000"},
			},
			wantErr: true,
		},
		{
			name: "empty rippled json rpc url",
			config: &Config{
				ListenPort:                    8080,
				ListenAddr:                    "0.0.0.0",
				RippledJSONRPCURL:             "",
				RippledWebSocketURL:           "ws://localhost:6006",
				Network:                       "mainnet",
				ValidatorRefreshInterval:      300,
				ValidatorListSites:            []string{"https://vl.ripple.com"},
				SecondaryValidatorRegistryURL: "https://api.xrpscan.com/api/v1/validatorregistry",
				MinPaymentDrops:               1000000000,
				CORSAllowedOrigins:            []string{"http://localhost:3000"},
			},
			wantErr: true,
		},
		{
			name: "empty rippled websocket url",
			config: &Config{
				ListenPort:                    8080,
				ListenAddr:                    "0.0.0.0",
				RippledJSONRPCURL:             "http://localhost:5005",
				RippledWebSocketURL:           "",
				Network:                       "mainnet",
				ValidatorRefreshInterval:      300,
				ValidatorListSites:            []string{"https://vl.ripple.com"},
				SecondaryValidatorRegistryURL: "https://api.xrpscan.com/api/v1/validatorregistry",
				MinPaymentDrops:               1000000000,
				CORSAllowedOrigins:            []string{"http://localhost:3000"},
			},
			wantErr: true,
		},
		{
			name: "empty network",
			config: &Config{
				ListenPort:                    8080,
				ListenAddr:                    "0.0.0.0",
				RippledJSONRPCURL:             "http://localhost:5005",
				RippledWebSocketURL:           "ws://localhost:6006",
				Network:                       "",
				ValidatorRefreshInterval:      300,
				ValidatorListSites:            []string{"https://vl.ripple.com"},
				SecondaryValidatorRegistryURL: "https://api.xrpscan.com/api/v1/validatorregistry",
				MinPaymentDrops:               1000000000,
				CORSAllowedOrigins:            []string{"http://localhost:3000"},
			},
			wantErr: true,
		},
		{
			name: "zero validator refresh interval",
			config: &Config{
				ListenPort:                    8080,
				ListenAddr:                    "0.0.0.0",
				RippledJSONRPCURL:             "http://localhost:5005",
				RippledWebSocketURL:           "ws://localhost:6006",
				Network:                       "mainnet",
				ValidatorRefreshInterval:      0,
				ValidatorListSites:            []string{"https://vl.ripple.com"},
				SecondaryValidatorRegistryURL: "https://api.xrpscan.com/api/v1/validatorregistry",
				MinPaymentDrops:               1000000000,
				CORSAllowedOrigins:            []string{"http://localhost:3000"},
			},
			wantErr: true,
		},
		{
			name: "empty validator list sites",
			config: &Config{
				ListenPort:                    8080,
				ListenAddr:                    "0.0.0.0",
				RippledJSONRPCURL:             "http://localhost:5005",
				RippledWebSocketURL:           "ws://localhost:6006",
				Network:                       "mainnet",
				ValidatorRefreshInterval:      300,
				ValidatorListSites:            []string{},
				SecondaryValidatorRegistryURL: "https://api.xrpscan.com/api/v1/validatorregistry",
				MinPaymentDrops:               1000000000,
				CORSAllowedOrigins:            []string{"http://localhost:3000"},
			},
			wantErr: true,
		},
		{
			name: "empty secondary validator registry url",
			config: &Config{
				ListenPort:                    8080,
				ListenAddr:                    "0.0.0.0",
				RippledJSONRPCURL:             "http://localhost:5005",
				RippledWebSocketURL:           "ws://localhost:6006",
				Network:                       "mainnet",
				ValidatorRefreshInterval:      300,
				ValidatorListSites:            []string{"https://vl.ripple.com"},
				SecondaryValidatorRegistryURL: "",
				MinPaymentDrops:               1000000000,
				CORSAllowedOrigins:            []string{"http://localhost:3000"},
			},
			wantErr: true,
		},
		{
			name: "zero min payment drops",
			config: &Config{
				ListenPort:                    8080,
				ListenAddr:                    "0.0.0.0",
				RippledJSONRPCURL:             "http://localhost:5005",
				RippledWebSocketURL:           "ws://localhost:6006",
				Network:                       "mainnet",
				ValidatorRefreshInterval:      300,
				ValidatorListSites:            []string{"https://vl.ripple.com"},
				SecondaryValidatorRegistryURL: "https://api.xrpscan.com/api/v1/validatorregistry",
				MinPaymentDrops:               0,
				CORSAllowedOrigins:            []string{"http://localhost:3000"},
			},
			wantErr: true,
		},
		{
			name: "empty cors allowed origins",
			config: &Config{
				ListenPort:                    8080,
				ListenAddr:                    "0.0.0.0",
				RippledJSONRPCURL:             "http://localhost:5005",
				RippledWebSocketURL:           "ws://localhost:6006",
				Network:                       "mainnet",
				ValidatorRefreshInterval:      300,
				ValidatorListSites:            []string{"https://vl.ripple.com"},
				SecondaryValidatorRegistryURL: "https://api.xrpscan.com/api/v1/validatorregistry",
				MinPaymentDrops:               1000000000,
				CORSAllowedOrigins:            []string{},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Config.Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

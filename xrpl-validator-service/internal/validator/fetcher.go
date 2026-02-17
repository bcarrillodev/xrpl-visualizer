package validator

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/brandon/xrpl-validator-service/internal/models"
	"github.com/brandon/xrpl-validator-service/internal/rippled"
	"github.com/sirupsen/logrus"
)

// Fetcher handles validator data retrieval and caching
type Fetcher struct {
	client               rippled.RippledClient
	logger               *logrus.Logger
	mu                   sync.RWMutex
	validators           map[string]*models.Validator // Address -> Validator
	lastUpdate           time.Time
	refreshInterval      time.Duration
	stopChan             chan struct{}
	geolocationProvider  GeoLocationProvider
	maxValidators        int
	validatorListSites   []string
	secondaryRegistryURL string
	network              string
}

// GeoLocationProvider defines the interface for geolocation enrichment
type GeoLocationProvider interface {
	// EnrichValidator adds geolocation data to a validator
	EnrichValidator(validator *models.Validator) error
}

// NewFetcher creates a new validator fetcher
func NewFetcher(client rippled.RippledClient, refreshInterval time.Duration, geoProvider GeoLocationProvider, validatorListSites []string, secondaryRegistryURL string, network string, logger *logrus.Logger) *Fetcher {
	if logger == nil {
		logger = logrus.New()
	}
	sites := make([]string, 0, len(validatorListSites))
	for _, site := range validatorListSites {
		trimmed := strings.TrimSpace(site)
		if trimmed != "" {
			sites = append(sites, trimmed)
		}
	}
	if len(sites) == 0 {
		sites = []string{"https://vl.ripple.com"}
	}
	if strings.TrimSpace(network) == "" {
		network = "mainnet"
	}
	if strings.TrimSpace(secondaryRegistryURL) == "" {
		secondaryRegistryURL = "https://api.xrpscan.com/api/v1/validatorregistry"
	}
	return &Fetcher{
		client:               client,
		logger:               logger,
		validators:           make(map[string]*models.Validator),
		refreshInterval:      refreshInterval,
		stopChan:             make(chan struct{}),
		geolocationProvider:  geoProvider,
		maxValidators:        1000, // Limit to prevent memory exhaustion
		validatorListSites:   sites,
		secondaryRegistryURL: secondaryRegistryURL,
		network:              strings.ToLower(network),
	}
}

// Start begins the periodic validator fetching
func (f *Fetcher) Start(ctx context.Context) {
	go func() {
		// Fetch immediately on start
		if err := f.Fetch(ctx); err != nil {
			f.logger.WithError(err).Error("Initial validator fetch failed")
		}

		// Set up periodic fetching
		ticker := time.NewTicker(f.refreshInterval)
		defer ticker.Stop()

		for {
			select {
			case <-f.stopChan:
				f.logger.Info("Validator fetcher stopped")
				return
			case <-ticker.C:
				if err := f.Fetch(ctx); err != nil {
					f.logger.WithError(err).Error("Periodic validator fetch failed")
				}
			}
		}
	}()
}

// Stop stops the periodic fetching
func (f *Fetcher) Stop() {
	close(f.stopChan)
}

// Fetch retrieves current validators from rippled
func (f *Fetcher) Fetch(ctx context.Context) error {
	f.logger.Debug("Fetching validators from rippled")

	// Query rippled for validator information
	// Using ledger_closed subscription to get updated validator set
	result, err := f.fetchValidatorList(ctx)
	if err != nil {
		return fmt.Errorf("failed to fetch validator list: %w", err)
	}

	validators, err := f.parseValidators(result)
	if err != nil {
		return fmt.Errorf("failed to parse validators: %w", err)
	}

	trustedValidators, trustedSet, err := f.fetchTrustedValidatorsFromRippled(ctx)
	if err != nil {
		f.logger.WithError(err).Warn("Failed to fetch trusted validators from rippled")
	}
	validators = mergeValidators(validators, trustedValidators)

	validators, err = f.applySecondaryRegistryDomains(ctx, validators, trustedSet)
	if err != nil {
		f.logger.WithError(err).Warn("Failed to enrich validators from secondary registry")
	}

	// Limit the number of validators to prevent memory exhaustion
	if len(validators) > f.maxValidators {
		f.logger.WithFields(logrus.Fields{
			"fetched": len(validators),
			"limit":   f.maxValidators,
		}).Warn("Limiting validators to prevent memory exhaustion")
		validators = validators[:f.maxValidators]
	}

	// Enrich validators with geolocation data
	for _, v := range validators {
		if f.geolocationProvider != nil {
			if err := f.geolocationProvider.EnrichValidator(v); err != nil {
				f.logger.WithError(err).WithField("address", v.Address).Warn("Failed to enrich validator geolocation")
			}
		}
	}

	// Update cache
	f.mu.Lock()
	f.validators = make(map[string]*models.Validator)
	for _, v := range validators {
		f.validators[v.Address] = v
	}
	f.lastUpdate = time.Now()
	f.mu.Unlock()

	f.logger.WithField("count", len(validators)).Info("Validators updated")
	return nil
}

// GetValidators returns the cached list of validators
func (f *Fetcher) GetValidators() []*models.Validator {
	f.mu.RLock()
	defer f.mu.RUnlock()

	validators := make([]*models.Validator, 0, len(f.validators))
	for _, v := range f.validators {
		validators = append(validators, v)
	}
	return validators
}

// GetValidator returns a specific validator by address
func (f *Fetcher) GetValidator(address string) *models.Validator {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return f.validators[address]
}

// GetLastUpdate returns the last update time
func (f *Fetcher) GetLastUpdate() time.Time {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return f.lastUpdate
}

// GetServerStatus retrieves current rippled server health information.
func (f *Fetcher) GetServerStatus(ctx context.Context) (*models.ServerStatus, error) {
	result, err := f.client.GetServerInfo(ctx)
	if err != nil {
		return nil, err
	}

	resultMap, ok := result.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("unexpected server_info response format")
	}
	payload, ok := resultMap["result"].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("missing server_info result payload")
	}
	info, ok := payload["info"].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("missing server_info info payload")
	}

	status := &models.ServerStatus{
		Connected:       true,
		ServerState:     getString(info, "server_state"),
		LedgerIndex:     uint32(getInt64(getMap(info, "validated_ledger"), "seq")),
		NetworkID:       uint16(getInt64(info, "network_id")),
		PeerCount:       int(getInt64(info, "peers")),
		CompleteLedgers: getString(info, "complete_ledgers"),
		Uptime:          getInt64(info, "uptime"),
		LastSync:        time.Now().Unix(),
	}

	return status, nil
}

func getMap(parent map[string]interface{}, key string) map[string]interface{} {
	value, ok := parent[key].(map[string]interface{})
	if !ok {
		return map[string]interface{}{}
	}
	return value
}

func getString(parent map[string]interface{}, key string) string {
	value, _ := parent[key].(string)
	return value
}

func getInt64(parent map[string]interface{}, key string) int64 {
	switch value := parent[key].(type) {
	case float64:
		return int64(value)
	case int64:
		return value
	case int:
		return int64(value)
	default:
		return 0
	}
}

// fetchValidatorList queries rippled for validator data
func (f *Fetcher) fetchValidatorList(ctx context.Context) (interface{}, error) {
	// Create HTTP client with timeout
	client := &http.Client{
		Timeout: 30 * time.Second,
	}

	var lastErr error
	maxRetries := 3
	for _, validatorListURL := range f.validatorListSites {
		for attempt := 0; attempt < maxRetries; attempt++ {
			if attempt > 0 {
				// Exponential backoff
				backoff := time.Duration(1<<uint(attempt-1)) * time.Second
				f.logger.WithFields(logrus.Fields{
					"attempt": attempt,
					"backoff": backoff,
					"url":     validatorListURL,
				}).Debug("Retrying validator list fetch")
				select {
				case <-time.After(backoff):
				case <-ctx.Done():
					return nil, ctx.Err()
				}
			}

			// Create HTTP request
			req, err := http.NewRequestWithContext(ctx, "GET", validatorListURL, nil)
			if err != nil {
				return nil, fmt.Errorf("failed to create request: %w", err)
			}
			req.Header.Set("Accept", "application/json")

			// Send request
			resp, err := client.Do(req)
			if err != nil {
				lastErr = fmt.Errorf("failed to fetch validator list: %w", err)
				f.logger.WithError(err).WithFields(logrus.Fields{
					"attempt": attempt + 1,
					"url":     validatorListURL,
				}).Warn("Validator list fetch failed")
				continue
			}
			if resp.StatusCode != http.StatusOK {
				resp.Body.Close()
				lastErr = fmt.Errorf("validator list site returned status %d", resp.StatusCode)
				f.logger.WithFields(logrus.Fields{
					"status":  resp.StatusCode,
					"attempt": attempt + 1,
					"url":     validatorListURL,
				}).Warn("Validator list fetch failed with bad status")
				continue
			}

			// Parse response
			var result map[string]interface{}
			if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
				resp.Body.Close()
				lastErr = fmt.Errorf("failed to parse validator list: %w", err)
				f.logger.WithError(err).WithFields(logrus.Fields{
					"attempt": attempt + 1,
					"url":     validatorListURL,
				}).Warn("Validator list parse failed")
				continue
			}
			resp.Body.Close()

			// Decode the base64 blob containing the validator list
			blobStr, ok := result["blob"].(string)
			if !ok {
				lastErr = fmt.Errorf("no blob field in validator list response")
				f.logger.WithFields(logrus.Fields{
					"attempt": attempt + 1,
					"url":     validatorListURL,
				}).Warn("No blob field in validator list response")
				continue
			}

			blobData, err := base64.StdEncoding.DecodeString(blobStr)
			if err != nil {
				lastErr = fmt.Errorf("failed to decode base64 blob: %w", err)
				f.logger.WithError(err).WithFields(logrus.Fields{
					"attempt": attempt + 1,
					"url":     validatorListURL,
				}).Warn("Base64 decode failed")
				continue
			}

			// Parse the decoded blob as JSON
			var blobResult map[string]interface{}
			if err := json.Unmarshal(blobData, &blobResult); err != nil {
				lastErr = fmt.Errorf("failed to parse decoded blob: %w", err)
				f.logger.WithError(err).WithFields(logrus.Fields{
					"attempt": attempt + 1,
					"url":     validatorListURL,
				}).Warn("Blob parse failed")
				continue
			}

			return blobResult, nil
		}
	}

	return nil, fmt.Errorf("failed after %d attempts: %w", maxRetries, lastErr)
}

func (f *Fetcher) fetchTrustedValidatorsFromRippled(ctx context.Context) ([]*models.Validator, map[string]struct{}, error) {
	resp, err := f.client.Command(ctx, "validators", map[string]interface{}{})
	if err != nil {
		return nil, nil, err
	}

	respMap, ok := resp.(map[string]interface{})
	if !ok {
		return nil, nil, fmt.Errorf("unexpected validators response format")
	}
	resultMap, ok := respMap["result"].(map[string]interface{})
	if !ok {
		return nil, nil, fmt.Errorf("validators response missing result")
	}
	keysRaw, _ := resultMap["trusted_validator_keys"].([]interface{})
	rawKeys := make([]interface{}, 0, len(keysRaw))
	rawKeys = append(rawKeys, keysRaw...)

	// During startup/bootstrap, trusted_validator_keys may be empty.
	// Fall back to publisher list keys so we can still map validator metadata.
	if len(rawKeys) == 0 {
		if publisherLists, ok := resultMap["publisher_lists"].([]interface{}); ok {
			for _, listRaw := range publisherLists {
				listMap, ok := listRaw.(map[string]interface{})
				if !ok {
					continue
				}
				members, ok := listMap["list"].([]interface{})
				if !ok {
					continue
				}
				rawKeys = append(rawKeys, members...)
			}
		}
	}

	out := make([]*models.Validator, 0, len(rawKeys))
	keySet := make(map[string]struct{}, len(rawKeys))
	now := time.Now().Unix()

	for _, raw := range rawKeys {
		key, ok := raw.(string)
		if !ok || key == "" {
			continue
		}
		keySet[key] = struct{}{}
		out = append(out, &models.Validator{
			Address:     key,
			PublicKey:   key,
			Name:        key,
			Network:     f.network,
			LastUpdated: now,
			IsActive:    true,
			CountryCode: "XX",
			City:        "Unknown",
		})
	}

	if len(keySet) == 0 {
		return nil, nil, fmt.Errorf("validators response did not include trusted or publisher list keys")
	}

	return out, keySet, nil
}

func (f *Fetcher) applySecondaryRegistryDomains(ctx context.Context, validators []*models.Validator, trustedSet map[string]struct{}) ([]*models.Validator, error) {
	registryURL := strings.TrimSpace(f.secondaryRegistryURL)
	if registryURL == "" {
		return validators, nil
	}
	if _, err := url.ParseRequestURI(registryURL); err != nil {
		return validators, fmt.Errorf("invalid secondary registry URL: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, registryURL, nil)
	if err != nil {
		return validators, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return validators, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return validators, fmt.Errorf("secondary registry returned status %d", resp.StatusCode)
	}

	var entries []struct {
		MasterKey    string `json:"master_key"`
		Chain        string `json:"chain"`
		Domain       string `json:"domain"`
		DomainLegacy string `json:"domain_legacy"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&entries); err != nil {
		return validators, err
	}

	byAddress := make(map[string]*models.Validator, len(validators))
	for _, v := range validators {
		if v != nil && v.Address != "" {
			byAddress[v.Address] = v
		}
	}

	now := time.Now().Unix()
	for _, entry := range entries {
		if entry.Chain != "" && entry.Chain != "main" {
			continue
		}
		if trustedSet != nil {
			if _, ok := trustedSet[entry.MasterKey]; !ok {
				continue
			}
		}

		domain := strings.TrimSpace(entry.Domain)
		if domain == "" {
			domain = strings.TrimSpace(entry.DomainLegacy)
		}
		if domain == "" {
			continue
		}

		if existing, ok := byAddress[entry.MasterKey]; ok {
			if existing.Domain == "" {
				existing.Domain = domain
				if existing.Name == "" || existing.Name == existing.Address {
					existing.Name = domain
				}
			}
			continue
		}

		v := &models.Validator{
			Address:     entry.MasterKey,
			PublicKey:   entry.MasterKey,
			Domain:      domain,
			Name:        domain,
			Network:     f.network,
			LastUpdated: now,
			IsActive:    true,
			CountryCode: "XX",
			City:        "Unknown",
		}
		validators = append(validators, v)
		byAddress[v.Address] = v
	}

	return validators, nil
}

func mergeValidators(primary []*models.Validator, secondary []*models.Validator) []*models.Validator {
	out := make([]*models.Validator, 0, len(primary)+len(secondary))
	seen := make(map[string]struct{}, len(primary)+len(secondary))

	for _, v := range append(primary, secondary...) {
		if v == nil || v.Address == "" {
			continue
		}
		if _, ok := seen[v.Address]; ok {
			continue
		}
		seen[v.Address] = struct{}{}
		out = append(out, v)
	}

	return out
}

// parseValidators extracts validator information from validator list response
func (f *Fetcher) parseValidators(data interface{}) ([]*models.Validator, error) {
	validators := make([]*models.Validator, 0)

	dataMap, ok := data.(map[string]interface{})
	if !ok {
		return validators, fmt.Errorf("unexpected response format")
	}

	// Extract validators array from the response
	// Expected format from validator list site:
	// { "validators": [ { "validation_public_key": "...", "domain": "...", ... }, ... ] }
	validatorsRaw, ok := dataMap["validators"]
	if !ok {
		// Some validator list sites return directly as an array
		if validatorsArray, ok := dataMap["data"]; ok {
			validatorsRaw = validatorsArray
		} else {
			return validators, fmt.Errorf("no validators field found in response")
		}
	}

	validatorsArray, ok := validatorsRaw.([]interface{})
	if !ok {
		return validators, fmt.Errorf("validators not in expected format")
	}

	for _, v := range validatorsArray {
		validator, err := f.parseValidator(v)
		if err != nil {
			f.logger.WithError(err).Warn("Failed to parse individual validator")
			continue
		}
		validators = append(validators, validator)
	}

	return validators, nil
}

// parseValidator converts a raw validator entry to a Validator model
func (f *Fetcher) parseValidator(raw interface{}) (*models.Validator, error) {
	rawMap, ok := raw.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("validator entry is not a map")
	}

	v := &models.Validator{
		Network:     f.network,
		LastUpdated: time.Now().Unix(),
		IsActive:    true,
	}

	// Extract public key (hex string)
	if pubKey, ok := rawMap["validation_public_key"].(string); ok {
		v.PublicKey = pubKey
	}

	// Extract domain
	if domain, ok := rawMap["domain"].(string); ok {
		v.Domain = domain
		v.Name = domain // Use domain as name if no separate name field
	}

	// Extract name if available
	if name, ok := rawMap["name"].(string); ok {
		v.Name = name
	}

	// Extract validator address if available (some lists provide it)
	if address, ok := rawMap["address"].(string); ok {
		v.Address = address
	} else if v.PublicKey != "" {
		// Use public key as identifier if address not available
		v.Address = v.PublicKey
	}

	// Set default geolocation (will be enriched later)
	v.Latitude = 0.0
	v.Longitude = 0.0
	v.CountryCode = "XX"
	v.City = "Unknown"

	return v, nil
}

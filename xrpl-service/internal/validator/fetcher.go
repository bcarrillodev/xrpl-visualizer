package validator

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/brandon/xrpl-validator-service/internal/models"
	"github.com/brandon/xrpl-validator-service/internal/xrpl"
	"github.com/sirupsen/logrus"
)

const (
	validatorListCacheTTL     = 10 * time.Minute
	secondaryRegistryCacheTTL = 30 * time.Minute
	defaultSourceCooldown     = 2 * time.Minute
	defaultRateLimitCooldown  = 10 * time.Minute
)

type validatorListCacheEntry struct {
	payload   map[string]interface{}
	expiresAt time.Time
}

type secondaryRegistryEntry struct {
	MasterKey    string `json:"master_key"`
	Chain        string `json:"chain"`
	Domain       string `json:"domain"`
	DomainLegacy string `json:"domain_legacy"`
}

type secondaryRegistryCacheEntry struct {
	entries   []secondaryRegistryEntry
	expiresAt time.Time
}

type validatorMetadataEntry struct {
	Address     string  `json:"address"`
	Domain      string  `json:"domain"`
	Name        string  `json:"name"`
	Latitude    float64 `json:"latitude"`
	Longitude   float64 `json:"longitude"`
	CountryCode string  `json:"country_code"`
	City        string  `json:"city"`
	LastSeenAt  int64   `json:"last_seen_at"`
}

type validatorMetadataCacheFile struct {
	Version int                                `json:"version"`
	Entries map[string]*validatorMetadataEntry `json:"entries"`
}

const validatorMetadataCacheVersion = 1

// Fetcher handles validator data retrieval and caching
type Fetcher struct {
	client               xrpl.NodeClient
	logger               *logrus.Logger
	httpClient           *http.Client
	mu                   sync.RWMutex
	validators           map[string]*models.Validator // Address -> Validator
	lastUpdate           time.Time
	refreshInterval      time.Duration
	stopChan             chan struct{}
	geolocationProvider  GeoLocationProvider
	maxValidators        int
	validatorListSites   []string
	secondaryRegistryURL string
	metadataCachePath    string
	networkHealthRPCURLs []string
	networkHealthRetries int
	network              string
	sourceStateMu        sync.Mutex
	validatorListCache   map[string]*validatorListCacheEntry
	secondaryCache       *secondaryRegistryCacheEntry
	sourceCooldownUntil  map[string]time.Time
	metadataCache        map[string]*validatorMetadataEntry
}

// GeoLocationProvider defines the interface for geolocation enrichment
type GeoLocationProvider interface {
	// EnrichValidator adds geolocation data to a validator
	EnrichValidator(validator *models.Validator) error
}

// NewFetcher creates a new validator fetcher
func NewFetcher(
	client xrpl.NodeClient,
	refreshInterval time.Duration,
	geoProvider GeoLocationProvider,
	validatorListSites []string,
	secondaryRegistryURL string,
	metadataCachePath string,
	networkHealthRPCURLs []string,
	networkHealthRetries int,
	network string,
	logger *logrus.Logger,
) *Fetcher {
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
	if strings.TrimSpace(metadataCachePath) == "" {
		metadataCachePath = "data/validator-metadata-cache.json"
	}
	endpoints := make([]string, 0, len(networkHealthRPCURLs))
	seenEndpoints := make(map[string]struct{}, len(networkHealthRPCURLs))
	for _, endpoint := range networkHealthRPCURLs {
		trimmed := strings.TrimSpace(endpoint)
		if trimmed == "" {
			continue
		}
		if _, exists := seenEndpoints[trimmed]; exists {
			continue
		}
		seenEndpoints[trimmed] = struct{}{}
		endpoints = append(endpoints, trimmed)
	}
	if len(endpoints) == 0 {
		endpoints = []string{"https://xrplcluster.com", "https://s2.ripple.com:51234"}
	}
	if networkHealthRetries <= 0 {
		networkHealthRetries = 2
	}
	fetcher := &Fetcher{
		client:               client,
		logger:               logger,
		httpClient:           &http.Client{Timeout: 30 * time.Second},
		validators:           make(map[string]*models.Validator),
		refreshInterval:      refreshInterval,
		stopChan:             make(chan struct{}),
		geolocationProvider:  geoProvider,
		maxValidators:        1000, // Limit to prevent memory exhaustion
		validatorListSites:   sites,
		secondaryRegistryURL: secondaryRegistryURL,
		metadataCachePath:    metadataCachePath,
		networkHealthRPCURLs: endpoints,
		networkHealthRetries: networkHealthRetries,
		network:              strings.ToLower(network),
		validatorListCache:   make(map[string]*validatorListCacheEntry),
		sourceCooldownUntil:  make(map[string]time.Time),
		metadataCache:        make(map[string]*validatorMetadataEntry),
	}
	fetcher.loadMetadataCache()
	return fetcher
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

// Fetch retrieves current validators from XRPL
func (f *Fetcher) Fetch(ctx context.Context) error {
	f.logger.Debug("Fetching validators from XRPL")

	// Query XRPL for validator information
	// Using ledger_closed subscription to get updated validator set
	result, err := f.fetchValidatorList(ctx)
	if err != nil {
		return fmt.Errorf("failed to fetch validator list: %w", err)
	}

	validators, err := f.parseValidators(result)
	if err != nil {
		return fmt.Errorf("failed to parse validators: %w", err)
	}

	trustedValidators, trustedSet, err := f.fetchTrustedValidatorsFromXRPL(ctx)
	if err != nil {
		f.logger.WithError(err).Warn("Failed to fetch trusted validators from XRPL")
	}
	validators = mergeValidators(validators, trustedValidators)

	validators, err = f.applySecondaryRegistryDomains(ctx, validators, trustedSet)
	if err != nil {
		f.logger.WithError(err).Warn("Failed to enrich validators from secondary registry")
	}

	// Apply previously persisted metadata before live enrichment to maximize coverage.
	f.applyPersistedMetadata(validators)

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

	// Coverage lock: never regress from known mapped coordinates to zeroed coordinates.
	f.preserveMappedCoverage(validators)

	// Update cache
	f.mu.Lock()
	f.validators = make(map[string]*models.Validator)
	for _, v := range validators {
		f.validators[v.Address] = v
	}
	f.lastUpdate = time.Now()
	f.mu.Unlock()

	f.updatePersistedMetadata(validators)

	f.logger.WithField("count", len(validators)).Info("Validators updated")
	return nil
}

func (f *Fetcher) preserveMappedCoverage(validators []*models.Validator) {
	previous := make(map[string]*models.Validator)

	f.mu.RLock()
	for k, v := range f.validators {
		if v != nil {
			previous[k] = v
		}
	}
	f.mu.RUnlock()

	for _, v := range validators {
		if v == nil || v.Address == "" {
			continue
		}
		// Already mapped; keep fresh value.
		if v.Latitude != 0 || v.Longitude != 0 {
			continue
		}

		// Prefer prior in-memory mapped value if present.
		if prev, ok := previous[v.Address]; ok && (prev.Latitude != 0 || prev.Longitude != 0) {
			v.Latitude = prev.Latitude
			v.Longitude = prev.Longitude
			if v.CountryCode == "" || v.CountryCode == "XX" {
				v.CountryCode = prev.CountryCode
			}
			if v.City == "" || v.City == "Unknown" {
				v.City = prev.City
			}
			continue
		}

		// Fall back to persisted metadata if in-memory has no mapped value.
		f.sourceStateMu.Lock()
		entry := f.metadataCache[v.Address]
		f.sourceStateMu.Unlock()
		if entry != nil && (entry.Latitude != 0 || entry.Longitude != 0) {
			v.Latitude = entry.Latitude
			v.Longitude = entry.Longitude
			if v.CountryCode == "" || v.CountryCode == "XX" {
				v.CountryCode = entry.CountryCode
			}
			if v.City == "" || v.City == "Unknown" {
				v.City = entry.City
			}
		}
	}
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

// GetServerStatus retrieves current XRPL server health information.
func (f *Fetcher) GetServerStatus(ctx context.Context) (*models.ServerStatus, error) {
	var endpointErrors []string
	for _, endpoint := range f.networkHealthRPCURLs {
		status, err := f.getServerStatusFromEndpoint(ctx, endpoint)
		if err == nil {
			return status, nil
		}
		endpointErrors = append(endpointErrors, fmt.Sprintf("%s: %v", endpoint, err))
	}

	if len(endpointErrors) > 0 {
		return nil, fmt.Errorf("all network health endpoints failed: %s", strings.Join(endpointErrors, " | "))
	}

	result, err := f.client.GetServerInfo(ctx)
	if err != nil {
		return nil, err
	}
	return parseServerStatusResult(result)
}

func (f *Fetcher) getServerStatusFromEndpoint(ctx context.Context, endpoint string) (*models.ServerStatus, error) {
	var lastErr error
	for attempt := 1; attempt <= f.networkHealthRetries; attempt++ {
		result, err := f.fetchServerInfoFromJSONRPC(ctx, endpoint)
		if err == nil {
			status, parseErr := parseServerStatusResult(result)
			if parseErr == nil {
				return status, nil
			}
			err = parseErr
		}
		lastErr = err
		if attempt == f.networkHealthRetries {
			break
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(time.Duration(attempt) * 150 * time.Millisecond):
		}
	}
	return nil, lastErr
}

func (f *Fetcher) fetchServerInfoFromJSONRPC(ctx context.Context, endpoint string) (map[string]interface{}, error) {
	requestPayload := map[string]interface{}{
		"method":  "server_info",
		"params":  []interface{}{map[string]interface{}{}},
		"id":      1,
		"jsonrpc": "2.0",
	}
	body, err := json.Marshal(requestPayload)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := f.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		snippet, _ := io.ReadAll(io.LimitReader(resp.Body, 120))
		return nil, fmt.Errorf("http %d: %s", resp.StatusCode, strings.TrimSpace(string(snippet)))
	}

	var parsed map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return nil, err
	}
	if errorResult, ok := parsed["error"]; ok {
		return nil, fmt.Errorf("JSON-RPC error: %v", errorResult)
	}
	return parsed, nil
}

func parseServerStatusResult(result interface{}) (*models.ServerStatus, error) {
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

	return &models.ServerStatus{
		Connected:       true,
		ServerState:     getString(info, "server_state"),
		LedgerIndex:     uint32(getInt64(getMap(info, "validated_ledger"), "seq")),
		NetworkID:       uint16(getInt64(info, "network_id")),
		PeerCount:       int(getInt64(info, "peers")),
		CompleteLedgers: getString(info, "complete_ledgers"),
		Uptime:          getInt64(info, "uptime"),
		LastSync:        time.Now().Unix(),
	}, nil
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

// fetchValidatorList queries XRPL for validator data
func (f *Fetcher) fetchValidatorList(ctx context.Context) (interface{}, error) {
	var lastErr error
	maxRetries := 3
	for _, validatorListURL := range f.validatorListSites {
		if until, ok := f.getSourceCooldown("validator-list:" + validatorListURL); ok && time.Now().Before(until) {
			f.logger.WithFields(logrus.Fields{
				"url":      validatorListURL,
				"cooldown": until.Format(time.RFC3339),
			}).Warn("Skipping validator list source while in cooldown")
			if cached, ok := f.getValidatorListCache(validatorListURL, true); ok {
				return cached, nil
			}
			continue
		}
		if cached, ok := f.getValidatorListCache(validatorListURL, false); ok {
			return cached, nil
		}

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
			resp, err := f.httpClient.Do(req)
			if err != nil {
				lastErr = fmt.Errorf("failed to fetch validator list: %w", err)
				f.logger.WithError(err).WithFields(logrus.Fields{
					"attempt": attempt + 1,
					"url":     validatorListURL,
				}).Warn("Validator list fetch failed")
				continue
			}
			if resp.StatusCode != http.StatusOK {
				if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode == http.StatusServiceUnavailable {
					f.setSourceCooldown(
						"validator-list:"+validatorListURL,
						cooldownFromResponse(resp, defaultRateLimitCooldown),
					)
				}
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

			f.setValidatorListCache(validatorListURL, blobResult)
			return blobResult, nil
		}
	}

	for _, validatorListURL := range f.validatorListSites {
		if cached, ok := f.getValidatorListCache(validatorListURL, true); ok {
			f.logger.WithField("url", validatorListURL).Warn("Using stale validator list cache after source failures")
			return cached, nil
		}
	}

	return nil, fmt.Errorf("failed after %d attempts: %w", maxRetries, lastErr)
}

func (f *Fetcher) fetchTrustedValidatorsFromXRPL(ctx context.Context) ([]*models.Validator, map[string]struct{}, error) {
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

	if until, ok := f.getSourceCooldown("registry:" + registryURL); ok && time.Now().Before(until) {
		if cached, ok := f.getSecondaryRegistryCache(true); ok {
			return f.mergeSecondaryRegistry(validators, trustedSet, cached), nil
		}
		return validators, fmt.Errorf("secondary registry in cooldown until %s", until.Format(time.RFC3339))
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, registryURL, nil)
	if err != nil {
		return validators, err
	}
	resp, err := f.httpClient.Do(req)
	if err != nil {
		if cached, ok := f.getSecondaryRegistryCache(true); ok {
			f.logger.WithError(err).Warn("Using stale secondary registry cache after fetch error")
			return f.mergeSecondaryRegistry(validators, trustedSet, cached), nil
		}
		return validators, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode == http.StatusServiceUnavailable {
			f.setSourceCooldown("registry:"+registryURL, cooldownFromResponse(resp, defaultRateLimitCooldown))
		} else {
			f.setSourceCooldown("registry:"+registryURL, time.Now().Add(defaultSourceCooldown))
		}
		if cached, ok := f.getSecondaryRegistryCache(true); ok {
			f.logger.WithField("status", resp.StatusCode).Warn("Using stale secondary registry cache after non-OK status")
			return f.mergeSecondaryRegistry(validators, trustedSet, cached), nil
		}
		return validators, fmt.Errorf("secondary registry returned status %d", resp.StatusCode)
	}

	var entries []secondaryRegistryEntry
	if err := json.NewDecoder(resp.Body).Decode(&entries); err != nil {
		if cached, ok := f.getSecondaryRegistryCache(true); ok {
			f.logger.WithError(err).Warn("Using stale secondary registry cache after parse error")
			return f.mergeSecondaryRegistry(validators, trustedSet, cached), nil
		}
		return validators, err
	}
	f.setSecondaryRegistryCache(entries)

	return f.mergeSecondaryRegistry(validators, trustedSet, entries), nil
}

func (f *Fetcher) mergeSecondaryRegistry(validators []*models.Validator, trustedSet map[string]struct{}, entries []secondaryRegistryEntry) []*models.Validator {
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

	return validators
}

func (f *Fetcher) getSourceCooldown(key string) (time.Time, bool) {
	f.sourceStateMu.Lock()
	defer f.sourceStateMu.Unlock()
	until, ok := f.sourceCooldownUntil[key]
	return until, ok
}

func (f *Fetcher) setSourceCooldown(key string, until time.Time) {
	f.sourceStateMu.Lock()
	f.sourceCooldownUntil[key] = until
	f.sourceStateMu.Unlock()
}

func (f *Fetcher) getValidatorListCache(source string, allowStale bool) (map[string]interface{}, bool) {
	f.sourceStateMu.Lock()
	defer f.sourceStateMu.Unlock()
	entry, ok := f.validatorListCache[source]
	if !ok || entry == nil {
		return nil, false
	}
	if !allowStale && time.Now().After(entry.expiresAt) {
		return nil, false
	}
	return entry.payload, true
}

func (f *Fetcher) setValidatorListCache(source string, payload map[string]interface{}) {
	f.sourceStateMu.Lock()
	f.validatorListCache[source] = &validatorListCacheEntry{
		payload:   payload,
		expiresAt: time.Now().Add(validatorListCacheTTL),
	}
	f.sourceStateMu.Unlock()
}

func (f *Fetcher) getSecondaryRegistryCache(allowStale bool) ([]secondaryRegistryEntry, bool) {
	f.sourceStateMu.Lock()
	defer f.sourceStateMu.Unlock()
	entry := f.secondaryCache
	if entry == nil {
		return nil, false
	}
	if !allowStale && time.Now().After(entry.expiresAt) {
		return nil, false
	}
	out := make([]secondaryRegistryEntry, 0, len(entry.entries))
	out = append(out, entry.entries...)
	return out, true
}

func (f *Fetcher) setSecondaryRegistryCache(entries []secondaryRegistryEntry) {
	out := make([]secondaryRegistryEntry, 0, len(entries))
	out = append(out, entries...)
	f.sourceStateMu.Lock()
	f.secondaryCache = &secondaryRegistryCacheEntry{
		entries:   out,
		expiresAt: time.Now().Add(secondaryRegistryCacheTTL),
	}
	f.sourceStateMu.Unlock()
}

func cooldownFromResponse(resp *http.Response, fallback time.Duration) time.Time {
	retryAfter := strings.TrimSpace(resp.Header.Get("Retry-After"))
	if retryAfter == "" {
		return time.Now().Add(fallback)
	}
	if secs, err := strconv.Atoi(retryAfter); err == nil && secs > 0 {
		return time.Now().Add(time.Duration(secs) * time.Second)
	}
	if t, err := time.Parse(time.RFC1123, retryAfter); err == nil {
		return t
	}
	if t, err := time.Parse(time.RFC1123Z, retryAfter); err == nil {
		return t
	}
	return time.Now().Add(fallback)
}

func (f *Fetcher) applyPersistedMetadata(validators []*models.Validator) {
	f.sourceStateMu.Lock()
	defer f.sourceStateMu.Unlock()

	for _, v := range validators {
		if v == nil || v.Address == "" {
			continue
		}
		entry, ok := f.metadataCache[v.Address]
		if !ok || entry == nil {
			continue
		}
		if v.Domain == "" {
			v.Domain = entry.Domain
		}
		if v.Name == "" || v.Name == v.Address {
			v.Name = entry.Name
		}
		if (v.Latitude == 0 && v.Longitude == 0) && (entry.Latitude != 0 || entry.Longitude != 0) {
			v.Latitude = entry.Latitude
			v.Longitude = entry.Longitude
			v.CountryCode = entry.CountryCode
			v.City = entry.City
		}
	}
}

func (f *Fetcher) updatePersistedMetadata(validators []*models.Validator) {
	changed := false
	now := time.Now().Unix()

	f.sourceStateMu.Lock()
	for _, v := range validators {
		if v == nil || v.Address == "" {
			continue
		}
		entry, ok := f.metadataCache[v.Address]
		if !ok || entry == nil {
			entry = &validatorMetadataEntry{Address: v.Address}
			f.metadataCache[v.Address] = entry
			changed = true
		}

		if v.Domain != "" && entry.Domain != v.Domain {
			entry.Domain = v.Domain
			changed = true
		}
		if v.Name != "" && entry.Name != v.Name {
			entry.Name = v.Name
			changed = true
		}
		if (v.Latitude != 0 || v.Longitude != 0) &&
			(entry.Latitude != v.Latitude || entry.Longitude != v.Longitude || entry.City != v.City || entry.CountryCode != v.CountryCode) {
			entry.Latitude = v.Latitude
			entry.Longitude = v.Longitude
			entry.CountryCode = v.CountryCode
			entry.City = v.City
			changed = true
		}
		if entry.LastSeenAt != now {
			entry.LastSeenAt = now
			changed = true
		}
	}
	f.sourceStateMu.Unlock()

	if changed {
		if err := f.persistMetadataCache(); err != nil {
			f.logger.WithError(err).Warn("Failed to persist validator metadata cache")
		}
	}
}

func (f *Fetcher) loadMetadataCache() {
	data, err := os.ReadFile(f.metadataCachePath)
	if err != nil {
		if !os.IsNotExist(err) {
			f.logger.WithError(err).WithField("path", f.metadataCachePath).Warn("Failed to read validator metadata cache")
		}
		return
	}

	var payload validatorMetadataCacheFile
	if err := json.Unmarshal(data, &payload); err != nil {
		f.logger.WithError(err).WithField("path", f.metadataCachePath).Warn("Failed to parse validator metadata cache")
		return
	}
	if payload.Version != validatorMetadataCacheVersion || payload.Entries == nil {
		return
	}

	f.sourceStateMu.Lock()
	f.metadataCache = payload.Entries
	f.sourceStateMu.Unlock()

	f.logger.WithFields(logrus.Fields{
		"path":    f.metadataCachePath,
		"entries": len(payload.Entries),
	}).Info("Loaded validator metadata cache")
}

func (f *Fetcher) persistMetadataCache() error {
	f.sourceStateMu.Lock()
	payload := validatorMetadataCacheFile{
		Version: validatorMetadataCacheVersion,
		Entries: make(map[string]*validatorMetadataEntry, len(f.metadataCache)),
	}
	for key, entry := range f.metadataCache {
		if entry == nil {
			continue
		}
		copy := *entry
		payload.Entries[key] = &copy
	}
	f.sourceStateMu.Unlock()

	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(f.metadataCachePath), 0o755); err != nil {
		return err
	}
	tmpPath := f.metadataCachePath + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmpPath, f.metadataCachePath)
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

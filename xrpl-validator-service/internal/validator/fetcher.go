package validator

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/brandon/xrpl-validator-service/internal/models"
	"github.com/brandon/xrpl-validator-service/internal/rippled"
	"github.com/sirupsen/logrus"
)

// Fetcher handles validator data retrieval and caching
type Fetcher struct {
	client              rippled.RippledClient
	logger              *logrus.Logger
	mu                  sync.RWMutex
	validators          map[string]*models.Validator // Address -> Validator
	lastUpdate          time.Time
	refreshInterval     time.Duration
	stopChan            chan struct{}
	geolocationProvider GeoLocationProvider
	maxValidators       int
}

// GeoLocationProvider defines the interface for geolocation enrichment
type GeoLocationProvider interface {
	// EnrichValidator adds geolocation data to a validator
	EnrichValidator(validator *models.Validator) error
}

// NewFetcher creates a new validator fetcher
func NewFetcher(client rippled.RippledClient, refreshInterval time.Duration, geoProvider GeoLocationProvider, logger *logrus.Logger) *Fetcher {
	if logger == nil {
		logger = logrus.New()
	}
	return &Fetcher{
		client:              client,
		logger:              logger,
		validators:          make(map[string]*models.Validator),
		refreshInterval:     refreshInterval,
		stopChan:            make(chan struct{}),
		geolocationProvider: geoProvider,
		maxValidators:       1000, // Limit to prevent memory exhaustion
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

	// Limit the number of validators to prevent memory exhaustion
	if len(validators) > f.maxValidators {
		f.logger.WithFields(logrus.Fields{
			"fetched": len(validators),
			"limit":   f.maxValidators,
		}).Warn("Limiting validators to prevent memory exhaustion")
		validators = validators[:f.maxValidators]
	}

	// Enrich validators with geolocation data
	if noOp, ok := f.geolocationProvider.(*NoOpGeoLocationProvider); ok {
		noOp.AssignDemoLocations(validators)
	} else {
		missingGeoValidators := make([]*models.Validator, 0)
		for _, v := range validators {
			if f.geolocationProvider != nil {
				if err := f.geolocationProvider.EnrichValidator(v); err != nil {
					f.logger.WithError(err).WithField("address", v.Address).Warn("Failed to enrich validator geolocation")
				}
			}

			if v.Latitude == 0 && v.Longitude == 0 {
				missingGeoValidators = append(missingGeoValidators, v)
			}
		}

		if len(missingGeoValidators) > 0 {
			fallback := NewNoOpGeoLocationProvider(f.logger)
			fallback.AssignDemoLocations(missingGeoValidators)
			f.logger.WithField("count", len(missingGeoValidators)).Info("Applied demo geolocation fallback for validators without coordinates")
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

// fetchValidatorList queries rippled for validator data
func (f *Fetcher) fetchValidatorList(ctx context.Context) (interface{}, error) {
	// For Altnet, fetch validator list from the validator list site
	// The validator list site is configured in rippled's validators.txt
	// For Altnet: https://vl.altnet.rippletest.net

	validatorListURL := "https://vl.altnet.rippletest.net"

	// Create HTTP client with timeout
	client := &http.Client{
		Timeout: 30 * time.Second,
	}

	var lastErr error
	maxRetries := 3
	for attempt := 0; attempt < maxRetries; attempt++ {
		if attempt > 0 {
			// Exponential backoff
			backoff := time.Duration(1<<uint(attempt-1)) * time.Second
			f.logger.WithFields(logrus.Fields{
				"attempt": attempt,
				"backoff": backoff,
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
			f.logger.WithError(err).WithField("attempt", attempt+1).Warn("Validator list fetch failed")
			continue
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			lastErr = fmt.Errorf("validator list site returned status %d", resp.StatusCode)
			f.logger.WithFields(logrus.Fields{
				"status":  resp.StatusCode,
				"attempt": attempt + 1,
			}).Warn("Validator list fetch failed with bad status")
			continue
		}

		// Parse response
		var result map[string]interface{}
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			lastErr = fmt.Errorf("failed to parse validator list: %w", err)
			f.logger.WithError(err).WithField("attempt", attempt+1).Warn("Validator list parse failed")
			continue
		}

		// Decode the base64 blob containing the validator list
		blobStr, ok := result["blob"].(string)
		if !ok {
			lastErr = fmt.Errorf("no blob field in validator list response")
			f.logger.WithField("attempt", attempt+1).Warn("No blob field in validator list response")
			continue
		}

		blobData, err := base64.StdEncoding.DecodeString(blobStr)
		if err != nil {
			lastErr = fmt.Errorf("failed to decode base64 blob: %w", err)
			f.logger.WithError(err).WithField("attempt", attempt+1).Warn("Base64 decode failed")
			continue
		}

		// Parse the decoded blob as JSON
		var blobResult map[string]interface{}
		if err := json.Unmarshal(blobData, &blobResult); err != nil {
			lastErr = fmt.Errorf("failed to parse decoded blob: %w", err)
			f.logger.WithError(err).WithField("attempt", attempt+1).Warn("Blob parse failed")
			continue
		}

		return blobResult, nil
	}

	return nil, fmt.Errorf("failed after %d attempts: %w", maxRetries, lastErr)
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
		Network:     "altnet",
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

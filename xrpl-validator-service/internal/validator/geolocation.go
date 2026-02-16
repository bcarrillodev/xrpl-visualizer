package validator

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/brandon/xrpl-validator-service/internal/models"
	"github.com/sirupsen/logrus"
)

var demoLocations = []struct {
	lat, lng      float64
	city, country string
}{
	{51.5074, -0.1278, "London", "GB"},
	{55.7558, 37.6173, "Moscow", "RU"},
	{40.7128, -74.0060, "New York", "US"},
	{35.6762, 139.6503, "Tokyo", "JP"},
	{48.8566, 2.3522, "Paris", "FR"},
	{-33.8688, 151.2093, "Sydney", "AU"},
}

// NoOpGeoLocationProvider is a stub implementation that doesn't enrich data
type NoOpGeoLocationProvider struct {
	logger *logrus.Logger
}

// NewNoOpGeoLocationProvider creates a new no-op geolocation provider
func NewNoOpGeoLocationProvider(logger *logrus.Logger) *NoOpGeoLocationProvider {
	return &NoOpGeoLocationProvider{logger: logger}
}

// EnrichValidator assigns demo coordinates for visualization
func (p *NoOpGeoLocationProvider) EnrichValidator(validator *models.Validator) error {
	// Fallback for single-assign usage; batch assignment is preferred.
	location := demoLocations[0]
	validator.Latitude = location.lat
	validator.Longitude = location.lng
	validator.CountryCode = location.country
	validator.City = location.city

	p.logger.WithFields(logrus.Fields{
		"address": validator.Address,
		"city":    location.city,
		"country": location.country,
	}).Debug("Assigned demo geolocation to validator")

	return nil
}

// AssignDemoLocations assigns demo locations in a round-robin pass.
func (p *NoOpGeoLocationProvider) AssignDemoLocations(validators []*models.Validator) {
	for i, v := range validators {
		location := demoLocations[i%len(demoLocations)]
		v.Latitude = location.lat
		v.Longitude = location.lng
		v.CountryCode = location.country
		v.City = location.city

		p.logger.WithFields(logrus.Fields{
			"address": v.Address,
			"city":    location.city,
			"country": location.country,
		}).Debug("Assigned demo geolocation to validator")
	}
}

// RealGeoLocationProvider uses IP geolocation API for real data
type RealGeoLocationProvider struct {
	logger *logrus.Logger
	client *http.Client
}

// NewRealGeoLocationProvider creates a new real geolocation provider
func NewRealGeoLocationProvider(logger *logrus.Logger) *RealGeoLocationProvider {
	return &RealGeoLocationProvider{
		logger: logger,
		client: &http.Client{Timeout: 10 * time.Second},
	}
}

// EnrichValidator attempts to get real geolocation data
func (p *RealGeoLocationProvider) EnrichValidator(validator *models.Validator) error {
	if validator.Domain == "" {
		return fmt.Errorf("no domain available for geolocation")
	}

	// Extract IP from domain
	ips, err := net.LookupIP(validator.Domain)
	if err != nil || len(ips) == 0 {
		p.logger.WithError(err).WithField("domain", validator.Domain).Warn("Failed to resolve domain")
		return fmt.Errorf("failed to resolve domain %s: %w", validator.Domain, err)
	}

	ip := ips[0].String()

	// Query IP geolocation API
	url := fmt.Sprintf("http://ip-api.com/json/%s", ip)
	resp, err := p.client.Get(url)
	if err != nil {
		p.logger.WithError(err).WithField("ip", ip).Warn("Failed to query geolocation API")
		return fmt.Errorf("failed to query geolocation API: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("geolocation API returned status %d", resp.StatusCode)
	}

	var result struct {
		Status      string  `json:"status"`
		CountryCode string  `json:"countryCode"`
		City        string  `json:"city"`
		Lat         float64 `json:"lat"`
		Lon         float64 `json:"lon"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		p.logger.WithError(err).WithField("ip", ip).Warn("Failed to parse geolocation response")
		return fmt.Errorf("failed to parse geolocation response: %w", err)
	}

	if result.Status != "success" {
		return fmt.Errorf("geolocation API returned status: %s", result.Status)
	}

	validator.Latitude = result.Lat
	validator.Longitude = result.Lon
	validator.CountryCode = result.CountryCode
	validator.City = result.City

	p.logger.WithFields(logrus.Fields{
		"domain":  validator.Domain,
		"ip":      ip,
		"city":    result.City,
		"country": result.CountryCode,
	}).Debug("Enriched validator with real geolocation")

	return nil
}

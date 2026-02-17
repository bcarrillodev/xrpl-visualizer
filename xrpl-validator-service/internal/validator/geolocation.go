package validator

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
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

const cacheVersion = 1

type geoCacheEntry struct {
	CountryCode string  `json:"country_code"`
	City        string  `json:"city"`
	Latitude    float64 `json:"latitude"`
	Longitude   float64 `json:"longitude"`
	UpdatedAt   int64   `json:"updated_at"`
}

type geoCacheFile struct {
	Version int                       `json:"version"`
	Entries map[string]*geoCacheEntry `json:"entries"`
}

type RealGeoLocationConfig struct {
	CachePath         string
	MinLookupInterval time.Duration
	RateLimitCooldown time.Duration
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
	logger            *logrus.Logger
	client            *http.Client
	cachePath         string
	minLookupInterval time.Duration
	rateLimitCooldown time.Duration
	lastLookupAt      time.Time
	rateLimitedUntil  time.Time
	mu                sync.Mutex
	cache             map[string]*geoCacheEntry
}

// NewRealGeoLocationProvider creates a new real geolocation provider
func NewRealGeoLocationProvider(logger *logrus.Logger, cfg RealGeoLocationConfig) *RealGeoLocationProvider {
	if cfg.CachePath == "" {
		cfg.CachePath = "data/geolocation-cache.json"
	}
	if cfg.MinLookupInterval <= 0 {
		cfg.MinLookupInterval = 1200 * time.Millisecond
	}
	if cfg.RateLimitCooldown <= 0 {
		cfg.RateLimitCooldown = 15 * time.Minute
	}

	p := &RealGeoLocationProvider{
		logger:            logger,
		client:            &http.Client{Timeout: 10 * time.Second},
		cachePath:         cfg.CachePath,
		minLookupInterval: cfg.MinLookupInterval,
		rateLimitCooldown: cfg.RateLimitCooldown,
		cache:             make(map[string]*geoCacheEntry),
	}
	p.loadCache()
	return p
}

// EnrichValidator attempts to get real geolocation data
func (p *RealGeoLocationProvider) EnrichValidator(validator *models.Validator) error {
	if validator.Domain == "" {
		return fmt.Errorf("no domain available for geolocation")
	}

	domain := normalizeDomain(validator.Domain)
	if domain == "" {
		return fmt.Errorf("invalid domain")
	}

	if entry, ok := p.getCached("domain:" + domain); ok {
		applyGeo(validator, entry)
		return nil
	}

	ips, err := net.LookupIP(domain)
	if err != nil || len(ips) == 0 {
		p.logger.WithError(err).WithField("domain", domain).Warn("Failed to resolve domain")
		return fmt.Errorf("failed to resolve domain %s: %w", domain, err)
	}

	ip := pickIP(ips)
	if entry, ok := p.getCached("ip:" + ip); ok {
		applyGeo(validator, entry)
		p.setCached("domain:"+domain, entry)
		return nil
	}

	if until := p.getRateLimitUntil(); time.Now().Before(until) {
		return fmt.Errorf("geolocation lookup in cooldown until %s", until.Format(time.RFC3339))
	}

	p.waitForThrottle()

	url := fmt.Sprintf("https://ipwho.is/%s", ip)
	resp, err := p.client.Get(url)
	if err != nil {
		p.logger.WithError(err).WithField("ip", ip).Warn("Failed to query geolocation API")
		return fmt.Errorf("failed to query geolocation API: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusTooManyRequests {
		p.setRateLimitUntil(time.Now().Add(p.rateLimitCooldown))
		return fmt.Errorf("geolocation API returned status %d", resp.StatusCode)
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("geolocation API returned status %d", resp.StatusCode)
	}

	var result struct {
		Success     bool    `json:"success"`
		Message     string  `json:"message"`
		CountryCode string  `json:"country_code"`
		City        string  `json:"city"`
		Lat         float64 `json:"latitude"`
		Lon         float64 `json:"longitude"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		p.logger.WithError(err).WithField("ip", ip).Warn("Failed to parse geolocation response")
		return fmt.Errorf("failed to parse geolocation response: %w", err)
	}
	if !result.Success {
		return fmt.Errorf("geolocation API failed: %s", result.Message)
	}

	entry := &geoCacheEntry{
		CountryCode: result.CountryCode,
		City:        result.City,
		Latitude:    result.Lat,
		Longitude:   result.Lon,
		UpdatedAt:   time.Now().Unix(),
	}
	applyGeo(validator, entry)
	p.setCached("ip:"+ip, entry)
	p.setCached("domain:"+domain, entry)
	if err := p.persistCache(); err != nil {
		p.logger.WithError(err).Warn("Failed to persist geolocation cache")
	}

	p.logger.WithFields(logrus.Fields{
		"domain":  domain,
		"ip":      ip,
		"city":    result.City,
		"country": result.CountryCode,
	}).Debug("Enriched validator with real geolocation")

	return nil
}

func normalizeDomain(raw string) string {
	domain := strings.TrimSpace(raw)
	domain = strings.TrimSuffix(domain, ".")
	if host, _, err := net.SplitHostPort(domain); err == nil {
		domain = host
	}
	return strings.ToLower(strings.TrimSpace(domain))
}

func pickIP(ips []net.IP) string {
	for _, candidate := range ips {
		if candidate.To4() != nil {
			return candidate.String()
		}
	}
	return ips[0].String()
}

func applyGeo(validator *models.Validator, entry *geoCacheEntry) {
	validator.Latitude = entry.Latitude
	validator.Longitude = entry.Longitude
	validator.CountryCode = entry.CountryCode
	validator.City = entry.City
}

func (p *RealGeoLocationProvider) getCached(key string) (*geoCacheEntry, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	entry, ok := p.cache[key]
	if !ok || entry == nil {
		return nil, false
	}
	copy := *entry
	return &copy, true
}

func (p *RealGeoLocationProvider) setCached(key string, entry *geoCacheEntry) {
	p.mu.Lock()
	defer p.mu.Unlock()
	copy := *entry
	p.cache[key] = &copy
}

func (p *RealGeoLocationProvider) waitForThrottle() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.lastLookupAt.IsZero() {
		p.lastLookupAt = time.Now()
		return
	}
	nextAllowed := p.lastLookupAt.Add(p.minLookupInterval)
	now := time.Now()
	if now.Before(nextAllowed) {
		time.Sleep(nextAllowed.Sub(now))
	}
	p.lastLookupAt = time.Now()
}

func (p *RealGeoLocationProvider) getRateLimitUntil() time.Time {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.rateLimitedUntil
}

func (p *RealGeoLocationProvider) setRateLimitUntil(until time.Time) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.rateLimitedUntil = until
}

func (p *RealGeoLocationProvider) loadCache() {
	data, err := os.ReadFile(p.cachePath)
	if err != nil {
		if !os.IsNotExist(err) {
			p.logger.WithError(err).WithField("path", p.cachePath).Warn("Failed to read geolocation cache")
		}
		return
	}

	var payload geoCacheFile
	if err := json.Unmarshal(data, &payload); err != nil {
		p.logger.WithError(err).WithField("path", p.cachePath).Warn("Failed to parse geolocation cache")
		return
	}
	if payload.Version != cacheVersion || payload.Entries == nil {
		return
	}

	p.mu.Lock()
	p.cache = payload.Entries
	p.mu.Unlock()

	p.logger.WithFields(logrus.Fields{
		"path":    p.cachePath,
		"entries": len(payload.Entries),
	}).Info("Loaded geolocation cache")
}

func (p *RealGeoLocationProvider) persistCache() error {
	p.mu.Lock()
	payload := geoCacheFile{
		Version: cacheVersion,
		Entries: make(map[string]*geoCacheEntry, len(p.cache)),
	}
	for key, entry := range p.cache {
		if entry == nil {
			continue
		}
		copy := *entry
		payload.Entries[key] = &copy
	}
	p.mu.Unlock()

	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(p.cachePath), 0o755); err != nil {
		return err
	}
	tmpPath := p.cachePath + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmpPath, p.cachePath)
}

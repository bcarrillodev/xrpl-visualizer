package geolocation

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/brandon/xrpl-validator-service/internal/models"
	"github.com/brandon/xrpl-validator-service/internal/rippled"
	"github.com/oschwald/geoip2-golang"
	"github.com/sirupsen/logrus"
)

const (
	defaultCachePath         = "data/geolocation-cache.json"
	defaultGeoLiteDBPath     = "data/GeoLite2-City.mmdb"
	defaultGeoLiteDownload   = "https://github.com/P3TERX/GeoLite.mmdb/raw/download/GeoLite2-City.mmdb"
	defaultMissingAccountTTL = time.Hour
	defaultDownloadTimeout   = 60 * time.Second
	cacheVersion             = 2
)

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

type ResolverConfig struct {
	CachePath          string
	GeoLiteDBPath      string
	GeoLiteDownloadURL string
	AutoDownload       bool
	MissingAccountTTL  time.Duration
	DownloadTimeout    time.Duration
}

// Resolver enriches validators and transactions with geolocation using GeoLite.
type Resolver struct {
	logger              *logrus.Logger
	db                  *geoip2.Reader
	cachePath           string
	missingAccountTTL   time.Duration
	dnsLookup           func(string) ([]net.IP, error)
	lookupGeoByIP       func(string) (*models.GeoLocation, error)
	mu                  sync.RWMutex
	cache               map[string]*geoCacheEntry
	missingAccountUntil map[string]time.Time
}

// NewResolver creates a resolver backed by the GeoLite2 City database.
func NewResolver(logger *logrus.Logger, cfg ResolverConfig) (*Resolver, error) {
	if logger == nil {
		logger = logrus.New()
	}

	cfg = withDefaults(cfg)
	if err := ensureGeoLiteDatabase(cfg, logger); err != nil {
		return nil, err
	}

	db, err := geoip2.Open(cfg.GeoLiteDBPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open GeoLite DB at %s: %w", cfg.GeoLiteDBPath, err)
	}

	r := &Resolver{
		logger:              logger,
		db:                  db,
		cachePath:           cfg.CachePath,
		missingAccountTTL:   cfg.MissingAccountTTL,
		dnsLookup:           net.LookupIP,
		cache:               make(map[string]*geoCacheEntry),
		missingAccountUntil: make(map[string]time.Time),
	}
	r.lookupGeoByIP = r.lookupGeoLiteIP
	r.loadCache()
	return r, nil
}

func withDefaults(cfg ResolverConfig) ResolverConfig {
	if strings.TrimSpace(cfg.CachePath) == "" {
		cfg.CachePath = defaultCachePath
	}
	if strings.TrimSpace(cfg.GeoLiteDBPath) == "" {
		cfg.GeoLiteDBPath = defaultGeoLiteDBPath
	}
	if strings.TrimSpace(cfg.GeoLiteDownloadURL) == "" {
		cfg.GeoLiteDownloadURL = defaultGeoLiteDownload
	}
	if cfg.MissingAccountTTL <= 0 {
		cfg.MissingAccountTTL = defaultMissingAccountTTL
	}
	if cfg.DownloadTimeout <= 0 {
		cfg.DownloadTimeout = defaultDownloadTimeout
	}
	return cfg
}

func ensureGeoLiteDatabase(cfg ResolverConfig, logger *logrus.Logger) error {
	if _, err := os.Stat(cfg.GeoLiteDBPath); err == nil {
		return nil
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("failed to access GeoLite DB path %s: %w", cfg.GeoLiteDBPath, err)
	}

	if !cfg.AutoDownload {
		return fmt.Errorf("GeoLite DB not found at %s and auto-download is disabled", cfg.GeoLiteDBPath)
	}
	if strings.TrimSpace(cfg.GeoLiteDownloadURL) == "" {
		return fmt.Errorf("GeoLite DB not found at %s and no download URL configured", cfg.GeoLiteDBPath)
	}

	logger.WithFields(logrus.Fields{
		"path": cfg.GeoLiteDBPath,
		"url":  cfg.GeoLiteDownloadURL,
	}).Info("GeoLite DB missing; downloading")

	if err := downloadFile(cfg.GeoLiteDownloadURL, cfg.GeoLiteDBPath, cfg.DownloadTimeout); err != nil {
		return fmt.Errorf("failed to download GeoLite DB: %w", err)
	}

	logger.WithField("path", cfg.GeoLiteDBPath).Info("GeoLite DB downloaded")
	return nil
}

func downloadFile(url, destination string, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download returned status %d", resp.StatusCode)
	}

	if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
		return err
	}

	tmpPath := destination + ".tmp"
	file, err := os.Create(tmpPath)
	if err != nil {
		return err
	}
	defer os.Remove(tmpPath)

	if _, err := io.Copy(file, resp.Body); err != nil {
		file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}

	return os.Rename(tmpPath, destination)
}

// Close releases the underlying GeoLite reader.
func (r *Resolver) Close() error {
	if r == nil || r.db == nil {
		return nil
	}
	return r.db.Close()
}

// EnrichValidator resolves the validator domain against GeoLite data.
func (r *Resolver) EnrichValidator(validator *models.Validator) error {
	if validator == nil {
		return fmt.Errorf("validator is nil")
	}
	if strings.TrimSpace(validator.Domain) == "" {
		return fmt.Errorf("no domain available for geolocation")
	}

	geo, err := r.ResolveDomainGeo(validator.Domain)
	if err != nil {
		return err
	}
	if geo == nil {
		return fmt.Errorf("no geolocation found for validator domain")
	}

	validator.Latitude = geo.Latitude
	validator.Longitude = geo.Longitude
	validator.CountryCode = geo.CountryCode
	validator.City = geo.City
	return nil
}

// ResolveAccountGeo resolves a transaction account to geolocation by reading the
// account domain from XRPL and then resolving that domain through GeoLite.
func (r *Resolver) ResolveAccountGeo(ctx context.Context, client rippled.RippledClient, account string) (*models.GeoLocation, error) {
	account = strings.TrimSpace(account)
	if account == "" {
		return nil, nil
	}

	if geo, ok := r.getCachedGeo("account:" + account); ok {
		geo.ValidatorAddress = account
		return geo, nil
	}
	if r.isAccountMissing(account) {
		return nil, nil
	}
	if client == nil {
		return nil, fmt.Errorf("rippled client is nil")
	}

	domain, err := fetchAccountDomain(ctx, client, account)
	if err != nil {
		if isMissingAccountError(err) {
			r.markAccountMissing(account)
		}
		return nil, err
	}
	if strings.TrimSpace(domain) == "" {
		r.markAccountMissing(account)
		return nil, nil
	}

	geo, err := r.ResolveDomainGeo(domain)
	if err != nil {
		return nil, err
	}
	if geo == nil {
		return nil, nil
	}

	geo.ValidatorAddress = account
	r.setCachedGeo("account:"+account, geo)
	if err := r.persistCache(); err != nil {
		r.logger.WithError(err).Warn("Failed to persist geolocation cache")
	}
	r.clearMissingAccount(account)
	return geo, nil
}

// ResolveDomainGeo resolves a domain via DNS and then GeoLite.
func (r *Resolver) ResolveDomainGeo(rawDomain string) (*models.GeoLocation, error) {
	domain := normalizeDomain(rawDomain)
	if domain == "" {
		return nil, fmt.Errorf("invalid domain")
	}

	if geo, ok := r.getCachedGeo("domain:" + domain); ok {
		return geo, nil
	}

	ip, err := r.resolveDomainIP(domain)
	if err != nil {
		return nil, err
	}

	if geo, ok := r.getCachedGeo("ip:" + ip); ok {
		r.setCachedGeo("domain:"+domain, geo)
		if err := r.persistCache(); err != nil {
			r.logger.WithError(err).Warn("Failed to persist geolocation cache")
		}
		return geo, nil
	}

	geo, err := r.lookupGeoByIP(ip)
	if err != nil {
		return nil, err
	}
	if geo == nil {
		return nil, fmt.Errorf("no geolocation found for ip %s", ip)
	}

	r.setCachedGeo("ip:"+ip, geo)
	r.setCachedGeo("domain:"+domain, geo)
	if err := r.persistCache(); err != nil {
		r.logger.WithError(err).Warn("Failed to persist geolocation cache")
	}
	return geo, nil
}

func (r *Resolver) lookupGeoLiteIP(ip string) (*models.GeoLocation, error) {
	parsed := net.ParseIP(ip)
	if parsed == nil {
		return nil, fmt.Errorf("invalid IP: %s", ip)
	}
	record, err := r.db.City(parsed)
	if err != nil {
		return nil, fmt.Errorf("GeoLite lookup failed for %s: %w", ip, err)
	}

	lat := record.Location.Latitude
	lng := record.Location.Longitude
	if lat == 0 && lng == 0 {
		return nil, fmt.Errorf("GeoLite record has no coordinates for %s", ip)
	}

	countryCode := strings.ToUpper(strings.TrimSpace(record.Country.IsoCode))
	if countryCode == "" {
		countryCode = "XX"
	}
	city := strings.TrimSpace(record.City.Names["en"])
	if city == "" {
		city = "Unknown"
	}

	return &models.GeoLocation{
		Latitude:    lat,
		Longitude:   lng,
		CountryCode: countryCode,
		City:        city,
	}, nil
}

func (r *Resolver) resolveDomainIP(domain string) (string, error) {
	ips, err := r.dnsLookup(domain)
	if err != nil {
		return "", fmt.Errorf("failed to resolve domain %s: %w", domain, err)
	}
	if len(ips) == 0 {
		return "", fmt.Errorf("domain %s resolved with no IPs", domain)
	}
	return pickIP(ips), nil
}

func pickIP(ips []net.IP) string {
	for _, candidate := range ips {
		if candidate.To4() != nil {
			return candidate.String()
		}
	}
	return ips[0].String()
}

func fetchAccountDomain(ctx context.Context, client rippled.RippledClient, account string) (string, error) {
	resp, err := client.Command(ctx, "account_info", map[string]interface{}{
		"account":      account,
		"ledger_index": "validated",
		"strict":       true,
	})
	if err != nil {
		return "", err
	}

	respMap, ok := resp.(map[string]interface{})
	if !ok {
		return "", fmt.Errorf("unexpected account_info response")
	}

	result, ok := respMap["result"].(map[string]interface{})
	if !ok {
		return "", fmt.Errorf("account_info missing result")
	}

	accountData, ok := result["account_data"].(map[string]interface{})
	if !ok {
		return "", nil
	}

	domainHex, _ := accountData["Domain"].(string)
	if domainHex == "" {
		return "", nil
	}

	domainRaw, err := hex.DecodeString(domainHex)
	if err != nil {
		return "", fmt.Errorf("failed to decode account domain: %w", err)
	}

	domain := strings.TrimSpace(strings.Trim(string(domainRaw), "\x00"))
	return normalizeDomain(domain), nil
}

func normalizeDomain(raw string) string {
	domain := strings.TrimSpace(raw)
	domain = strings.TrimPrefix(domain, "http://")
	domain = strings.TrimPrefix(domain, "https://")
	domain = strings.TrimSuffix(domain, "/")
	domain = strings.TrimSuffix(domain, ".")
	if host, _, err := net.SplitHostPort(domain); err == nil {
		domain = host
	}
	return strings.ToLower(strings.TrimSpace(domain))
}

func isMissingAccountError(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "actnotfound") ||
		strings.Contains(message, "account not found") ||
		strings.Contains(message, "no account") ||
		strings.Contains(message, "malformedaddress")
}

func (r *Resolver) isAccountMissing(account string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	until, ok := r.missingAccountUntil[account]
	if !ok {
		return false
	}
	if time.Now().After(until) {
		delete(r.missingAccountUntil, account)
		return false
	}
	return true
}

func (r *Resolver) markAccountMissing(account string) {
	r.mu.Lock()
	r.missingAccountUntil[account] = time.Now().Add(r.missingAccountTTL)
	r.mu.Unlock()
}

func (r *Resolver) clearMissingAccount(account string) {
	r.mu.Lock()
	delete(r.missingAccountUntil, account)
	r.mu.Unlock()
}

func (r *Resolver) getCachedGeo(key string) (*models.GeoLocation, bool) {
	r.mu.RLock()
	entry, ok := r.cache[key]
	r.mu.RUnlock()
	if !ok || entry == nil {
		return nil, false
	}

	return &models.GeoLocation{
		Latitude:    entry.Latitude,
		Longitude:   entry.Longitude,
		CountryCode: entry.CountryCode,
		City:        entry.City,
	}, true
}

func (r *Resolver) setCachedGeo(key string, geo *models.GeoLocation) {
	if geo == nil {
		return
	}

	r.mu.Lock()
	r.cache[key] = &geoCacheEntry{
		CountryCode: geo.CountryCode,
		City:        geo.City,
		Latitude:    geo.Latitude,
		Longitude:   geo.Longitude,
		UpdatedAt:   time.Now().Unix(),
	}
	r.mu.Unlock()
}

func (r *Resolver) loadCache() {
	data, err := os.ReadFile(r.cachePath)
	if err != nil {
		if !os.IsNotExist(err) {
			r.logger.WithError(err).WithField("path", r.cachePath).Warn("Failed to read geolocation cache")
		}
		return
	}

	var payload geoCacheFile
	if err := json.Unmarshal(data, &payload); err != nil {
		r.logger.WithError(err).WithField("path", r.cachePath).Warn("Failed to parse geolocation cache")
		return
	}
	if payload.Version != cacheVersion || payload.Entries == nil {
		return
	}

	r.mu.Lock()
	r.cache = payload.Entries
	r.mu.Unlock()

	r.logger.WithFields(logrus.Fields{
		"path":    r.cachePath,
		"entries": len(payload.Entries),
	}).Info("Loaded geolocation cache")
}

func (r *Resolver) persistCache() error {
	r.mu.RLock()
	payload := geoCacheFile{
		Version: cacheVersion,
		Entries: make(map[string]*geoCacheEntry, len(r.cache)),
	}
	for key, entry := range r.cache {
		if entry == nil {
			continue
		}
		copy := *entry
		payload.Entries[key] = &copy
	}
	r.mu.RUnlock()

	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(r.cachePath), 0o755); err != nil {
		return err
	}
	tmpPath := r.cachePath + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmpPath, r.cachePath)
}

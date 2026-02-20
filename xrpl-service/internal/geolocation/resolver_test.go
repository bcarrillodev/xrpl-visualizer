package geolocation

import (
	"context"
	"encoding/hex"
	"errors"
	"net"
	"path/filepath"
	"testing"
	"time"

	"github.com/brandon/xrpl-validator-service/internal/models"
	"github.com/sirupsen/logrus"
)

type stubXRPLClient struct {
	commandCalls int
	commandFunc  func(method string, params interface{}) (interface{}, error)
}

func (s *stubXRPLClient) Connect(ctx context.Context) error { return nil }
func (s *stubXRPLClient) Close() error                      { return nil }
func (s *stubXRPLClient) IsConnected() bool                 { return true }
func (s *stubXRPLClient) Subscribe(ctx context.Context, streams []string, callback func(interface{})) error {
	return nil
}
func (s *stubXRPLClient) Unsubscribe(ctx context.Context, streams []string) error { return nil }
func (s *stubXRPLClient) GetValidators(ctx context.Context) (interface{}, error)  { return nil, nil }
func (s *stubXRPLClient) GetServerInfo(ctx context.Context) (interface{}, error)  { return nil, nil }
func (s *stubXRPLClient) Command(ctx context.Context, method string, params interface{}) (interface{}, error) {
	s.commandCalls++
	if s.commandFunc == nil {
		return nil, errors.New("stub command func not set")
	}
	return s.commandFunc(method, params)
}

func newTestResolver(t *testing.T, cachePath string) *Resolver {
	t.Helper()
	return &Resolver{
		logger:              logrus.New(),
		cachePath:           cachePath,
		missingAccountTTL:   time.Hour,
		cache:               make(map[string]*geoCacheEntry),
		missingAccountUntil: make(map[string]time.Time),
	}
}

func TestResolveDomainGeoCachesByDomainAndIP(t *testing.T) {
	resolver := newTestResolver(t, filepath.Join(t.TempDir(), "geo-cache.json"))

	dnsCalls := 0
	lookupCalls := 0
	resolver.dnsLookup = func(host string) ([]net.IP, error) {
		dnsCalls++
		if host != "example.com" {
			t.Fatalf("unexpected host lookup: %s", host)
		}
		return []net.IP{net.ParseIP("1.2.3.4")}, nil
	}
	resolver.lookupGeoByIP = func(ip string) (*models.GeoLocation, error) {
		lookupCalls++
		if ip != "1.2.3.4" {
			t.Fatalf("unexpected ip lookup: %s", ip)
		}
		return &models.GeoLocation{
			Latitude:    40.7128,
			Longitude:   -74.0060,
			CountryCode: "US",
			City:        "New York",
		}, nil
	}

	first, err := resolver.ResolveDomainGeo("https://Example.com/")
	if err != nil {
		t.Fatalf("ResolveDomainGeo first call failed: %v", err)
	}
	second, err := resolver.ResolveDomainGeo("example.com")
	if err != nil {
		t.Fatalf("ResolveDomainGeo second call failed: %v", err)
	}

	if first == nil || second == nil {
		t.Fatalf("expected geolocation result for both calls")
	}
	if first.City != "New York" || second.City != "New York" {
		t.Fatalf("expected city New York, got %q and %q", first.City, second.City)
	}
	if dnsCalls != 1 {
		t.Fatalf("expected 1 DNS lookup, got %d", dnsCalls)
	}
	if lookupCalls != 1 {
		t.Fatalf("expected 1 IP geolocation lookup, got %d", lookupCalls)
	}
}

func TestResolveAccountGeoCachesAccountLookup(t *testing.T) {
	resolver := newTestResolver(t, filepath.Join(t.TempDir(), "geo-cache.json"))

	dnsCalls := 0
	resolver.dnsLookup = func(host string) ([]net.IP, error) {
		dnsCalls++
		if host != "example.com" {
			t.Fatalf("expected normalized domain example.com, got %s", host)
		}
		return []net.IP{net.ParseIP("8.8.8.8")}, nil
	}
	resolver.lookupGeoByIP = func(ip string) (*models.GeoLocation, error) {
		return &models.GeoLocation{
			Latitude:    37.3860,
			Longitude:   -122.0840,
			CountryCode: "US",
			City:        "Mountain View",
		}, nil
	}

	client := &stubXRPLClient{
		commandFunc: func(method string, params interface{}) (interface{}, error) {
			if method != "account_info" {
				t.Fatalf("unexpected method: %s", method)
			}
			return map[string]interface{}{
				"result": map[string]interface{}{
					"account_data": map[string]interface{}{
						"Domain": hex.EncodeToString([]byte("https://Example.com/")),
					},
				},
			}, nil
		},
	}

	account := "rSourceAccount"
	first, err := resolver.ResolveAccountGeo(context.Background(), client, account)
	if err != nil {
		t.Fatalf("ResolveAccountGeo first call failed: %v", err)
	}
	second, err := resolver.ResolveAccountGeo(context.Background(), client, account)
	if err != nil {
		t.Fatalf("ResolveAccountGeo second call failed: %v", err)
	}

	if first == nil || second == nil {
		t.Fatalf("expected geolocation result for both calls")
	}
	if first.ValidatorAddress != account || second.ValidatorAddress != account {
		t.Fatalf("expected validator address %s on both calls", account)
	}
	if client.commandCalls != 1 {
		t.Fatalf("expected 1 account_info call, got %d", client.commandCalls)
	}
	if dnsCalls != 1 {
		t.Fatalf("expected 1 DNS call, got %d", dnsCalls)
	}
}

func TestResolveAccountGeoCachesMissingDomain(t *testing.T) {
	resolver := newTestResolver(t, filepath.Join(t.TempDir(), "geo-cache.json"))
	resolver.dnsLookup = func(host string) ([]net.IP, error) {
		t.Fatalf("dns lookup should not be called for missing domain")
		return nil, nil
	}
	resolver.lookupGeoByIP = func(ip string) (*models.GeoLocation, error) {
		t.Fatalf("geo lookup should not be called for missing domain")
		return nil, nil
	}

	client := &stubXRPLClient{
		commandFunc: func(method string, params interface{}) (interface{}, error) {
			return map[string]interface{}{
				"result": map[string]interface{}{
					"account_data": map[string]interface{}{},
				},
			}, nil
		},
	}

	account := "rNoDomain"
	for i := 0; i < 2; i++ {
		geo, err := resolver.ResolveAccountGeo(context.Background(), client, account)
		if err != nil {
			t.Fatalf("ResolveAccountGeo call %d failed: %v", i+1, err)
		}
		if geo != nil {
			t.Fatalf("expected nil geolocation for account without domain")
		}
	}

	if client.commandCalls != 1 {
		t.Fatalf("expected missing account negative-cache to avoid repeat calls, got %d", client.commandCalls)
	}
}

func TestResolveDomainGeoLoadsFromPersistedCache(t *testing.T) {
	cachePath := filepath.Join(t.TempDir(), "geo-cache.json")

	writer := newTestResolver(t, cachePath)
	writer.dnsLookup = func(host string) ([]net.IP, error) {
		return []net.IP{net.ParseIP("9.9.9.9")}, nil
	}
	writer.lookupGeoByIP = func(ip string) (*models.GeoLocation, error) {
		return &models.GeoLocation{
			Latitude:    48.8566,
			Longitude:   2.3522,
			CountryCode: "FR",
			City:        "Paris",
		}, nil
	}
	if _, err := writer.ResolveDomainGeo("example.org"); err != nil {
		t.Fatalf("failed to prime cache: %v", err)
	}

	reader := newTestResolver(t, cachePath)
	reader.dnsLookup = func(host string) ([]net.IP, error) {
		t.Fatalf("dns lookup should not run when domain cache is loaded")
		return nil, nil
	}
	reader.lookupGeoByIP = func(ip string) (*models.GeoLocation, error) {
		t.Fatalf("geo lookup should not run when domain cache is loaded")
		return nil, nil
	}
	reader.loadCache()

	geo, err := reader.ResolveDomainGeo("example.org")
	if err != nil {
		t.Fatalf("ResolveDomainGeo from persisted cache failed: %v", err)
	}
	if geo == nil || geo.City != "Paris" {
		t.Fatalf("expected persisted Paris geolocation, got %+v", geo)
	}
}

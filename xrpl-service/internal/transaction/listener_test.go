package transaction

import (
	"context"
	"testing"

	"github.com/brandon/xrpl-validator-service/internal/models"
	"github.com/brandon/xrpl-validator-service/internal/xrpl"
)

type mockGeoResolver struct {
	locations map[string]*models.GeoLocation
}

func (m *mockGeoResolver) ResolveAccountGeo(ctx context.Context, client xrpl.NodeClient, account string) (*models.GeoLocation, error) {
	if geo, ok := m.locations[account]; ok {
		return geo, nil
	}
	return nil, nil
}

func containsAccount(accounts []string, expected string) bool {
	for _, account := range accounts {
		if account == expected {
			return true
		}
	}
	return false
}

func TestParseTransaction_UsesDeliveredAmountForPartialPayment(t *testing.T) {
	listener := NewListener(nil, 10_000_000_000, nil, nil) // 10,000 XRP

	msg := map[string]interface{}{
		"type":      "transaction",
		"validated": true,
		"date":      float64(760000000),
		"transaction": map[string]interface{}{
			"TransactionType": "Payment",
			"hash":            "ABC123",
			"Account":         "rSource",
			"Destination":     "rDest",
			"Amount":          "10000000000000000", // 10,000,000,000 XRP requested
			"Fee":             "12",
			"Flags":           float64(tfPartialPayment),
		},
		"meta": map[string]interface{}{
			"TransactionResult": "tesSUCCESS",
			"delivered_amount":  "50000000000", // 50,000 XRP delivered
		},
	}

	tx, err := listener.parseTransaction(msg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tx == nil {
		t.Fatal("expected transaction, got nil")
	}
	if tx.Amount != "50000000000" {
		t.Fatalf("expected delivered drops amount, got %s", tx.Amount)
	}
}

func TestParseTransaction_SkipsPartialPaymentWhenDeliveredAmountUnavailable(t *testing.T) {
	listener := NewListener(nil, 10_000_000_000, nil, nil) // 10,000 XRP

	msg := map[string]interface{}{
		"type":      "transaction",
		"validated": true,
		"date":      float64(760000000),
		"transaction": map[string]interface{}{
			"TransactionType": "Payment",
			"hash":            "DEF456",
			"Account":         "rSource",
			"Destination":     "rDest",
			"Amount":          "10000000000000000", // would be misleading if used
			"Fee":             "12",
			"Flags":           float64(tfPartialPayment),
		},
		"meta": map[string]interface{}{
			"TransactionResult": "tesSUCCESS",
			"delivered_amount":  "unavailable",
		},
	}

	tx, err := listener.parseTransaction(msg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tx != nil {
		t.Fatalf("expected nil transaction, got %+v", tx)
	}
}

func TestParseTransaction_FallsBackToAmountForNonPartialPayment(t *testing.T) {
	listener := NewListener(nil, 10_000_000_000, nil, nil) // 10,000 XRP

	msg := map[string]interface{}{
		"type":      "transaction",
		"validated": true,
		"date":      float64(760000000),
		"transaction": map[string]interface{}{
			"TransactionType": "Payment",
			"hash":            "GHI789",
			"Account":         "rSource",
			"Destination":     "rDest",
			"Amount":          "15000000000", // 15,000 XRP
			"Fee":             "12",
			"Flags":           float64(0),
		},
		"meta": map[string]interface{}{
			"TransactionResult": "tesSUCCESS",
		},
	}

	tx, err := listener.parseTransaction(msg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tx == nil {
		t.Fatal("expected transaction, got nil")
	}
	if tx.Amount != "15000000000" {
		t.Fatalf("expected fallback amount in drops, got %s", tx.Amount)
	}
}

func TestParseTransaction_CollectsGeoCandidatesFromIssuerAndMetadata(t *testing.T) {
	listener := NewListener(nil, 1, nil, nil)
	source := "rHb9CJAWyB4rj91VRWn96DkukG4bwdtyTh"
	destination := "rLHzPsX6oXkzU9cRHEwKmMSWJfpJ9nE4VY"
	issuer := "rPT1Sjq2YGrBMTttX4GZHjKu9dyfzbpAYe"
	owner := "rDsbeomae4FXwgQTJp9Rs64Qg9vDiTCdBv"

	msg := map[string]interface{}{
		"type":      "transaction",
		"validated": true,
		"date":      float64(760000001),
		"transaction": map[string]interface{}{
			"TransactionType": "Payment",
			"hash":            "JKL123",
			"Account":         source,
			"Destination":     destination,
			"Amount":          "15000000",
			"Fee":             "12",
			"SendMax": map[string]interface{}{
				"currency": "USD",
				"issuer":   issuer,
				"value":    "100",
			},
		},
		"meta": map[string]interface{}{
			"TransactionResult": "tesSUCCESS",
			"AffectedNodes": []interface{}{
				map[string]interface{}{
					"ModifiedNode": map[string]interface{}{
						"FinalFields": map[string]interface{}{
							"Owner": owner,
						},
					},
				},
			},
		},
	}

	tx, err := listener.parseTransaction(msg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tx == nil {
		t.Fatal("expected transaction, got nil")
	}
	if !containsAccount(tx.GeoCandidates, source) {
		t.Fatalf("expected source account in candidates: %+v", tx.GeoCandidates)
	}
	if !containsAccount(tx.GeoCandidates, destination) {
		t.Fatalf("expected destination account in candidates: %+v", tx.GeoCandidates)
	}
	if !containsAccount(tx.GeoCandidates, issuer) {
		t.Fatalf("expected issuer account in candidates: %+v", tx.GeoCandidates)
	}
	if !containsAccount(tx.GeoCandidates, owner) {
		t.Fatalf("expected owner account in candidates: %+v", tx.GeoCandidates)
	}
}

func TestGatherGeoCandidates_LimitPreservesSourceAndDestination(t *testing.T) {
	source := "rHb9CJAWyB4rj91VRWn96DkukG4bwdtyTh"
	destination := "rLHzPsX6oXkzU9cRHEwKmMSWJfpJ9nE4VY"
	issuerA := "rPT1Sjq2YGrBMTttX4GZHjKu9dyfzbpAYe"
	issuerB := "rDsbeomae4FXwgQTJp9Rs64Qg9vDiTCdBv"

	txnRaw := map[string]interface{}{
		"Account":     source,
		"Destination": destination,
		"SendMax": map[string]interface{}{
			"issuer": issuerA,
		},
	}
	meta := map[string]interface{}{
		"AffectedNodes": []interface{}{
			map[string]interface{}{
				"ModifiedNode": map[string]interface{}{
					"FinalFields": map[string]interface{}{
						"Issuer": issuerB,
					},
				},
			},
		},
	}

	candidates := gatherGeoCandidates(txnRaw, meta, source, destination, 3)
	if len(candidates) != 3 {
		t.Fatalf("expected 3 candidates, got %d (%+v)", len(candidates), candidates)
	}
	if candidates[0] != source {
		t.Fatalf("expected source first, got %+v", candidates)
	}
	if candidates[1] != destination {
		t.Fatalf("expected destination second, got %+v", candidates)
	}
}

func TestEnrichTransaction_PopulatesLocations(t *testing.T) {
	source := "rHb9CJAWyB4rj91VRWn96DkukG4bwdtyTh"
	destination := "rLHzPsX6oXkzU9cRHEwKmMSWJfpJ9nE4VY"
	issuer := "rPT1Sjq2YGrBMTttX4GZHjKu9dyfzbpAYe"

	resolver := &mockGeoResolver{
		locations: map[string]*models.GeoLocation{
			source: {
				Latitude:  37.7749,
				Longitude: -122.4194,
				City:      "San Francisco",
			},
			destination: {
				Latitude:  51.5074,
				Longitude: -0.1278,
				City:      "London",
			},
			issuer: {
				Latitude:  35.6762,
				Longitude: 139.6503,
				City:      "Tokyo",
			},
		},
	}
	listener := NewListener(nil, 1, resolver, nil)

	tx := &models.Transaction{
		Account:       source,
		Destination:   destination,
		GeoCandidates: []string{issuer},
	}

	listener.enrichTransaction(context.Background(), tx)

	if len(tx.Locations) != 3 {
		t.Fatalf("expected 3 mapped locations, got %d", len(tx.Locations))
	}
	if tx.Locations[0].City != "San Francisco" {
		t.Fatalf("expected source location first, got %+v", tx.Locations[0])
	}
	if tx.Locations[1].City != "London" {
		t.Fatalf("expected destination location second, got %+v", tx.Locations[1])
	}
	if tx.Locations[2].City != "Tokyo" {
		t.Fatalf("expected extra candidate location third, got %+v", tx.Locations[2])
	}
}

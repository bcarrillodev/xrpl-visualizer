package models

import (
	"encoding/json"
	"testing"
	"time"
)

func TestValidatorJSON(t *testing.T) {
	v := &Validator{
		Address:     "r123456789",
		PublicKey:   "ED123456789",
		Domain:      "example.com",
		Name:        "Example Validator",
		Network:     "altnet",
		Latitude:    40.7128,
		Longitude:   -74.0060,
		CountryCode: "US",
		City:        "New York",
		LastUpdated: 1640995200,
		IsActive:    true,
	}

	data, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("Failed to marshal Validator: %v", err)
	}

	var v2 Validator
	if err := json.Unmarshal(data, &v2); err != nil {
		t.Fatalf("Failed to unmarshal Validator: %v", err)
	}

	if v2.Address != v.Address {
		t.Errorf("Expected Address %s, got %s", v.Address, v2.Address)
	}
	if v2.PublicKey != v.PublicKey {
		t.Errorf("Expected PublicKey %s, got %s", v.PublicKey, v2.PublicKey)
	}
	if v2.Latitude != v.Latitude {
		t.Errorf("Expected Latitude %f, got %f", v.Latitude, v2.Latitude)
	}
	if v2.IsActive != v.IsActive {
		t.Errorf("Expected IsActive %t, got %t", v.IsActive, v2.IsActive)
	}
}

func TestTransactionJSON(t *testing.T) {
	now := time.Now().Unix()
	tx := &Transaction{
		Hash:              "ABC123",
		LedgerIndex:       12345,
		Account:           "rSource",
		Destination:       "rDest",
		TransactionType:   "Payment",
		Amount:            "1000000",
		Fee:               "10",
		TransactionResult: "tesSUCCESS",
		Timestamp:         now,
		CloseTime:         1640995200,
		Validated:         true,
		Locations: []*GeoLocation{
			{
				Latitude:         40.7128,
				Longitude:        -74.0060,
				CountryCode:      "US",
				City:             "New York",
				ValidatorAddress: "rSource",
			},
			{
				Latitude:         51.5074,
				Longitude:        -0.1278,
				CountryCode:      "GB",
				City:             "London",
				ValidatorAddress: "rDest",
			},
		},
	}

	data, err := json.Marshal(tx)
	if err != nil {
		t.Fatalf("Failed to marshal Transaction: %v", err)
	}

	var tx2 Transaction
	if err := json.Unmarshal(data, &tx2); err != nil {
		t.Fatalf("Failed to unmarshal Transaction: %v", err)
	}

	if tx2.Hash != tx.Hash {
		t.Errorf("Expected Hash %s, got %s", tx.Hash, tx2.Hash)
	}
	if tx2.TransactionType != tx.TransactionType {
		t.Errorf("Expected TransactionType %s, got %s", tx.TransactionType, tx2.TransactionType)
	}
	if len(tx2.Locations) != len(tx.Locations) {
		t.Fatalf("Expected %d locations, got %d", len(tx.Locations), len(tx2.Locations))
	}
	if tx2.Locations[0].City != tx.Locations[0].City {
		t.Errorf("Expected first location city %s, got %s", tx.Locations[0].City, tx2.Locations[0].City)
	}
	if tx2.Locations[1].City != tx.Locations[1].City {
		t.Errorf("Expected second location city %s, got %s", tx.Locations[1].City, tx2.Locations[1].City)
	}
}

func TestGeoLocationJSON(t *testing.T) {
	gl := &GeoLocation{
		Latitude:         40.7128,
		Longitude:        -74.0060,
		CountryCode:      "US",
		City:             "New York",
		ValidatorAddress: "r123",
	}

	data, err := json.Marshal(gl)
	if err != nil {
		t.Fatalf("Failed to marshal GeoLocation: %v", err)
	}

	var gl2 GeoLocation
	if err := json.Unmarshal(data, &gl2); err != nil {
		t.Fatalf("Failed to unmarshal GeoLocation: %v", err)
	}

	if gl2.Latitude != gl.Latitude {
		t.Errorf("Expected Latitude %f, got %f", gl.Latitude, gl2.Latitude)
	}
	if gl2.City != gl.City {
		t.Errorf("Expected City %s, got %s", gl.City, gl2.City)
	}
	if gl2.ValidatorAddress != gl.ValidatorAddress {
		t.Errorf("Expected ValidatorAddress %s, got %s", gl.ValidatorAddress, gl2.ValidatorAddress)
	}
}

func TestServerStatusJSON(t *testing.T) {
	ss := &ServerStatus{
		Connected:   true,
		ServerState: "full",
		LedgerIndex: 12345,
		NetworkID:   1,
		Uptime:      3600,
		LastSync:    1640995200,
	}

	data, err := json.Marshal(ss)
	if err != nil {
		t.Fatalf("Failed to marshal ServerStatus: %v", err)
	}

	var ss2 ServerStatus
	if err := json.Unmarshal(data, &ss2); err != nil {
		t.Fatalf("Failed to unmarshal ServerStatus: %v", err)
	}

	if ss2.Connected != ss.Connected {
		t.Errorf("Expected Connected %t, got %t", ss.Connected, ss2.Connected)
	}
	if ss2.ServerState != ss.ServerState {
		t.Errorf("Expected ServerState %s, got %s", ss.ServerState, ss2.ServerState)
	}
	if ss2.LedgerIndex != ss.LedgerIndex {
		t.Errorf("Expected LedgerIndex %d, got %d", ss.LedgerIndex, ss2.LedgerIndex)
	}
}

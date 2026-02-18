package models

// Validator represents an XRPL validator with geolocation data
type Validator struct {
	// Validator Identifier
	Address   string `json:"address"`    // Base58 validator address
	PublicKey string `json:"public_key"` // Hex-encoded public key
	Domain    string `json:"domain"`     // Domain name if available
	Name      string `json:"name"`       // Human-readable name

	// Network Info
	Network string `json:"network"` // "altnet", "mainnet", etc.

	// Geolocation Data
	Latitude    float64 `json:"latitude"`
	Longitude   float64 `json:"longitude"`
	CountryCode string  `json:"country_code"`
	City        string  `json:"city"`

	// Metadata
	LastUpdated int64 `json:"last_updated"` // Unix timestamp
	IsActive    bool  `json:"is_active"`
}

// Transaction represents an XRP Ledger transaction
type Transaction struct {
	// Transaction Identifier
	Hash        string `json:"hash"` // Transaction hash
	LedgerIndex uint32 `json:"ledger_index"`

	// Parties Involved
	Account     string `json:"account"`     // Source account
	Destination string `json:"destination"` // Destination account

	// Transaction Details
	TransactionType string `json:"transaction_type"` // "Payment", "TrustSet", etc.
	Amount          string `json:"amount"`           // Amount in drops or JSON object
	Fee             string `json:"fee"`              // Fee in drops

	// Status
	TransactionResult string `json:"transaction_result"` // "tesSUCCESS", etc.

	// Timestamp
	Timestamp int64  `json:"timestamp"`  // Unix timestamp (if available)
	CloseTime uint32 `json:"close_time"` // Ledger close time

	// Metadata
	Validated     bool           `json:"validated"`
	Locations     []*GeoLocation `json:"locations,omitempty"` // Mapped account endpoints for hotspot/activity layers
	GeoCandidates []string       `json:"-"`                   // Internal candidate accounts for enrichment
}

// GeoLocation represents geographic location data
type GeoLocation struct {
	Latitude         float64 `json:"latitude"`
	Longitude        float64 `json:"longitude"`
	CountryCode      string  `json:"country_code"`
	City             string  `json:"city"`
	ValidatorAddress string  `json:"validator_address,omitempty"`
}

// ServerStatus represents rippled server health status
type ServerStatus struct {
	Connected       bool   `json:"connected"`
	ServerState     string `json:"server_state"`
	LedgerIndex     uint32 `json:"ledger_index"`
	NetworkID       uint16 `json:"network_id"`
	PeerCount       int    `json:"peer_count"`
	CompleteLedgers string `json:"complete_ledgers"`
	Uptime          int64  `json:"uptime"`
	LastSync        int64  `json:"last_sync"`
}

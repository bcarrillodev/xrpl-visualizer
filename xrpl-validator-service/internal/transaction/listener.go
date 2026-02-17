package transaction

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/brandon/xrpl-validator-service/internal/models"
	"github.com/brandon/xrpl-validator-service/internal/rippled"
	"github.com/sirupsen/logrus"
)

const rippleEpochOffset = 946684800

// Listener handles transaction stream subscriptions and callbacks
type Listener struct {
	client            rippled.RippledClient
	logger            *logrus.Logger
	mu                sync.RWMutex
	callbacks         []TransactionCallback
	isSubscribed      bool
	stopChan          chan struct{}
	transactionBuffer chan *models.Transaction
	minPaymentDrops   int64

	geoClient     *http.Client
	geoMu         sync.RWMutex
	accountGeo    map[string]*models.GeoLocation
	noGeoAccounts map[string]struct{}
}

// TransactionCallback is a function that processes transactions
type TransactionCallback func(*models.Transaction)

// NewListener creates a new transaction listener
func NewListener(client rippled.RippledClient, minPaymentDrops int64, logger *logrus.Logger) *Listener {
	if logger == nil {
		logger = logrus.New()
	}
	if minPaymentDrops <= 0 {
		minPaymentDrops = 10000000000
	}
	return &Listener{
		client:            client,
		logger:            logger,
		callbacks:         make([]TransactionCallback, 0),
		stopChan:          make(chan struct{}),
		transactionBuffer: make(chan *models.Transaction, 100),
		minPaymentDrops:   minPaymentDrops,
		geoClient:         &http.Client{Timeout: 8 * time.Second},
		accountGeo:        make(map[string]*models.GeoLocation),
		noGeoAccounts:     make(map[string]struct{}),
	}
}

// AddCallback registers a callback function for transaction processing
func (l *Listener) AddCallback(callback TransactionCallback) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.callbacks = append(l.callbacks, callback)
}

// Start begins listening for transactions
func (l *Listener) Start(ctx context.Context) error {
	l.mu.Lock()
	if l.isSubscribed {
		l.mu.Unlock()
		return fmt.Errorf("already subscribed")
	}
	l.mu.Unlock()

	if l.client != nil && !l.client.IsConnected() {
		if err := l.client.Connect(ctx); err != nil {
			return fmt.Errorf("failed to connect to rippled websocket: %w", err)
		}
	}

	err := l.client.Subscribe(ctx, []string{"transactions"}, l.handleMessage)
	if err != nil {
		return fmt.Errorf("failed to subscribe to transactions: %w", err)
	}

	l.mu.Lock()
	l.isSubscribed = true
	l.mu.Unlock()

	l.logger.WithField("min_payment_drops", l.minPaymentDrops).Info("Transaction listener started")

	go l.processTransactions()

	return nil
}

// Stop stops the transaction listener
func (l *Listener) Stop(ctx context.Context) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	if !l.isSubscribed {
		return nil
	}

	close(l.stopChan)

	if l.client != nil && l.client.IsConnected() {
		err := l.client.Unsubscribe(ctx, []string{"transactions"})
		if err != nil {
			l.logger.WithError(err).Error("Failed to unsubscribe from transactions")
			return err
		}
	}

	l.isSubscribed = false
	l.logger.Info("Transaction listener stopped")
	return nil
}

// handleMessage processes incoming WebSocket messages from rippled
func (l *Listener) handleMessage(msg interface{}) {
	msgMap, ok := msg.(map[string]interface{})
	if !ok {
		return
	}

	tx, err := l.parseTransaction(msgMap)
	if err != nil {
		l.logger.WithError(err).Debug("Skipping transaction")
		return
	}
	if tx == nil {
		return
	}

	l.enrichTransaction(context.Background(), tx)

	select {
	case l.transactionBuffer <- tx:
	case <-l.stopChan:
		return
	default:
		l.logger.Warn("Transaction buffer full, dropping transaction")
	}
}

// processTransactions processes buffered transactions
func (l *Listener) processTransactions() {
	for {
		select {
		case tx := <-l.transactionBuffer:
			l.mu.RLock()
			callbacks := make([]TransactionCallback, len(l.callbacks))
			copy(callbacks, l.callbacks)
			l.mu.RUnlock()

			for _, callback := range callbacks {
				callback(tx)
			}

		case <-l.stopChan:
			return
		}
	}
}

// parseTransaction converts a raw stream message to a Transaction model.
func (l *Listener) parseTransaction(msg map[string]interface{}) (*models.Transaction, error) {
	msgType, _ := msg["type"].(string)
	if msgType != "transaction" {
		return nil, nil
	}

	validated, _ := msg["validated"].(bool)
	if !validated {
		return nil, nil
	}

	txnRaw, ok := msg["transaction"].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("missing transaction payload")
	}

	txType, _ := txnRaw["TransactionType"].(string)
	if txType != "Payment" {
		return nil, nil
	}

	amountDrops, ok := parseDrops(txnRaw["Amount"])
	if !ok || amountDrops < l.minPaymentDrops {
		return nil, nil
	}

	tx := &models.Transaction{
		Hash:            stringify(txnRaw["hash"]),
		Account:         stringify(txnRaw["Account"]),
		Destination:     stringify(txnRaw["Destination"]),
		TransactionType: txType,
		Amount:          strconv.FormatInt(amountDrops, 10),
		Fee:             stringify(txnRaw["Fee"]),
		Validated:       validated,
		Timestamp:       toUnixTimestamp(msg["date"]),
	}

	if tx.Hash == "" || tx.Account == "" || tx.Destination == "" {
		return nil, fmt.Errorf("missing required payment fields")
	}

	if li, ok := toUint32(msg["ledger_index"]); ok {
		tx.LedgerIndex = li
	}

	if result, ok := msg["engine_result"].(string); ok {
		tx.TransactionResult = result
	}
	if tx.TransactionResult == "" {
		if meta, ok := msg["meta"].(map[string]interface{}); ok {
			tx.TransactionResult = stringify(meta["TransactionResult"])
		}
	}
	if tx.TransactionResult != "tesSUCCESS" {
		return nil, nil
	}

	return tx, nil
}

func parseDrops(amount interface{}) (int64, bool) {
	asString, ok := amount.(string)
	if !ok {
		return 0, false
	}
	value, err := strconv.ParseInt(asString, 10, 64)
	if err != nil {
		return 0, false
	}
	return value, true
}

func stringify(v interface{}) string {
	switch t := v.(type) {
	case string:
		return t
	case float64:
		return strconv.FormatInt(int64(t), 10)
	default:
		return ""
	}
}

func toUint32(v interface{}) (uint32, bool) {
	n, ok := v.(float64)
	if !ok || n < 0 {
		return 0, false
	}
	return uint32(n), true
}

func toUnixTimestamp(v interface{}) int64 {
	rippleTime, ok := v.(float64)
	if !ok {
		return time.Now().Unix()
	}
	return int64(rippleTime) + rippleEpochOffset
}

// enrichTransaction adds geolocation data to transaction.
func (l *Listener) enrichTransaction(ctx context.Context, tx *models.Transaction) {
	sourceGeo, err := l.resolveAccountGeo(ctx, tx.Account)
	if err != nil {
		l.logger.WithError(err).WithField("account", tx.Account).Debug("Failed to resolve source geolocation")
	}
	if sourceGeo != nil {
		tx.SourceInfo = sourceGeo
	}

	destGeo, err := l.resolveAccountGeo(ctx, tx.Destination)
	if err != nil {
		l.logger.WithError(err).WithField("account", tx.Destination).Debug("Failed to resolve destination geolocation")
	}
	if destGeo != nil {
		tx.DestInfo = destGeo
	}
}

func (l *Listener) resolveAccountGeo(ctx context.Context, account string) (*models.GeoLocation, error) {
	if account == "" {
		return nil, nil
	}

	l.geoMu.RLock()
	if geo, ok := l.accountGeo[account]; ok {
		l.geoMu.RUnlock()
		return geo, nil
	}
	if _, ok := l.noGeoAccounts[account]; ok {
		l.geoMu.RUnlock()
		return nil, nil
	}
	l.geoMu.RUnlock()

	domain, err := l.fetchAccountDomain(ctx, account)
	if err != nil {
		l.markNoGeo(account)
		return nil, err
	}
	if domain == "" {
		l.markNoGeo(account)
		return nil, nil
	}

	ip, err := resolveDomainIP(domain)
	if err != nil {
		l.markNoGeo(account)
		return nil, err
	}

	geo, err := l.lookupIPGeo(ctx, ip)
	if err != nil {
		l.markNoGeo(account)
		return nil, err
	}

	geo.ValidatorAddress = account
	l.geoMu.Lock()
	l.accountGeo[account] = geo
	l.geoMu.Unlock()
	return geo, nil
}

func (l *Listener) markNoGeo(account string) {
	l.geoMu.Lock()
	l.noGeoAccounts[account] = struct{}{}
	l.geoMu.Unlock()
}

func (l *Listener) fetchAccountDomain(ctx context.Context, account string) (string, error) {
	resp, err := l.client.Command(ctx, "account_info", map[string]interface{}{
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
	domain = strings.TrimPrefix(domain, "http://")
	domain = strings.TrimPrefix(domain, "https://")
	domain = strings.TrimSuffix(domain, "/")
	if host, _, err := net.SplitHostPort(domain); err == nil {
		domain = host
	}

	return domain, nil
}

func resolveDomainIP(domain string) (string, error) {
	ips, err := net.LookupIP(domain)
	if err != nil {
		return "", fmt.Errorf("failed to resolve domain %s: %w", domain, err)
	}
	if len(ips) == 0 {
		return "", fmt.Errorf("domain %s resolved with no IPs", domain)
	}

	for _, ip := range ips {
		if ip.To4() != nil {
			return ip.String(), nil
		}
	}
	return ips[0].String(), nil
}

func (l *Listener) lookupIPGeo(ctx context.Context, ip string) (*models.GeoLocation, error) {
	url := fmt.Sprintf("https://ipwho.is/%s", ip)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := l.geoClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("geolocation API returned status %d", resp.StatusCode)
	}

	var result struct {
		Success     bool    `json:"success"`
		Message     string  `json:"message"`
		CountryCode string  `json:"country_code"`
		City        string  `json:"city"`
		Latitude    float64 `json:"latitude"`
		Longitude   float64 `json:"longitude"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	if !result.Success {
		return nil, fmt.Errorf("geolocation lookup failed: %s", result.Message)
	}
	if result.Latitude == 0 && result.Longitude == 0 {
		return nil, fmt.Errorf("missing geolocation coordinates for ip %s", ip)
	}

	return &models.GeoLocation{
		Latitude:    result.Latitude,
		Longitude:   result.Longitude,
		CountryCode: strings.ToUpper(result.CountryCode),
		City:        result.City,
	}, nil
}

// IsSubscribed returns subscription status
func (l *Listener) IsSubscribed() bool {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.isSubscribed
}

// MinPaymentDrops returns the currently configured minimum payment amount filter.
func (l *Listener) MinPaymentDrops() int64 {
	return l.minPaymentDrops
}

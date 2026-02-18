package transaction

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/brandon/xrpl-validator-service/internal/models"
	"github.com/brandon/xrpl-validator-service/internal/rippled"
	"github.com/sirupsen/logrus"
)

const rippleEpochOffset = 946684800
const tfPartialPayment = 0x00020000
const reconnectInterval = 5 * time.Second
const defaultTransactionBufferSize = 2048
const defaultGeoEnrichmentQueueSize = 2048
const defaultGeoWorkerCount = 8
const defaultMaxGeoCandidates = 6
const xrplBase58Alphabet = "123456789ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz"

// AccountGeoResolver resolves XRPL accounts to geolocation.
type AccountGeoResolver interface {
	ResolveAccountGeo(ctx context.Context, client rippled.RippledClient, account string) (*models.GeoLocation, error)
}

// Listener handles transaction stream subscriptions and callbacks
type Listener struct {
	client            rippled.RippledClient
	logger            *logrus.Logger
	mu                sync.RWMutex
	callbacks         []TransactionCallback
	isSubscribed      bool
	stopChan          chan struct{}
	transactionBuffer chan *models.Transaction
	geoEnrichmentQ    chan *models.Transaction
	minPaymentDrops   int64
	geoWorkerCount    int
	maxGeoCandidates  int

	geoResolver AccountGeoResolver
}

// ListenerOptions controls listener queueing and enrichment behavior.
type ListenerOptions struct {
	TransactionBufferSize int
	GeoEnrichmentQSize    int
	GeoWorkerCount        int
	MaxGeoCandidates      int
}

// TransactionCallback is a function that processes transactions
type TransactionCallback func(*models.Transaction)

// NewListener creates a new transaction listener
func NewListener(
	client rippled.RippledClient,
	minPaymentDrops int64,
	geoResolver AccountGeoResolver,
	logger *logrus.Logger,
	options ...ListenerOptions,
) *Listener {
	if logger == nil {
		logger = logrus.New()
	}
	if minPaymentDrops <= 0 {
		minPaymentDrops = 1000000
	}
	opts := ListenerOptions{}
	if len(options) > 0 {
		opts = options[0]
	}
	transactionBufferSize := opts.TransactionBufferSize
	if transactionBufferSize <= 0 {
		transactionBufferSize = defaultTransactionBufferSize
	}
	geoQueueSize := opts.GeoEnrichmentQSize
	if geoQueueSize <= 0 {
		geoQueueSize = defaultGeoEnrichmentQueueSize
	}
	geoWorkerCount := opts.GeoWorkerCount
	if geoWorkerCount <= 0 {
		geoWorkerCount = defaultGeoWorkerCount
	}
	maxGeoCandidates := opts.MaxGeoCandidates
	if maxGeoCandidates <= 0 {
		maxGeoCandidates = defaultMaxGeoCandidates
	}

	return &Listener{
		client:            client,
		logger:            logger,
		callbacks:         make([]TransactionCallback, 0),
		stopChan:          make(chan struct{}),
		transactionBuffer: make(chan *models.Transaction, transactionBufferSize),
		geoEnrichmentQ:    make(chan *models.Transaction, geoQueueSize),
		minPaymentDrops:   minPaymentDrops,
		geoWorkerCount:    geoWorkerCount,
		maxGeoCandidates:  maxGeoCandidates,
		geoResolver:       geoResolver,
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
	if l.client == nil {
		return fmt.Errorf("rippled client is nil")
	}

	if !l.client.IsConnected() {
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
	if l.geoResolver != nil {
		for i := 0; i < l.geoWorkerCount; i++ {
			go l.processGeoEnrichment()
		}
	}
	go l.maintainSubscription(ctx)

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

	if l.geoResolver == nil {
		l.enqueueTransaction(tx)
		return
	}

	select {
	case l.geoEnrichmentQ <- tx:
	case <-l.stopChan:
		return
	default:
		l.logger.Warn("Geo enrichment queue full, forwarding transaction without enrichment")
		l.enqueueTransaction(tx)
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

func (l *Listener) processGeoEnrichment() {
	for {
		select {
		case tx := <-l.geoEnrichmentQ:
			l.enrichTransaction(context.Background(), tx)
			l.enqueueTransaction(tx)
		case <-l.stopChan:
			return
		}
	}
}

func (l *Listener) enqueueTransaction(tx *models.Transaction) {
	if tx == nil {
		return
	}
	select {
	case l.transactionBuffer <- tx:
	case <-l.stopChan:
		return
	default:
		l.logger.Warn("Transaction buffer full, dropping transaction")
	}
}

// maintainSubscription reconnects and resubscribes if the WebSocket drops.
func (l *Listener) maintainSubscription(parentCtx context.Context) {
	ticker := time.NewTicker(reconnectInterval)
	defer ticker.Stop()

	for {
		select {
		case <-parentCtx.Done():
			return
		case <-l.stopChan:
			return
		case <-ticker.C:
			l.mu.RLock()
			subscribed := l.isSubscribed
			l.mu.RUnlock()
			if !subscribed || l.client == nil || l.client.IsConnected() {
				continue
			}

			reconnectCtx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
			if err := l.client.Connect(reconnectCtx); err != nil {
				l.logger.WithError(err).Warn("Failed to reconnect transaction stream")
				cancel()
				continue
			}
			if err := l.client.Subscribe(reconnectCtx, []string{"transactions"}, nil); err != nil {
				l.logger.WithError(err).Warn("Failed to resubscribe transaction stream")
			}
			cancel()
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

	amountDrops, ok := parsePaymentAmountDrops(msg, txnRaw)
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

	tx.GeoCandidates = gatherGeoCandidates(txnRaw, msg["meta"], tx.Account, tx.Destination, l.maxGeoCandidates)

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

func parsePaymentAmountDrops(msg map[string]interface{}, txnRaw map[string]interface{}) (int64, bool) {
	if meta, ok := msg["meta"].(map[string]interface{}); ok {
		if drops, ok := parseDeliveredDrops(meta); ok {
			return drops, true
		}
	}

	// Partial payments can advertise a large Amount while delivering less.
	// If delivered amount is unavailable, skip to avoid overstating flow.
	if isPartialPayment(txnRaw) {
		return 0, false
	}

	return parseDrops(txnRaw["Amount"])
}

func parseDeliveredDrops(meta map[string]interface{}) (int64, bool) {
	for _, key := range []string{"delivered_amount", "DeliveredAmount"} {
		value, exists := meta[key]
		if !exists {
			continue
		}
		if asString, ok := value.(string); ok && strings.EqualFold(asString, "unavailable") {
			return 0, false
		}
		if drops, ok := parseDrops(value); ok {
			return drops, true
		}
	}
	return 0, false
}

func isPartialPayment(txnRaw map[string]interface{}) bool {
	flags, ok := toUint32(txnRaw["Flags"])
	if !ok {
		return false
	}
	return (flags & tfPartialPayment) != 0
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
	if l.geoResolver == nil || tx == nil {
		return
	}
	if ctx == nil {
		ctx = context.Background()
	}

	candidates := prioritizeCandidates(tx.GeoCandidates, tx.Account, tx.Destination, l.maxGeoCandidates)
	if len(candidates) == 0 {
		return
	}

	extras := make([]*models.GeoLocation, 0, len(candidates))
	seenExtras := make(map[string]struct{})
	for _, account := range candidates {
		lookupCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
		geo, err := l.geoResolver.ResolveAccountGeo(lookupCtx, l.client, account)
		cancel()
		if err != nil {
			l.logger.WithError(err).WithField("account", account).Debug("Failed to resolve account geolocation")
			continue
		}
		if geo == nil {
			continue
		}

		switch account {
		case tx.Account:
			tx.SourceInfo = geo
		case tx.Destination:
			tx.DestInfo = geo
		default:
			key := fmt.Sprintf("%s:%0.4f:%0.4f", account, geo.Latitude, geo.Longitude)
			if _, exists := seenExtras[key]; exists {
				continue
			}
			seenExtras[key] = struct{}{}
			extras = append(extras, geo)
		}
	}
	if len(extras) > 0 {
		tx.ExtraInfo = extras
	}
}

func gatherGeoCandidates(
	txnRaw map[string]interface{},
	meta interface{},
	account string,
	destination string,
	maxCandidates int,
) []string {
	candidates := make([]string, 0, maxCandidates)
	seen := make(map[string]struct{})
	add := func(candidate string) {
		trimmed := strings.TrimSpace(candidate)
		if !isLikelyXRPLAccount(trimmed) {
			return
		}
		if _, exists := seen[trimmed]; exists {
			return
		}
		seen[trimmed] = struct{}{}
		candidates = append(candidates, trimmed)
	}

	add(account)
	add(destination)
	collectCandidateAccounts(txnRaw, add)
	collectCandidateAccounts(meta, add)

	if maxCandidates > 0 && len(candidates) > maxCandidates {
		return candidates[:maxCandidates]
	}
	return candidates
}

func collectCandidateAccounts(value interface{}, add func(string)) {
	switch typed := value.(type) {
	case map[string]interface{}:
		for key, val := range typed {
			lowerKey := strings.ToLower(strings.TrimSpace(key))
			if shouldParseAsAccount(lowerKey) {
				if asString, ok := val.(string); ok {
					add(asString)
				}
			}
			collectCandidateAccounts(val, add)
		}
	case []interface{}:
		for _, item := range typed {
			collectCandidateAccounts(item, add)
		}
	}
}

func shouldParseAsAccount(key string) bool {
	switch key {
	case "account", "destination", "issuer", "owner", "counterparty", "regularkey":
		return true
	}
	return strings.Contains(key, "issuer") ||
		strings.Contains(key, "account") ||
		strings.Contains(key, "destination") ||
		strings.Contains(key, "owner")
}

func prioritizeCandidates(
	existing []string,
	account string,
	destination string,
	maxCandidates int,
) []string {
	out := make([]string, 0, len(existing)+2)
	seen := make(map[string]struct{})
	add := func(candidate string) {
		trimmed := strings.TrimSpace(candidate)
		if !isLikelyXRPLAccount(trimmed) {
			return
		}
		if _, exists := seen[trimmed]; exists {
			return
		}
		seen[trimmed] = struct{}{}
		out = append(out, trimmed)
	}

	add(account)
	add(destination)
	for _, candidate := range existing {
		add(candidate)
	}

	if maxCandidates > 0 && len(out) > maxCandidates {
		return out[:maxCandidates]
	}
	return out
}

func isLikelyXRPLAccount(account string) bool {
	if len(account) < 25 || len(account) > 40 {
		return false
	}
	if !strings.HasPrefix(account, "r") {
		return false
	}
	for _, r := range account {
		if !strings.ContainsRune(xrplBase58Alphabet, r) {
			return false
		}
	}
	return true
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

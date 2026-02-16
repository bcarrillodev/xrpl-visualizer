package transaction

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/brandon/xrpl-validator-service/internal/models"
	"github.com/brandon/xrpl-validator-service/internal/rippled"
	"github.com/sirupsen/logrus"
)

// Listener handles transaction stream subscriptions and callbacks
type Listener struct {
	client            rippled.RippledClient
	logger            *logrus.Logger
	mu                sync.RWMutex
	callbacks         []TransactionCallback
	isSubscribed      bool
	stopChan          chan struct{}
	validatorFetcher  ValidatorFetcher // For geolocation enrichment
	transactionBuffer chan *models.Transaction
	bufferSize        int
}

// TransactionCallback is a function that processes transactions
type TransactionCallback func(*models.Transaction)

// ValidatorFetcher defines interface for validator lookup
type ValidatorFetcher interface {
	GetValidator(address string) *models.Validator
	GetValidators() []*models.Validator
}

// NewListener creates a new transaction listener
func NewListener(client rippled.RippledClient, validatorFetcher ValidatorFetcher, logger *logrus.Logger) *Listener {
	if logger == nil {
		logger = logrus.New()
	}
	return &Listener{
		client:            client,
		logger:            logger,
		callbacks:         make([]TransactionCallback, 0),
		stopChan:          make(chan struct{}),
		validatorFetcher:  validatorFetcher,
		transactionBuffer: make(chan *models.Transaction, 100),
		bufferSize:        100,
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

	// Subscribe to transaction stream
	err := l.client.Subscribe(ctx, []string{"transactions"}, l.handleMessage)
	if err != nil {
		l.logger.WithError(err).Warn("Failed to subscribe to transactions; continuing with demo transaction generator")
	}

	l.mu.Lock()
	l.isSubscribed = true
	l.mu.Unlock()

	l.logger.Info("Transaction listener started")

	// Start processor goroutine
	go l.processTransactions()

	// Start fake transaction generator for demo
	go l.generateFakeTransactions()

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
		l.logger.Warn("Received non-map message")
		return
	}

	// Check if this is a transaction message
	if msgType, ok := msgMap["type"].(string); ok && msgType == "transaction" {
		tx, err := l.parseTransaction(msgMap)
		if err != nil {
			l.logger.WithError(err).Warn("Failed to parse transaction")
			return
		}

		// Enrich with geolocation if available
		l.enrichTransaction(tx)

		// Send to buffer for processing
		select {
		case l.transactionBuffer <- tx:
		case <-l.stopChan:
			return
		default:
			l.logger.Warn("Transaction buffer full, dropping transaction")
		}
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

// parseTransaction converts a raw message to a Transaction model
func (l *Listener) parseTransaction(msg map[string]interface{}) (*models.Transaction, error) {
	data, err := json.Marshal(msg)
	if err != nil {
		return nil, err
	}

	var tx models.Transaction
	if err := json.Unmarshal(data, &tx); err != nil {
		return nil, err
	}

	tx.Timestamp = time.Now().Unix()
	tx.Validated = true // Assume validated transactions for now

	return &tx, nil
}

// enrichTransaction adds geolocation data to transaction
func (l *Listener) enrichTransaction(tx *models.Transaction) {
	if l.validatorFetcher == nil {
		return
	}

	// Enrich source information
	if sourceValidator := l.validatorFetcher.GetValidator(tx.Account); sourceValidator != nil {
		tx.SourceInfo = &models.GeoLocation{
			Latitude:         sourceValidator.Latitude,
			Longitude:        sourceValidator.Longitude,
			CountryCode:      sourceValidator.CountryCode,
			City:             sourceValidator.City,
			ValidatorAddress: sourceValidator.Address,
		}
	}

	// Enrich destination information
	if destValidator := l.validatorFetcher.GetValidator(tx.Destination); destValidator != nil {
		tx.DestInfo = &models.GeoLocation{
			Latitude:         destValidator.Latitude,
			Longitude:        destValidator.Longitude,
			CountryCode:      destValidator.CountryCode,
			City:             destValidator.City,
			ValidatorAddress: destValidator.Address,
		}
	}
}

// IsSubscribed returns subscription status
func (l *Listener) IsSubscribed() bool {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.isSubscribed
}

// generateFakeTransactions creates demo transactions for visualization
func (l *Listener) generateFakeTransactions() {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	fakeTxCounter := 0

	for {
		select {
		case <-ticker.C:
			tx := l.createFakeTransaction(fakeTxCounter)
			fakeTxCounter++

			select {
			case l.transactionBuffer <- tx:
				l.logger.WithField("hash", tx.Hash).Debug("Generated fake transaction")
			case <-l.stopChan:
				return
			default:
				l.logger.Warn("Transaction buffer full, skipping fake transaction")
			}

		case <-l.stopChan:
			return
		}
	}
}

// createFakeTransaction generates a fake transaction for demo purposes
func (l *Listener) createFakeTransaction(counter int) *models.Transaction {
	// Get all validators for source/dest selection
	validators := l.getAllValidators()

	if len(validators) < 2 {
		// Fallback if no validators
		return &models.Transaction{
			Hash:              fmt.Sprintf("fake_%d", counter),
			LedgerIndex:       uint32(1000000 + counter),
			Account:           "rFakeSource",
			Destination:       "rFakeDest",
			TransactionType:   "Payment",
			Amount:            "1000000", // 1 XRP in drops
			Fee:               "10",
			TransactionResult: "tesSUCCESS",
			Timestamp:         time.Now().Unix(),
			Validated:         true,
		}
	}

	// Select source and dest validators
	sourceIdx := counter % len(validators)
	destIdx := (counter + 1) % len(validators)

	source := validators[sourceIdx]
	dest := validators[destIdx]

	// Alternate transaction types
	txTypes := []string{"Payment", "TrustSet", "OfferCreate"}
	txType := txTypes[counter%len(txTypes)]

	amount := "1000000" // 1 XRP
	if txType == "TrustSet" {
		amount = "1000000000" // 1000 XRP limit
	}

	tx := &models.Transaction{
		Hash:              fmt.Sprintf("fake_%d", counter),
		LedgerIndex:       uint32(1000000 + counter),
		Account:           source.Address,
		Destination:       dest.Address,
		TransactionType:   txType,
		Amount:            amount,
		Fee:               "10",
		TransactionResult: "tesSUCCESS",
		Timestamp:         time.Now().Unix(),
		Validated:         true,
	}

	// Enrich with geolocation
	l.enrichTransaction(tx)

	return tx
}

// getAllValidators returns all known validators (hacky, but for demo)
func (l *Listener) getAllValidators() []*models.Validator {
	return l.validatorFetcher.GetValidators()
}

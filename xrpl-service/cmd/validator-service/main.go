package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/brandon/xrpl-validator-service/internal/config"
	"github.com/brandon/xrpl-validator-service/internal/geolocation"
	"github.com/brandon/xrpl-validator-service/internal/metrics"
	"github.com/brandon/xrpl-validator-service/internal/server"
	"github.com/brandon/xrpl-validator-service/internal/transaction"
	"github.com/brandon/xrpl-validator-service/internal/validator"
	"github.com/brandon/xrpl-validator-service/internal/xrpl"
	"github.com/sirupsen/logrus"
)

func main() {
	// Load configuration
	cfg := config.NewConfig()

	// Validate configuration
	if err := cfg.Validate(); err != nil {
		panic(fmt.Sprintf("Invalid configuration: %v", err))
	}

	// Initialize logger
	logger := logrus.New()
	logger.SetFormatter(&logrus.JSONFormatter{})
	logLevel, err := logrus.ParseLevel(cfg.LogLevel)
	if err != nil {
		logLevel = logrus.InfoLevel
	}
	logger.SetLevel(logLevel)

	logger.WithFields(logrus.Fields{
		"validator_json_rpc":  cfg.PublicXRPLJSONRPCURL,
		"validator_websocket": cfg.PublicXRPLWebSocketURL,
		"tx_json_rpc":         cfg.TransactionJSONRPCURL,
		"tx_websocket":        cfg.TransactionWebSocketURL,
		"tx_buffer_size":      cfg.TransactionBufferSize,
		"geo_enrichment_q":    cfg.GeoEnrichmentQSize,
		"geo_enrichment_w":    cfg.GeoEnrichmentWorkers,
		"max_geo_candidates":  cfg.MaxGeoCandidates,
		"broadcast_buffer":    cfg.BroadcastBufferSize,
		"ws_client_buffer":    cfg.WSClientBufferSize,
		"geolite_db_path":     cfg.GeoLiteDBPath,
		"network":             cfg.Network,
		"listen_addr":         cfg.ListenAddr,
		"listen_port":         cfg.ListenPort,
	}).Info("XRPL Validator Service starting")

	validatorClient := xrpl.NewClient(cfg.PublicXRPLJSONRPCURL, cfg.PublicXRPLWebSocketURL, logger)
	txClient := xrpl.NewClient(cfg.TransactionJSONRPCURL, cfg.TransactionWebSocketURL, logger)
	appCtx, appCancel := context.WithCancel(context.Background())
	defer appCancel()

	geoResolver, err := geolocation.NewResolver(logger, geolocation.ResolverConfig{
		CachePath:          cfg.GeoCachePath,
		GeoLiteDBPath:      cfg.GeoLiteDBPath,
		GeoLiteDownloadURL: cfg.GeoLiteDownloadURL,
		AutoDownload:       cfg.GeoLiteAutoDownload,
	})
	if err != nil {
		logger.WithError(err).Fatal("Failed to initialize GeoLite resolver")
	}
	defer func() {
		if err := geoResolver.Close(); err != nil {
			logger.WithError(err).Warn("Error closing GeoLite resolver")
		}
	}()

	// Create validator fetcher
	validatorFetcher := validator.NewFetcher(
		validatorClient,
		time.Duration(cfg.ValidatorRefreshInterval)*time.Second,
		geoResolver,
		cfg.ValidatorListSites,
		cfg.SecondaryValidatorRegistryURL,
		cfg.ValidatorMetadataCachePath,
		cfg.NetworkHealthJSONRPCURLs,
		cfg.NetworkHealthRetries,
		cfg.Network,
		logger,
	)
	validatorFetcher.Start(appCtx)

	// Create transaction listener
	transactionListener := transaction.NewListener(
		txClient,
		cfg.MinPaymentDrops,
		geoResolver,
		logger,
		transaction.ListenerOptions{
			TransactionBufferSize: cfg.TransactionBufferSize,
			GeoEnrichmentQSize:    cfg.GeoEnrichmentQSize,
			GeoWorkerCount:        cfg.GeoEnrichmentWorkers,
			MaxGeoCandidates:      cfg.MaxGeoCandidates,
		},
	)
	if err := transactionListener.Start(appCtx); err != nil {
		metrics.ValidatorFetchTotal.WithLabelValues("error").Inc() // Note: reusing for listener start
		logger.WithError(err).Error("Failed to start transaction listener")
	}

	// Create HTTP server
	httpServer := server.NewServer(
		validatorFetcher,
		transactionListener,
		cfg.ListenAddr,
		cfg.ListenPort,
		cfg.CORSAllowedOrigins,
		cfg.BroadcastBufferSize,
		cfg.WSClientBufferSize,
		logger,
	)

	// Start HTTP server in a goroutine
	go func() {
		logger.Info("HTTP Server started")
		if err := httpServer.Start(appCtx); err != nil && err.Error() != "http: Server closed" {
			logger.WithError(err).Fatal("HTTP server error")
		}
	}()

	// Wait for interrupt signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	logger.Info("Shutdown signal received")
	appCancel()

	// Graceful shutdown
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()

	// Stop transaction listener
	if err := transactionListener.Stop(shutdownCtx); err != nil {
		logger.WithError(err).Error("Error stopping transaction listener")
	}

	// Stop validator fetcher
	validatorFetcher.Stop()

	// Stop HTTP server
	if err := httpServer.Stop(shutdownCtx); err != nil {
		logger.WithError(err).Error("Error stopping HTTP server")
	}

	// Close XRPL clients
	if err := validatorClient.Close(); err != nil {
		logger.WithError(err).Error("Error closing validator source client")
	}
	if err := txClient.Close(); err != nil {
		logger.WithError(err).Error("Error closing transaction source client")
	}

	logger.Info("Service shutdown complete")
}

package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/brandon/xrpl-validator-service/internal/config"
	"github.com/brandon/xrpl-validator-service/internal/metrics"
	"github.com/brandon/xrpl-validator-service/internal/rippled"
	"github.com/brandon/xrpl-validator-service/internal/server"
	"github.com/brandon/xrpl-validator-service/internal/transaction"
	"github.com/brandon/xrpl-validator-service/internal/validator"
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
		"rippled_json_rpc":  cfg.RippledJSONRPCURL,
		"rippled_websocket": cfg.RippledWebSocketURL,
		"network":           cfg.Network,
		"listen_addr":       cfg.ListenAddr,
		"listen_port":       cfg.ListenPort,
	}).Info("XRPL Validator Service starting")

	// Create rippled client
	rippledClient := rippled.NewClient(cfg.RippledJSONRPCURL, cfg.RippledWebSocketURL, logger)

	// Attempt to connect to rippled
	connectCtx, connectCancel := context.WithTimeout(context.Background(), 10*time.Second)
	if err := rippledClient.Connect(connectCtx); err != nil {
		connectCancel()
		logger.WithError(err).Fatal("Failed to connect to rippled")
	}
	connectCancel()

	// Create geolocation provider (try real, fallback to demo)
	geoProvider := validator.NewRealGeoLocationProvider(logger)

	// Create validator fetcher
	validatorFetcher := validator.NewFetcher(
		rippledClient,
		time.Duration(cfg.ValidatorRefreshInterval)*time.Second,
		geoProvider,
		cfg.ValidatorListSites,
		cfg.Network,
		logger,
	)
	validatorFetcher.Start(context.Background())

	// Create transaction listener
	transactionListener := transaction.NewListener(rippledClient, cfg.MinPaymentDrops, logger)
	if err := transactionListener.Start(context.Background()); err != nil {
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
		logger,
	)

	// Start HTTP server in a goroutine
	go func() {
		logger.Info("HTTP Server started")
		if err := httpServer.Start(context.Background()); err != nil && err.Error() != "http: Server closed" {
			logger.WithError(err).Fatal("HTTP server error")
		}
	}()

	// Wait for interrupt signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	logger.Info("Shutdown signal received")

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

	// Close rippled client
	if err := rippledClient.Close(); err != nil {
		logger.WithError(err).Error("Error closing rippled client")
	}

	logger.Info("Service shutdown complete")
}

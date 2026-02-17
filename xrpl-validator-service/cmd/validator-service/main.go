package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strings"
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
		"source_mode":      cfg.SourceMode,
		"local_json_rpc":   cfg.RippledJSONRPCURL,
		"local_websocket":  cfg.RippledWebSocketURL,
		"public_json_rpc":  cfg.PublicRippledJSONRPCURL,
		"public_websocket": cfg.PublicRippledWebSocketURL,
		"network":          cfg.Network,
		"listen_addr":      cfg.ListenAddr,
		"listen_port":      cfg.ListenPort,
	}).Info("XRPL Validator Service starting")

	localClient := rippled.NewClient(cfg.RippledJSONRPCURL, cfg.RippledWebSocketURL, logger)
	publicClient := rippled.NewClient(cfg.PublicRippledJSONRPCURL, cfg.PublicRippledWebSocketURL, logger)
	validatorClient, txClient := selectClients(cfg, localClient, publicClient, logger)
	appCtx, appCancel := context.WithCancel(context.Background())
	defer appCancel()

	// Create geolocation provider (try real, fallback to demo)
	geoProvider := validator.NewRealGeoLocationProvider(logger, validator.RealGeoLocationConfig{
		CachePath:         cfg.GeoCachePath,
		MinLookupInterval: time.Duration(cfg.GeoLookupMinIntervalMS) * time.Millisecond,
		RateLimitCooldown: time.Duration(cfg.GeoRateLimitCooldownSeconds) * time.Second,
	})

	// Create validator fetcher
	validatorFetcher := validator.NewFetcher(
		validatorClient,
		time.Duration(cfg.ValidatorRefreshInterval)*time.Second,
		geoProvider,
		cfg.ValidatorListSites,
		cfg.SecondaryValidatorRegistryURL,
		cfg.ValidatorMetadataCachePath,
		cfg.Network,
		logger,
	)
	validatorFetcher.Start(appCtx)
	if cfg.SourceMode == "hybrid" {
		startHybridValidatorSourceMonitor(appCtx, validatorFetcher, localClient, publicClient, logger)
	}

	// Create transaction listener
	transactionListener := transaction.NewListener(txClient, cfg.MinPaymentDrops, logger)
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

	// Close rippled clients
	if err := localClient.Close(); err != nil {
		logger.WithError(err).Error("Error closing local rippled client")
	}
	if publicClient != localClient {
		if err := publicClient.Close(); err != nil {
			logger.WithError(err).Error("Error closing public rippled client")
		}
	}

	logger.Info("Service shutdown complete")
}

func selectClients(cfg *config.Config, localClient, publicClient rippled.RippledClient, logger *logrus.Logger) (rippled.RippledClient, rippled.RippledClient) {
	switch cfg.SourceMode {
	case "local":
		logger.Info("Using local rippled for validators and transactions")
		return localClient, localClient
	case "public":
		logger.Info("Using public rippled for validators and transactions")
		return publicClient, publicClient
	case "hybrid":
		validatorClient := publicClient
		txClient := publicClient

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		ready, reason := isLocalReady(ctx, localClient)
		cancel()
		if ready {
			validatorClient = localClient
			logger.Info("Using hybrid mode: local ready for validators/health; public for transactions")
		} else {
			logger.WithField("reason", reason).Info("Using hybrid mode: public transactions and validators until local rippled is ready")
		}
		return validatorClient, txClient
	default:
		logger.Warn("Unknown source mode; defaulting to hybrid")
		return publicClient, publicClient
	}
}

func startHybridValidatorSourceMonitor(ctx context.Context, fetcher *validator.Fetcher, localClient, publicClient rippled.RippledClient, logger *logrus.Logger) {
	ticker := time.NewTicker(30 * time.Second)

	go func() {
		defer ticker.Stop()

		current := "public"
		timeoutCtx, timeoutCancel := context.WithTimeout(ctx, 5*time.Second)
		if ready, _ := isLocalReady(timeoutCtx, localClient); ready {
			current = "local"
		}
		timeoutCancel()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				checkCtx, checkCancel := context.WithTimeout(ctx, 5*time.Second)
				ready, reason := isLocalReady(checkCtx, localClient)
				checkCancel()

				next := "public"
				if ready {
					next = "local"
				}
				if next == current {
					continue
				}

				if next == "local" {
					fetcher.SetClient(localClient)
					logger.Info("Hybrid mode switched validator/health source to local rippled")
				} else {
					fetcher.SetClient(publicClient)
					logger.WithField("reason", reason).Warn("Hybrid mode switched validator/health source to public rippled")
				}
				current = next

				refreshCtx, refreshCancel := context.WithTimeout(ctx, 20*time.Second)
				if err := fetcher.Fetch(refreshCtx); err != nil {
					logger.WithError(err).Warn("Hybrid source switch refresh failed")
				}
				refreshCancel()
			}
		}
	}()
}

func isLocalReady(ctx context.Context, client rippled.RippledClient) (bool, string) {
	result, err := client.GetServerInfo(ctx)
	if err != nil {
		return false, err.Error()
	}

	resultMap, ok := result.(map[string]interface{})
	if !ok {
		return false, "unexpected server_info format"
	}
	payload, ok := resultMap["result"].(map[string]interface{})
	if !ok {
		return false, "missing result payload"
	}
	info, ok := payload["info"].(map[string]interface{})
	if !ok {
		return false, "missing info payload"
	}

	serverState, _ := info["server_state"].(string)
	completeLedgers, _ := info["complete_ledgers"].(string)
	if strings.TrimSpace(completeLedgers) == "" {
		return false, "complete_ledgers empty"
	}

	switch strings.ToLower(serverState) {
	case "full", "proposing", "validating":
		return true, ""
	default:
		return false, fmt.Sprintf("server_state=%s", serverState)
	}
}

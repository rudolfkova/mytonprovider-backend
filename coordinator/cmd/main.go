package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/prometheus/client_golang/prometheus"

	"mytonprovider-coordinator/internal/cache"
	"mytonprovider-coordinator/internal/clients/agentrpc"
	"mytonprovider-coordinator/internal/clients/ifconfig"
	tonclient "mytonprovider-coordinator/internal/clients/ton"
	"mytonprovider-coordinator/internal/httpServer"
	providersRepository "mytonprovider-coordinator/internal/repositories/providers"
	systemRepository "mytonprovider-coordinator/internal/repositories/system"
	providersService "mytonprovider-coordinator/internal/services/providers"
	"mytonprovider-coordinator/internal/workers"
	"mytonprovider-coordinator/internal/workers/cleaner"
	providersmaster "mytonprovider-coordinator/internal/workers/providersMaster"
	"mytonprovider-coordinator/internal/workers/telemetry"
)

func main() {
	if err := run(); err != nil {
		os.Exit(1)
	}
}

func run() (err error) {
	// Tools
	config := loadConfig()
	if config == nil {
		fmt.Println("failed to load configuration")
		return
	}

	logLevel := slog.LevelInfo
	if level, ok := logLevels[config.System.LogLevel]; ok {
		logLevel = level
	}

	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: logLevel,
	}))
	telemetryCache := cache.NewSimpleCache(2 * time.Minute)
	benchmarksCache := cache.NewSimpleCache(2 * time.Minute)

	// Metrics
	dbRequestsCount := prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: config.Metrics.Namespace,
			Subsystem: config.Metrics.DbSubsystem,
			Name:      "db_requests_count",
			Help:      "Db requests count",
		},
		[]string{"method", "error"},
	)

	dbRequestsDuration := prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: config.Metrics.Namespace,
			Subsystem: config.Metrics.DbSubsystem,
			Name:      "db_requests_duration",
			Help:      "Db requests duration",
		},
		[]string{"method", "error"},
	)

	workersRunCount := prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: config.Metrics.Namespace,
			Subsystem: config.Metrics.DbSubsystem,
			Name:      "workers_requests_count",
			Help:      "Workers requests count",
		},
		[]string{"method", "error"},
	)

	workersRunDuration := prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: config.Metrics.Namespace,
			Subsystem: config.Metrics.DbSubsystem,
			Name:      "workers_requests_duration",
			Help:      "Workers requests duration",
		},
		[]string{"method", "error"},
	)

	providersNetLoad := prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: config.Metrics.Namespace,
			Subsystem: config.Metrics.DbSubsystem,
			Name:      "providers_net_load",
			Help:      "Providers network load",
		},
		[]string{"provider_pubkey", "type"},
	)

	prometheus.MustRegister(
		dbRequestsCount,
		dbRequestsDuration,
		workersRunCount,
		workersRunDuration,
		providersNetLoad,
	)

	// Clients
	ton, err := tonclient.NewClient(context.Background(), config.TON.ConfigURL, logger)
	if err != nil {
		logger.Error("failed to create TON client", slog.String("error", err.Error()))
		return
	}

	ipinfo := ifconfig.NewClient(logger)

	dhtClient, providerClient, err := newProviderClient(context.Background(), config.TON.ConfigURL, config.System.ADNLPort, config.System.Key)
	if err != nil {
		logger.Error("failed to create provider client", slog.String("error", err.Error()))
		return
	}

	agentEndpoints := agentrpc.ParseEndpointsCSV(config.Agents.Endpoints)
	agentRPC, err := agentrpc.New(agentrpc.Config{
		Endpoints:      agentEndpoints,
		AuthToken:      config.Agents.AuthToken,
		CACertFile:     config.Agents.CACertFile,
		RequestTimeout: time.Duration(config.Agents.RequestTimeoutMs) * time.Millisecond,
	}, logger)
	if err != nil {
		logger.Error("failed to initialize agent RPC client", slog.String("error", err.Error()))
	}
	if agentRPC != nil {
		defer func() {
			if cErr := agentRPC.Close(); cErr != nil {
				logger.Warn("failed to close agent RPC clients", slog.String("error", cErr.Error()))
			}
		}()
	}

	// Postgres
	connPool, err := connectPostgres(context.Background(), config, logger)
	if err != nil {
		logger.Error("failed to connect to Postgres", slog.String("error", err.Error()))
		return
	}

	// Database
	providersRepo := providersRepository.NewRepository(connPool)
	providersRepo = providersRepository.NewMetrics(dbRequestsCount, dbRequestsDuration, providersRepo)

	systemRepo := systemRepository.NewRepository(connPool)
	systemRepo = systemRepository.NewMetrics(dbRequestsCount, dbRequestsDuration, systemRepo)

	// Workers
	telemetryWorker := telemetry.NewWorker(providersRepo, telemetryCache, benchmarksCache, providersNetLoad, logger)
	telemetryWorker = telemetry.NewMetrics(workersRunCount, workersRunDuration, telemetryWorker)

	providersMasterWorker := providersmaster.NewWorker(
		providersRepo,
		systemRepo,
		ton,
		providerClient,
		dhtClient,
		ipinfo,
		agentRPC,
		config.TON.MasterAddress,
		config.TON.BatchSize,
		providersmaster.RunChecksTimeouts{
			PingMs:              config.Agents.RunChecksPingMs,
			RldpMs:              config.Agents.RunChecksRldpMs,
			TotalMs:             config.Agents.RunChecksTotalMs,
			StorageRatesQueryMs: config.Agents.StorageRatesQueryMs,
		},
		logger,
	)
	providersMasterWorker = providersmaster.NewMetrics(workersRunCount, workersRunDuration, providersMasterWorker)

	cleanerWorker := cleaner.NewWorker(providersRepo, config.System.StoreHistoryDays, logger)
	cleanerWorker = cleaner.NewMetrics(workersRunCount, workersRunDuration, cleanerWorker)

	cancelCtx, cancel := context.WithCancel(context.Background())
	workers := workers.NewWorkers(telemetryWorker, providersMasterWorker, cleanerWorker, logger)
	go func() {
		if wErr := workers.Start(cancelCtx); wErr != nil {
			logger.Error("failed to start workers", slog.String("error", wErr.Error()))
			err = wErr
			return
		}
	}()

	// Services
	providersSvc := providersService.NewService(providersRepo, logger)
	providersSvc = providersService.NewCacheMiddleware(providersSvc, telemetryCache, benchmarksCache)

	// HTTP Server
	accessTokens := strings.Split(config.System.AccessTokens, ",")
	app := fiber.New()
	server := httpServer.New(
		app,
		providersSvc,
		accessTokens,
		config.Metrics.Namespace,
		config.Metrics.ServerSubsystem,
		logger,
	)

	server.RegisterRoutes()

	go func() {
		if err := app.Listen(":" + config.System.Port); err != nil {
			logger.Error("error starting server", slog.String("err", err.Error()))
		}
	}()

	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, syscall.SIGINT, syscall.SIGTERM)

	<-signalChan
	cancel()

	err = app.ShutdownWithTimeout(time.Second * 5)
	if err != nil {
		logger.Error("server shut down error", slog.String("err", err.Error()))
		return err
	}

	return err
}

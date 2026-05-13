package main

import (
	"context"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"mytonprovider-agent/internal/config"
	"mytonprovider-agent/internal/grpcserver"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))

	cfg, err := config.Load()
	if err != nil {
		logger.Error("failed to load config", "error", err)
		os.Exit(1)
	}

	var metricsSrv *http.Server
	if addr := cfg.MetricsListenAddr; addr != "" {
		metricsLis, err := net.Listen("tcp", addr)
		if err != nil {
			logger.Error("failed to listen metrics", "address", addr, "error", err)
			os.Exit(1)
		}
		metricsSrv = &http.Server{
			Handler: promhttp.HandlerFor(prometheus.DefaultGatherer, promhttp.HandlerOpts{
				EnableOpenMetrics: true,
			}),
		}
		go func() {
			logger.Info("agent metrics server started", "listen_addr", metricsLis.Addr().String())
			if err := metricsSrv.Serve(metricsLis); err != nil && err != http.ErrServerClosed {
				logger.Error("metrics server exited", "error", err)
			}
		}()
	}

	lis, err := net.Listen("tcp", cfg.ListenAddr)
	if err != nil {
		logger.Error("failed to create listener", "address", cfg.ListenAddr, "error", err)
		shutdownMetrics(metricsSrv, logger)
		os.Exit(1)
	}

	server, cleanup, err := grpcserver.New(cfg, logger)
	if err != nil {
		logger.Error("failed to initialize gRPC server", "error", err)
		shutdownMetrics(metricsSrv, logger)
		os.Exit(1)
	}
	defer cleanup()
	defer shutdownMetrics(metricsSrv, logger)

	serveErr := make(chan error, 1)
	go func() {
		logger.Info("agent gRPC server started", "listen_addr", cfg.ListenAddr, "agent_id", cfg.AgentID, "location", cfg.Location)
		serveErr <- server.Serve(lis)
	}()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	select {
	case sig := <-sigCh:
		logger.Info("shutdown signal received", "signal", sig.String())
	case err := <-serveErr:
		if err != nil {
			logger.Error("gRPC server exited with error", "error", err)
			shutdownMetrics(metricsSrv, logger)
			os.Exit(1)
		}
		return
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	done := make(chan struct{})
	go func() {
		server.GracefulStop()
		close(done)
	}()

	select {
	case <-done:
		logger.Info("gRPC server stopped gracefully")
	case <-shutdownCtx.Done():
		logger.Warn("graceful shutdown timed out, forcing stop")
		server.Stop()
	}
}

func shutdownMetrics(srv *http.Server, log *slog.Logger) {
	if srv == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Warn("metrics server shutdown", "error", err)
	}
}

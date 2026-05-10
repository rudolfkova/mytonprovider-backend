package main

import (
	"context"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

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

	lis, err := net.Listen("tcp", cfg.ListenAddr)
	if err != nil {
		logger.Error("failed to create listener", "address", cfg.ListenAddr, "error", err)
		os.Exit(1)
	}

	server, err := grpcserver.New(cfg, logger)
	if err != nil {
		logger.Error("failed to initialize gRPC server", "error", err)
		os.Exit(1)
	}

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

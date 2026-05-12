package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"mytonprovider-coordinator/internal/clients/ifconfig"
	tonclient "mytonprovider-coordinator/internal/clients/ton"
	providersRepository "mytonprovider-coordinator/internal/repositories/providers"
	providersinmemory "mytonprovider-coordinator/internal/repositories/providers/inmemory"
	systemRepository "mytonprovider-coordinator/internal/repositories/system"
	systeminmemory "mytonprovider-coordinator/internal/repositories/system/inmemory"
	providersmaster "mytonprovider-coordinator/internal/workers/providersMaster"

	"github.com/xssnick/tonutils-go/address"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func run() error {
	var (
		outPath       string
		providerLimit int
		jobID         string
		envelope      bool
		storage       string
		syncTimeout   time.Duration
		pingMS        uint
		rldpMS        uint
		totalMS       uint
	)

	flag.StringVar(&outPath, "out", "", "output file path (default: stdout)")
	flag.IntVar(&providerLimit, "limit", 1, "max number of providers in dumped request (0 = all)")
	flag.StringVar(&jobID, "job-id", "", "job id override")
	flag.BoolVar(&envelope, "envelope", false, "wrap output into {meta,request} envelope")
	flag.StringVar(&storage, "storage", "memory", "memory (scan chain into RAM, no Postgres) or postgres")
	flag.DurationVar(&syncTimeout, "sync-timeout", 15*time.Minute, "timeout for chain sync steps (memory mode)")
	flag.UintVar(&pingMS, "ping-ms", 7000, "ping timeout in milliseconds")
	flag.UintVar(&rldpMS, "rldp-ms", 10000, "rldp timeout in milliseconds")
	flag.UintVar(&totalMS, "total-ms", 30000, "total timeout in milliseconds")
	flag.Parse()

	storage = strings.ToLower(strings.TrimSpace(storage))
	if storage != "memory" && storage != "postgres" {
		return fmt.Errorf(`--storage must be "memory" or "postgres", got %q`, storage)
	}

	cfg := loadConfig(storage == "postgres")
	if cfg == nil {
		return fmt.Errorf("failed to load configuration")
	}

	if storage == "memory" {
		ma := strings.TrimSpace(cfg.TON.MasterAddress)
		if ma == "" {
			return fmt.Errorf("MASTER_ADDRESS is required for --storage=memory (coordinator master wallet, user-friendly format)")
		}
		if _, err := address.ParseAddr(ma); err != nil {
			return fmt.Errorf("invalid MASTER_ADDRESS %q: %w", ma, err)
		}
	}

	logLevel := slog.LevelInfo
	if level, ok := logLevels[cfg.System.LogLevel]; ok {
		logLevel = level
	}
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: logLevel}))

	ctx, cancel := context.WithTimeout(context.Background(), syncTimeout)
	defer cancel()

	ton, err := tonclient.NewClient(ctx, cfg.TON.ConfigURL, logger)
	if err != nil {
		return fmt.Errorf("create TON client: %w", err)
	}

	ipinfo := ifconfig.NewClient(logger)
	dhtClient, providerClient, err := newProviderClient(ctx, cfg.TON.ConfigURL, cfg.System.ADNLPort, cfg.System.Key)
	if err != nil {
		return fmt.Errorf("create provider client: %w", err)
	}

	switch storage {
	case "memory":
		var lastErr error
		for attempt := 1; attempt <= memoryDumpMaxAttempts; attempt++ {
			memP := providersinmemory.NewRepository()
			memS := systeminmemory.NewRepository()
			worker := providersmaster.NewWorker(
				memP,
				memS,
				ton,
				providerClient,
				dhtClient,
				ipinfo,
				cfg.TON.MasterAddress,
				cfg.TON.BatchSize,
				logger,
			)
			if worker == nil {
				return fmt.Errorf("worker is nil")
			}
			if _, err := worker.CollectNewProviders(ctx); err != nil {
				return fmt.Errorf("CollectNewProviders: %w", err)
			}
			if _, err := worker.CollectProvidersNewStorageContracts(ctx); err != nil {
				return fmt.Errorf("CollectProvidersNewStorageContracts: %w", err)
			}
			logger.Info("memory sync finished", "master", cfg.TON.MasterAddress, "attempt", attempt, "max_attempts", memoryDumpMaxAttempts)

			builder := providersmaster.NewRequestBuilder(
				memP, memS, ton, providerClient, dhtClient, ipinfo, logger,
			)
			lastErr = writeRunChecksDump(ctx, builder, ipinfo, logger, outPath, jobID, providerLimit, envelope, storage, pingMS, rldpMS, totalMS)
			if lastErr == nil {
				return nil
			}
			if attempt == memoryDumpMaxAttempts || !shouldRetryMemoryDump(lastErr) {
				return lastErr
			}

			logger.Warn(
				"memory dump produced no usable contracts, retrying",
				"attempt", attempt,
				"max_attempts", memoryDumpMaxAttempts,
				"retry_delay", memoryDumpRetryDelay.String(),
				"error", lastErr,
			)

			timer := time.NewTimer(memoryDumpRetryDelay)
			select {
			case <-ctx.Done():
				timer.Stop()
				return ctx.Err()
			case <-timer.C:
			}
		}

		return lastErr

	case "postgres":
		connPool, err := connectPostgres(ctx, cfg, logger)
		if err != nil {
			return fmt.Errorf("connect Postgres: %w", err)
		}
		defer connPool.Close()

		provRepo := providersRepository.NewRepository(connPool)
		sysRepo := systemRepository.NewRepository(connPool)
		builder := providersmaster.NewRequestBuilder(
			provRepo, sysRepo, ton, providerClient, dhtClient, ipinfo, logger,
		)
		return writeRunChecksDump(ctx, builder, ipinfo, logger, outPath, jobID, providerLimit, envelope, storage, pingMS, rldpMS, totalMS)
	}

	return fmt.Errorf("unknown storage %q", storage)
}

func metaPathForDump(outPath string) string {
	base := strings.TrimSuffix(outPath, filepath.Ext(outPath))
	return base + "-meta.json"
}

type providerGeoMetaRow struct {
	ProviderPubkey  string `json:"providerPubkey"`
	ProviderAddress string `json:"providerAddress"`
	StorageIP       string `json:"storageIp"`
	Country         string `json:"country,omitempty"`
	CountryISO      string `json:"countryIso,omitempty"`
	City            string `json:"city,omitempty"`
	GeoLookupError  string `json:"geoLookupError,omitempty"`
}

type runChecksMetaFile struct {
	GeneratedAt string               `json:"generatedAt"`
	Source      string               `json:"source"`
	Providers   []providerGeoMetaRow `json:"providers"`
}

const (
	geoLookupTimeout = 10 * time.Second
	geoLookupDelay   = 1 * time.Second
	memoryDumpMaxAttempts = 3
	memoryDumpRetryDelay  = 3 * time.Second
)

type providerGeoLookup struct {
	Country        string
	CountryISO     string
	City           string
	GeoLookupError string
}

func writeProviderGeoMeta(ctx context.Context, ipinfo ifconfig.IFConfig, req *providersmaster.RunChecksRequestPayload, metaPath string, logger *slog.Logger) error {
	if req == nil || len(req.Providers) == 0 {
		return nil
	}
	rows := make([]providerGeoMetaRow, len(req.Providers))
	lookupCache := make(map[string]providerGeoLookup, len(req.Providers))
	var lookedUpIPs int
	for i := range req.Providers {
		p := req.Providers[i]
		r := providerGeoMetaRow{
			ProviderPubkey:  p.ProviderPubkey,
			ProviderAddress: p.ProviderAddress,
			StorageIP:       p.StorageEndpoint.IP,
		}
		ip := strings.TrimSpace(p.StorageEndpoint.IP)
		if ip == "" {
			r.GeoLookupError = "empty storage IP"
			rows[i] = r
			continue
		}

		lookup, ok := lookupCache[ip]
		if !ok {
			if lookedUpIPs > 0 {
				timer := time.NewTimer(geoLookupDelay)
				select {
				case <-ctx.Done():
					timer.Stop()
					return ctx.Err()
				case <-timer.C:
				}
			}

			timeoutCtx, cancel := context.WithTimeout(ctx, geoLookupTimeout)
			info, err := ipinfo.GetIPInfo(timeoutCtx, ip)
			cancel()

			if err != nil {
				lookup.GeoLookupError = err.Error()
			} else if info != nil {
				lookup.Country = info.Country
				lookup.CountryISO = info.CountryISO
				lookup.City = info.City
			}
			lookupCache[ip] = lookup
			lookedUpIPs++
		}

		r.Country = lookup.Country
		r.CountryISO = lookup.CountryISO
		r.City = lookup.City
		r.GeoLookupError = lookup.GeoLookupError
		rows[i] = r
	}

	out := runChecksMetaFile{
		GeneratedAt: time.Now().UTC().Format(time.RFC3339),
		Source:      "coordinator dump-runchecks: geo by storage IP (ifconfig.co)",
		Providers:   rows,
	}
	body, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal meta json: %w", err)
	}
	body = append(body, '\n')
	if err := os.WriteFile(metaPath, body, 0o644); err != nil {
		return fmt.Errorf("write meta file: %w", err)
	}
	logger.Info("runchecks geo meta written", "path", metaPath, "providers", len(rows))
	return nil
}

func writeRunChecksDump(
	ctx context.Context,
	builder providersmaster.RequestBuilder,
	ipinfo ifconfig.IFConfig,
	logger *slog.Logger,
	outPath, jobID string,
	providerLimit int,
	envelope bool,
	storage string,
	pingMS, rldpMS, totalMS uint,
) error {
	if builder == nil {
		return fmt.Errorf("request builder is nil")
	}

	req, err := builder.BuildRunChecksRequest(ctx, jobID, providerLimit, providersmaster.CheckTimeoutsPayload{
		PingMs:  uint32(pingMS),
		RldpMs:  uint32(rldpMS),
		TotalMs: uint32(totalMS),
	})
	if err != nil {
		return err
	}

	var payload any = req
	if envelope {
		payload = map[string]any{
			"meta": map[string]any{
				"generatedAt": time.Now().UTC().Format(time.RFC3339),
				"source":      "coordinator-dump-runchecks",
				"storage":     storage,
			},
			"request": req,
		}
	}

	body, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal request json: %w", err)
	}
	body = append(body, '\n')

	if outPath == "" {
		_, err = os.Stdout.Write(body)
		return err
	}

	if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
		return fmt.Errorf("create output directory: %w", err)
	}
	if err := os.WriteFile(outPath, body, 0o644); err != nil {
		return fmt.Errorf("write output file: %w", err)
	}

	logger.Info("runchecks request dumped", "out", outPath, "providers", len(req.Providers), "storage", storage)

	if outPath != "" && ipinfo != nil {
		metaPath := metaPathForDump(outPath)
		if err := writeProviderGeoMeta(ctx, ipinfo, req, metaPath, logger); err != nil {
			logger.Warn("geo meta skipped", "error", err)
		}
	}
	return nil
}

func shouldRetryMemoryDump(err error) bool {
	return errors.Is(err, providersmaster.ErrNoStorageContracts) ||
		errors.Is(err, providersmaster.ErrNoResolvedProviderEndpoints) ||
		errors.Is(err, providersmaster.ErrNoValidProvidersForRunChecks)
}

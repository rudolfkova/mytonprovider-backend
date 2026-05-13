package main

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	providersmaster "mytonprovider-coordinator/internal/workers/providersMaster"
)

// storageRatesDumpConfig is non-nil only when --also-dump-storage-rates is set.
type storageRatesDumpConfig struct {
	ExplicitOutPath string
	JobIDOverride   string
	QueryTimeoutMs  uint32
	TotalMs         uint32
	QuerySize       uint64
}

func storageRatesDefaultOutPath(runChecksOut string) string {
	ext := filepath.Ext(runChecksOut)
	base := strings.TrimSuffix(runChecksOut, ext)
	if ext == "" {
		return runChecksOut + "-storage-rates.json"
	}
	return base + "-storage-rates" + ext
}

type runStorageRatesJSON struct {
	JobID           string                   `json:"jobId"`
	ProviderPubkeys []string                 `json:"providerPubkeys"`
	QuerySize       uint64                   `json:"querySize"`
	Timeouts        storageRatesTimeoutsJSON `json:"timeouts"`
}

type storageRatesTimeoutsJSON struct {
	QueryTimeoutMs uint32 `json:"queryTimeoutMs"`
	TotalMs        uint32 `json:"totalMs"`
}

func collectProviderPubkeysForRates(req *providersmaster.RunChecksRequestPayload) []string {
	if req == nil {
		return nil
	}
	out := make([]string, 0, len(req.Providers))
	for i := range req.Providers {
		pk := strings.TrimSpace(req.Providers[i].ProviderPubkey)
		if pk != "" {
			out = append(out, pk)
		}
	}
	return out
}

func writeRunStorageRatesDump(
	req *providersmaster.RunChecksRequestPayload,
	runChecksOut string,
	cfg *storageRatesDumpConfig,
	logger *slog.Logger,
) error {
	if cfg == nil {
		return nil
	}
	pubkeys := collectProviderPubkeysForRates(req)
	if len(pubkeys) == 0 {
		logger.Warn("storage rates dump skipped: no provider pubkeys in RunChecks payload")
		return nil
	}

	outPath := strings.TrimSpace(cfg.ExplicitOutPath)
	if outPath == "" {
		outPath = storageRatesDefaultOutPath(runChecksOut)
	}

	jobID := strings.TrimSpace(cfg.JobIDOverride)
	if jobID == "" && req != nil {
		jobID = req.JobID + "-storage-rates"
	}

	querySize := cfg.QuerySize
	if querySize == 0 {
		querySize = 1
	}

	payload := runStorageRatesJSON{
		JobID:           jobID,
		ProviderPubkeys: pubkeys,
		QuerySize:       querySize,
		Timeouts: storageRatesTimeoutsJSON{
			QueryTimeoutMs: cfg.QueryTimeoutMs,
			TotalMs:        cfg.TotalMs,
		},
	}

	body, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal storage rates request json: %w", err)
	}
	body = append(body, '\n')

	if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
		return fmt.Errorf("create storage rates output directory: %w", err)
	}
	if err := os.WriteFile(outPath, body, 0o644); err != nil {
		return fmt.Errorf("write storage rates output file: %w", err)
	}

	logger.Info("RunStorageRates request dumped", "out", outPath, "pubkeys", len(pubkeys))
	return nil
}

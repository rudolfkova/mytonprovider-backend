package grpcserver

import (
	"context"
	"encoding/hex"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"mytonprovider-agent/internal/metrics"
	providerchecksv1 "mytonprovider-contracts/gen/go/providerchecks/v1"
)

const (
	maxStorageRatesPubkeys = 4096
	defaultRatesQueryMs    = uint32(14_000) // align with coordinator providerResponseTimeout
)

func withStorageRatesTotalTimeout(ctx context.Context, timeouts *providerchecksv1.StorageRatesTimeouts) (context.Context, context.CancelFunc) {
	if timeouts == nil || timeouts.GetTotalMs() == 0 {
		return ctx, func() {}
	}
	d := time.Duration(timeouts.GetTotalMs()) * time.Millisecond
	if deadline, ok := ctx.Deadline(); ok {
		remaining := time.Until(deadline)
		if remaining <= 0 {
			return ctx, func() {}
		}
		if remaining < d {
			d = remaining
		}
	}
	return context.WithTimeout(ctx, d)
}

func validateRunStorageRatesRequest(req *providerchecksv1.RunStorageRatesRequest) error {
	if req == nil {
		return fmt.Errorf("request is nil")
	}
	if strings.TrimSpace(req.GetJobId()) == "" {
		return fmt.Errorf("job_id is empty")
	}
	if len(req.GetProviderPubkeys()) == 0 {
		return fmt.Errorf("provider_pubkeys is empty")
	}
	if len(req.GetProviderPubkeys()) > maxStorageRatesPubkeys {
		return fmt.Errorf("provider_pubkeys exceeds max of %d", maxStorageRatesPubkeys)
	}
	return nil
}

func (s *service) RunStorageRates(ctx context.Context, req *providerchecksv1.RunStorageRatesRequest) (*providerchecksv1.RunStorageRatesResponse, error) {
	if err := validateRunStorageRatesRequest(req); err != nil {
		s.logger.Warn("invalid RunStorageRates request", "error", err)
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	started := time.Now()
	ctx, cancel := withStorageRatesTotalTimeout(ctx, req.GetTimeouts())
	defer cancel()

	log := s.logger.With("job_id", req.GetJobId(), "agent_id", s.agentID, "location", s.location, "rpc", "RunStorageRates")
	log.Debug("start RunStorageRates", "pubkeys", len(req.GetProviderPubkeys()))

	querySize := req.GetQuerySize()
	if querySize == 0 {
		querySize = 1
	}

	queryTimeoutMs := defaultRatesQueryMs
	if t := req.GetTimeouts(); t != nil && t.GetQueryTimeoutMs() > 0 {
		queryTimeoutMs = t.GetQueryTimeoutMs()
	}
	perQuery := time.Duration(queryTimeoutMs) * time.Millisecond

	pubkeys := req.GetProviderPubkeys()
	results := make([]*providerchecksv1.StorageRatesResult, len(pubkeys))

	semN := s.maxConcurrentRates
	if semN <= 0 {
		semN = 1
	}
	sem := make(chan struct{}, semN)
	var wg sync.WaitGroup

	for i, pk := range pubkeys {
		wg.Add(1)
		go func(i int, pk string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			results[i] = s.fetchOneStorageRate(ctx, pk, querySize, perQuery, log)
		}(i, pk)
	}
	wg.Wait()

	resp := &providerchecksv1.RunStorageRatesResponse{
		JobId:          req.GetJobId(),
		AgentId:        s.agentID,
		Location:       s.location,
		StartedAtUnix:  started.Unix(),
		FinishedAtUnix: time.Now().Unix(),
		Results:        results,
	}

	if err := ctx.Err(); err != nil {
		log.Warn("RunStorageRates completed with context error", "error", err)
		resp.Warnings = append(resp.Warnings, &providerchecksv1.ErrorDetail{
			Code:      providerchecksv1.ErrorCode_DEADLINE_EXCEEDED,
			Message:   "processing timeout reached",
			Retryable: true,
		})
	}

	ok, fail := 0, 0
	for _, r := range results {
		if r != nil && r.GetOk() {
			metrics.IncRunStorageRatesRow(true)
			ok++
		} else {
			metrics.IncRunStorageRatesRow(false)
			fail++
		}
	}
	log.Info(
		"RunStorageRates completed",
		"pubkeys_total", len(pubkeys),
		"ok", ok,
		"failed", fail,
		"duration_ms", time.Since(started).Milliseconds(),
	)
	metrics.ObserveRunStorageRatesJob(time.Since(started))
	return resp, nil
}

func (s *service) fetchOneStorageRate(
	ctx context.Context,
	pubkeyHex string,
	querySize uint64,
	perQuery time.Duration,
	log *slog.Logger,
) *providerchecksv1.StorageRatesResult {
	row := &providerchecksv1.StorageRatesResult{ProviderPubkey: pubkeyHex}
	pk := strings.TrimSpace(pubkeyHex)
	if len(pk) != 64 {
		row.Details = fmt.Sprintf("invalid provider_pubkey length: got %d want 64 hex chars", len(pk))
		return row
	}
	key, err := hex.DecodeString(pk)
	if err != nil {
		row.Details = fmt.Sprintf("invalid hex in provider_pubkey: %v", err)
		return row
	}
	if len(key) != 32 {
		row.Details = fmt.Sprintf("decoded pubkey length %d != 32", len(key))
		return row
	}

	t0 := time.Now()
	qctx, qcancel := context.WithTimeout(ctx, perQuery)
	defer qcancel()

	rates, err := s.ratesTransport.Client().GetStorageRates(qctx, key, querySize)
	row.LatencyMs = uint32(time.Since(t0).Milliseconds())
	if err != nil {
		row.Details = err.Error()
		log.Debug("GetStorageRates failed", "provider_pubkey", pk, "error", err)
		return row
	}

	row.Ok = true
	row.Available = rates.Available
	row.RatePerMbDay = append([]byte(nil), rates.RatePerMBDay...)
	row.MinBounty = append([]byte(nil), rates.MinBounty...)
	row.SpaceAvailableMb = rates.SpaceAvailableMB
	row.MinSpan = rates.MinSpan
	row.MaxSpan = rates.MaxSpan
	return row
}

package grpcserver

import (
	"context"
	"encoding/hex"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/xssnick/tonutils-go/address"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"mytonprovider-agent/internal/metrics"
	providerchecksv1 "github.com/rudolfkova/mytonprovider-backend/contracts/gen/go/providerchecks/v1"
)

const (
	maxStorageInfoQueries = 4096
	defaultStorageInfoQueryMs = uint32(10_000)
)

func withStorageInfoTotalTimeout(ctx context.Context, timeouts *providerchecksv1.StorageInfoTimeouts) (context.Context, context.CancelFunc) {
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

func validateRequestStorageInfoRequest(req *providerchecksv1.RequestStorageInfoRequest) error {
	if req == nil {
		return fmt.Errorf("request is nil")
	}
	if strings.TrimSpace(req.GetJobId()) == "" {
		return fmt.Errorf("job_id is empty")
	}
	if len(req.GetQueries()) == 0 {
		return fmt.Errorf("queries is empty")
	}
	if len(req.GetQueries()) > maxStorageInfoQueries {
		return fmt.Errorf("queries exceeds max of %d", maxStorageInfoQueries)
	}
	return nil
}

func (s *service) RequestStorageInfo(ctx context.Context, req *providerchecksv1.RequestStorageInfoRequest) (*providerchecksv1.RequestStorageInfoResponse, error) {
	if err := validateRequestStorageInfoRequest(req); err != nil {
		s.logger.Warn("invalid RequestStorageInfo request", "error", err)
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	started := time.Now()
	ctx, cancel := withStorageInfoTotalTimeout(ctx, req.GetTimeouts())
	defer cancel()

	log := s.logger.With("job_id", req.GetJobId(), "agent_id", s.agentID, "location", s.location, "rpc", "RequestStorageInfo")
	log.Debug("start RequestStorageInfo", "queries", len(req.GetQueries()))

	queryTimeoutMs := defaultStorageInfoQueryMs
	if t := req.GetTimeouts(); t != nil && t.GetQueryTimeoutMs() > 0 {
		queryTimeoutMs = t.GetQueryTimeoutMs()
	}
	perQuery := time.Duration(queryTimeoutMs) * time.Millisecond

	queries := req.GetQueries()
	results := make([]*providerchecksv1.StorageInfoResult, len(queries))

	semN := s.maxConcurrentRates
	if semN <= 0 {
		semN = 1
	}
	sem := make(chan struct{}, semN)
	var wg sync.WaitGroup

	for i, q := range queries {
		wg.Add(1)
		go func(i int, q *providerchecksv1.StorageInfoQuery) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			results[i] = s.fetchOneStorageInfo(ctx, q, perQuery, log)
		}(i, q)
	}
	wg.Wait()

	resp := &providerchecksv1.RequestStorageInfoResponse{
		JobId:          req.GetJobId(),
		AgentId:        s.agentID,
		Location:       s.location,
		StartedAtUnix:  started.Unix(),
		FinishedAtUnix: time.Now().Unix(),
		Results:        results,
	}

	if err := ctx.Err(); err != nil {
		log.Warn("RequestStorageInfo completed with context error", "error", err)
		resp.Warnings = append(resp.Warnings, &providerchecksv1.ErrorDetail{
			Code:      providerchecksv1.ErrorCode_DEADLINE_EXCEEDED,
			Message:   "processing timeout reached",
			Retryable: true,
		})
	}

	ok, fail := 0, 0
	for _, r := range results {
		if r != nil && r.GetOk() {
			metrics.IncRequestStorageInfoRow(true)
			ok++
		} else {
			metrics.IncRequestStorageInfoRow(false)
			fail++
		}
	}
	log.Info(
		"RequestStorageInfo completed",
		"queries_total", len(queries),
		"ok", ok,
		"failed", fail,
		"duration_ms", time.Since(started).Milliseconds(),
	)
	metrics.ObserveRequestStorageInfoJob(time.Since(started))
	return resp, nil
}

func (s *service) fetchOneStorageInfo(
	ctx context.Context,
	q *providerchecksv1.StorageInfoQuery,
	perQuery time.Duration,
	log *slog.Logger,
) *providerchecksv1.StorageInfoResult {
	row := &providerchecksv1.StorageInfoResult{
		ProviderPubkey:  strings.TrimSpace(q.GetProviderPubkey()),
		ContractAddress: strings.TrimSpace(q.GetContractAddress()),
	}
	pk := row.ProviderPubkey
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
	if row.ContractAddress == "" {
		row.Details = "contract_address is empty"
		return row
	}

	contractAddr, err := address.ParseAddr(row.ContractAddress)
	if err != nil {
		row.Details = fmt.Sprintf("invalid contract_address: %v", err)
		return row
	}

	t0 := time.Now()
	qctx, qcancel := context.WithTimeout(ctx, perQuery)
	defer qcancel()

	info, err := s.ratesTransport.Client().RequestStorageInfo(qctx, key, contractAddr, q.GetByteToProof())
	row.LatencyMs = uint32(time.Since(t0).Milliseconds())
	if err != nil {
		row.Details = err.Error()
		log.Debug("RequestStorageInfo failed",
			"provider_pubkey", pk,
			"contract_address", row.ContractAddress,
			"error", err)
		return row
	}

	row.Ok = true
	row.Status = info.Status
	row.Reason = info.Reason
	row.Downloaded = info.Downloaded
	row.Proof = append([]byte(nil), info.Proof...)
	return row
}

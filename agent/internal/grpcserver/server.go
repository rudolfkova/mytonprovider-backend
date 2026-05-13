package grpcserver

import (
	"context"
	"crypto/ed25519"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/status"

	"mytonprovider-agent/internal/checker"
	"mytonprovider-agent/internal/config"
	"mytonprovider-agent/internal/tontransport"
	providerchecksv1 "mytonprovider-contracts/gen/go/providerchecks/v1"
)

type service struct {
	providerchecksv1.UnimplementedProviderChecksServiceServer

	checker            *checker.Checker
	ratesTransport     *tontransport.ProviderTransport
	maxConcurrentRates int
	agentID            string
	location           string
	logger             *slog.Logger
}

func New(cfg config.Config, logger *slog.Logger) (*grpc.Server, func(), error) {
	creds, err := credentials.NewServerTLSFromFile(cfg.TLSCertFile, cfg.TLSKeyFile)
	if err != nil {
		return nil, nil, fmt.Errorf("create TLS credentials: %w", err)
	}

	checks, err := checker.New(cfg.MaxConcurrentProviders, logger)
	if err != nil {
		return nil, nil, fmt.Errorf("initialize checker: %w", err)
	}

	ratesTransport := tontransport.NewProviderTransport(cfg.TonConfigURL, cfg.AdnlPort, cfg.AdnlPrivateKey)
	startCtx, cancelStart := context.WithTimeout(context.Background(), 2*time.Minute)
	if err := ratesTransport.Start(startCtx); err != nil {
		cancelStart()
		_ = ratesTransport.Close()
		return nil, nil, fmt.Errorf("start ton transport: %w", err)
	}
	cancelStart()

	maxCR := cfg.MaxConcurrentRates
	if maxCR <= 0 {
		maxCR = cfg.MaxConcurrentProviders
	}

	grpcServer := grpc.NewServer(
		grpc.Creds(creds),
		grpc.UnaryInterceptor(authInterceptor(cfg.AuthToken)),
	)

	providerchecksv1.RegisterProviderChecksServiceServer(grpcServer, &service{
		checker:            checks,
		ratesTransport:     ratesTransport,
		maxConcurrentRates: maxCR,
		agentID:            cfg.AgentID,
		location:           cfg.Location,
		logger:             logger,
	})

	cleanup := func() {
		if err := ratesTransport.Close(); err != nil {
			logger.Warn("ton transport close", "error", err)
		}
	}

	return grpcServer, cleanup, nil
}

func (s *service) RunChecks(ctx context.Context, req *providerchecksv1.RunChecksRequest) (*providerchecksv1.RunChecksResponse, error) {
	if err := validateRequest(req); err != nil {
		s.logger.Warn("invalid RunChecks request", "error", err)
		return nil, status.Error(codes.InvalidArgument, "invalid request payload")
	}

	started := time.Now()
	ctx, cancel := withTotalTimeout(ctx, req.GetTimeouts())
	defer cancel()

	log := s.logger.With("job_id", req.GetJobId(), "agent_id", s.agentID, "location", s.location)
	log.Debug("start processing RunChecks", "providers", len(req.GetProviders()))

	results := s.checker.Run(ctx, req.GetProviders(), req.GetTimeouts(), log)

	resp := &providerchecksv1.RunChecksResponse{
		JobId:          req.GetJobId(),
		AgentId:        s.agentID,
		Location:       s.location,
		StartedAtUnix:  started.Unix(),
		FinishedAtUnix: time.Now().Unix(),
		Results:        results,
	}

	if err := ctx.Err(); err != nil {
		log.Warn("RunChecks completed with context error", "error", err)
		resp.Warnings = append(resp.Warnings, &providerchecksv1.ErrorDetail{
			Code:      providerchecksv1.ErrorCode_DEADLINE_EXCEEDED,
			Message:   "processing timeout reached",
			Retryable: true,
		})
	}

	contractsTotal := 0
	for _, p := range req.GetProviders() {
		if p != nil {
			contractsTotal += len(p.GetContracts())
		}
	}

	reasonCounts := make(map[string]int)
	errorSignatures := make(map[string]int)
	for _, r := range resp.Results {
		if r == nil {
			continue
		}
		reason := r.GetReasonCode().String()
		reasonCounts[reason]++
		if r.GetReasonCode() != providerchecksv1.ReasonCode_VALID_STORAGE_PROOF {
			d := r.GetDetails()
			if d == "" {
				d = "no_details"
			}
			errorSignatures[fmt.Sprintf("%s | %s", reason, d)]++
		}
	}

	log.Info(
		"RunChecks completed",
		"providers_total", len(req.GetProviders()),
		"contracts_total", contractsTotal,
		"results_total", len(resp.Results),
		"duration_ms", time.Since(started).Milliseconds(),
		"reason_counts", reasonCounts,
		"error_signatures", errorSignatures,
	)
	return resp, nil
}

func validateRequest(req *providerchecksv1.RunChecksRequest) error {
	if req == nil {
		return fmt.Errorf("request is nil")
	}
	if strings.TrimSpace(req.GetJobId()) == "" {
		return fmt.Errorf("job_id is empty")
	}
	if len(req.GetProviders()) == 0 {
		return fmt.Errorf("providers list is empty")
	}

	for i, provider := range req.GetProviders() {
		if provider == nil {
			return fmt.Errorf("provider at index %d is nil", i)
		}
		if strings.TrimSpace(provider.GetProviderPubkey()) == "" {
			return fmt.Errorf("provider_pubkey is empty")
		}
		if strings.TrimSpace(provider.GetProviderAddress()) == "" {
			return fmt.Errorf("provider_address is empty")
		}
		if len(provider.GetContracts()) == 0 {
			return fmt.Errorf("contracts are empty for provider %s", provider.GetProviderPubkey())
		}

		endpoint := provider.GetStorageEndpoint()
		if endpoint == nil {
			return fmt.Errorf("storage_endpoint is nil for provider %s", provider.GetProviderPubkey())
		}
		if strings.TrimSpace(endpoint.GetIp()) == "" {
			return fmt.Errorf("storage endpoint ip is empty for provider %s", provider.GetProviderPubkey())
		}
		if endpoint.GetPort() <= 0 {
			return fmt.Errorf("storage endpoint port is invalid for provider %s", provider.GetProviderPubkey())
		}
		if len(endpoint.GetAdnlPubkey()) != ed25519.PublicKeySize {
			return fmt.Errorf("storage endpoint adnl_pubkey has invalid size for provider %s", provider.GetProviderPubkey())
		}

		for j, contract := range provider.GetContracts() {
			if contract == nil {
				return fmt.Errorf("contract at index %d is nil for provider %s", j, provider.GetProviderPubkey())
			}
			if strings.TrimSpace(contract.GetContractAddress()) == "" {
				return fmt.Errorf("contract_address is empty for provider %s", provider.GetProviderPubkey())
			}
			if strings.TrimSpace(contract.GetBagId()) == "" {
				return fmt.Errorf("bag_id is empty for provider %s", provider.GetProviderPubkey())
			}
		}
	}

	return nil
}

func withTotalTimeout(ctx context.Context, timeouts *providerchecksv1.CheckTimeouts) (context.Context, context.CancelFunc) {
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

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

	providerchecksv1 "github.com/grach/mytonprovider-contracts/gen/go/providerchecks/v1"
	"mytonprovider-agent/internal/checker"
	"mytonprovider-agent/internal/config"
)

type service struct {
	providerchecksv1.UnimplementedProviderChecksServiceServer

	checker  *checker.Checker
	agentID  string
	location string
	logger   *slog.Logger
}

func New(cfg config.Config, logger *slog.Logger) (*grpc.Server, error) {
	creds, err := credentials.NewServerTLSFromFile(cfg.TLSCertFile, cfg.TLSKeyFile)
	if err != nil {
		return nil, fmt.Errorf("create TLS credentials: %w", err)
	}

	checks, err := checker.New(cfg.MaxConcurrentProviders, logger)
	if err != nil {
		return nil, fmt.Errorf("initialize checker: %w", err)
	}

	grpcServer := grpc.NewServer(
		grpc.Creds(creds),
		grpc.UnaryInterceptor(authInterceptor(cfg.AuthToken)),
	)

	providerchecksv1.RegisterProviderChecksServiceServer(grpcServer, &service{
		checker:  checks,
		agentID:  cfg.AgentID,
		location: cfg.Location,
		logger:   logger,
	})

	return grpcServer, nil
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

	results := s.checker.Run(ctx, req.GetProviders(), req.GetTimeouts())

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

	log.Info("RunChecks completed", "results", len(resp.Results))
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

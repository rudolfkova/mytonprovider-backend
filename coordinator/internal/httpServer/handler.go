package httpServer

import (
	"context"
	"crypto/md5"
	"fmt"
	"log/slog"
	"strings"

	"github.com/gofiber/fiber/v2"

	v1 "mytonprovider-coordinator/internal/models/api/v1"
)

type providers interface {
	SearchProviders(ctx context.Context, req v1.SearchProvidersRequest) ([]v1.Provider, error)
	GetLatestTelemetry(ctx context.Context) (providers []interface{}, err error)
	GetFiltersRange(ctx context.Context) (filtersRange v1.FiltersRangeResp, err error)
	UpdateTelemetry(ctx context.Context, telemetry v1.TelemetryRequest, rawBody []byte) (err error)
	UpdateBenchmarks(ctx context.Context, benchmark v1.BenchmarksRequest) (err error)
	GetStorageContractsChecks(ctx context.Context, req v1.ContractsStatusesRequest) ([]v1.ContractCheck, error)
	GetContractBags(ctx context.Context, req v1.ContractBagsRequest) ([]v1.ContractBag, int, error)
}

type errorResponse struct {
	Error string `json:"error"`
}

type handler struct {
	server            *fiber.App
	logger            *slog.Logger
	providers         providers
	namespace         string
	subsystem         string
	accessTokens      map[string]struct{}
	corsAllowedOrigins []string
}

func New(
	server *fiber.App,
	providers providers,
	accessTokens []string,
	corsAllowedOrigins []string,
	namespace string,
	subsystem string,
	logger *slog.Logger,
) *handler {
	accessTokensMap := make(map[string]struct{})
	for _, token := range accessTokens {
		token = strings.TrimSpace(token)
		if token == "" {
			continue
		}
		sum := md5.Sum([]byte(token))
		accessTokensMap[fmt.Sprintf("%x", sum[:])] = struct{}{}
	}

	h := &handler{
		server:             server,
		providers:          providers,
		namespace:          namespace,
		subsystem:          subsystem,
		accessTokens:       accessTokensMap,
		corsAllowedOrigins: corsAllowedOrigins,
		logger:             logger,
	}

	return h
}

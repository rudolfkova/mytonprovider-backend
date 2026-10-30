package providers

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"

	"mytonprovider-coordinator/internal/models"
	v1 "mytonprovider-coordinator/internal/models/api/v1"
	"mytonprovider-coordinator/internal/models/db"
)

type fakeProvidersRepo struct {
	ip    string
	ipErr error
}

func (f *fakeProvidersRepo) GetProvidersByPubkeys(context.Context, []string) ([]db.ProviderDB, error) {
	return nil, nil
}

func (f *fakeProvidersRepo) GetProviderIP(context.Context, string) (string, error) {
	return f.ip, f.ipErr
}

func (f *fakeProvidersRepo) GetFiltersRange(context.Context) (db.FiltersRange, error) {
	return db.FiltersRange{}, nil
}

func (f *fakeProvidersRepo) GetFilteredProviders(context.Context, db.ProviderFilters, db.ProviderSort, int, int) ([]db.ProviderDB, error) {
	return nil, nil
}

func (f *fakeProvidersRepo) GetStorageContractsChecks(context.Context, []string) ([]db.ContractCheck, error) {
	return nil, nil
}

func (f *fakeProvidersRepo) GetContractBags(context.Context, string, int, int) ([]db.ContractBag, int, error) {
	return nil, 0, nil
}

func newTestService(repo providers) *service {
	return &service{
		providers: repo,
		logger:    slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
}

func TestUpdateTelemetrySourceIP(t *testing.T) {
	t.Parallel()

	req := v1.TelemetryRequest{
		Storage: v1.StorageInfo{
			Provider: v1.ProviderInfo{PubKey: "abc"},
		},
		XRealIP: "1.2.3.4",
	}
	body := []byte(`{"storage":{"provider":{"pubkey":"abc"}}}`)

	t.Run("accepts matching ip", func(t *testing.T) {
		t.Parallel()
		svc := newTestService(&fakeProvidersRepo{ip: "1.2.3.4"})
		err := svc.UpdateTelemetry(context.Background(), req, body)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("rejects mismatch", func(t *testing.T) {
		t.Parallel()
		svc := newTestService(&fakeProvidersRepo{ip: "8.8.8.8"})
		err := svc.UpdateTelemetry(context.Background(), req, body)
		assertForbidden(t, err)
	})

	t.Run("rejects missing provider ip", func(t *testing.T) {
		t.Parallel()
		svc := newTestService(&fakeProvidersRepo{})
		err := svc.UpdateTelemetry(context.Background(), req, body)
		assertForbidden(t, err)
	})

	t.Run("rejects lookup error as internal", func(t *testing.T) {
		t.Parallel()
		svc := newTestService(&fakeProvidersRepo{ipErr: errors.New("db down")})
		err := svc.UpdateTelemetry(context.Background(), req, body)
		assertAppErrorCode(t, err, models.InternalServerErrorCode)
	})
}

func TestUpdateBenchmarksSourceIP(t *testing.T) {
	t.Parallel()

	req := v1.BenchmarksRequest{
		PubKey:  "abc",
		XRealIP: "1.2.3.4",
	}

	t.Run("accepts matching ip", func(t *testing.T) {
		t.Parallel()
		svc := newTestService(&fakeProvidersRepo{ip: "1.2.3.4"})
		err := svc.UpdateBenchmarks(context.Background(), req)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("rejects mismatch", func(t *testing.T) {
		t.Parallel()
		svc := newTestService(&fakeProvidersRepo{ip: "8.8.8.8"})
		err := svc.UpdateBenchmarks(context.Background(), req)
		assertForbidden(t, err)
	})
}

func assertForbidden(t *testing.T, err error) {
	t.Helper()
	assertAppErrorCode(t, err, models.ForbiddenErrorCode)
}

func assertAppErrorCode(t *testing.T, err error, code int) {
	t.Helper()
	if err == nil {
		t.Fatal("expected error")
	}
	appErr, ok := err.(*models.AppError)
	if !ok {
		t.Fatalf("expected AppError, got %T %v", err, err)
	}
	if appErr.Code != code {
		t.Fatalf("error code = %d, want %d", appErr.Code, code)
	}
}

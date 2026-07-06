package providersmaster

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	tonclient "mytonprovider-coordinator/internal/clients/ton"
	"mytonprovider-coordinator/internal/models/db"
)

type fakeProvidersRepo struct {
	closed []db.ContractToProviderRelation
}

func (f *fakeProvidersRepo) GetAllProvidersPubkeys(context.Context) ([]string, error) {
	return nil, nil
}

func (f *fakeProvidersRepo) GetAllProvidersWallets(context.Context) ([]db.ProviderWallet, error) {
	return nil, nil
}

func (f *fakeProvidersRepo) UpdateProvidersLT(context.Context, []db.ProviderWalletLT) error {
	return nil
}

func (f *fakeProvidersRepo) AddStorageContracts(context.Context, []db.StorageContract) error {
	return nil
}

func (f *fakeProvidersRepo) GetStorageContracts(context.Context) ([]db.ContractToProviderRelation, error) {
	return nil, nil
}

func (f *fakeProvidersRepo) UpdateRejectedStorageContracts(_ context.Context, rels []db.ContractToProviderRelation) error {
	f.closed = append([]db.ContractToProviderRelation(nil), rels...)
	return nil
}

func (f *fakeProvidersRepo) AddProviders(context.Context, []db.ProviderCreate) error {
	return nil
}

func (f *fakeProvidersRepo) UpdateProvidersIPs(context.Context, []db.ProviderIP) error {
	return nil
}

func (f *fakeProvidersRepo) UpdateProviders(context.Context, []db.ProviderUpdate) error {
	return nil
}

func (f *fakeProvidersRepo) AddStatuses(context.Context, []db.ProviderStatusUpdate) error {
	return nil
}

func (f *fakeProvidersRepo) UpdateContractProofsChecks(context.Context, []db.ContractProofsCheck) error {
	return nil
}

func (f *fakeProvidersRepo) UpdateStatuses(context.Context) error {
	return nil
}

func (f *fakeProvidersRepo) UpdateUptime(context.Context) error {
	return nil
}

func (f *fakeProvidersRepo) UpdateRating(context.Context) error {
	return nil
}

func (f *fakeProvidersRepo) GetProvidersIPs(context.Context) ([]db.ProviderIP, error) {
	return nil, nil
}

func (f *fakeProvidersRepo) GetProvidersEndpointState(context.Context, []string) ([]db.ProviderEndpointState, error) {
	return nil, nil
}

func (f *fakeProvidersRepo) UpdateProvidersIPInfo(context.Context, []db.ProviderIPInfo) error {
	return nil
}

func (f *fakeProvidersRepo) InsertProviderPipelineEvents(context.Context, []db.ProviderPipelineEvent) error {
	return nil
}

func (f *fakeProvidersRepo) InsertBagPipelineEvents(context.Context, []db.BagPipelineEvent) error {
	return nil
}

func (f *fakeProvidersRepo) GetLastProviderPipelineEventStatus(context.Context, []string) (map[string]db.PipelineEventStatus, error) {
	return map[string]db.PipelineEventStatus{}, nil
}

type fakeSystemRepo struct{}

func (f *fakeSystemRepo) SetParam(context.Context, string, string) error {
	return nil
}

func (f *fakeSystemRepo) GetParam(context.Context, string) (string, error) {
	return "0", nil
}

type fakeTonClient struct {
	contractsProviders []tonclient.StorageContractProviders
}

func (f *fakeTonClient) GetTransactions(context.Context, string, uint64) ([]*tonclient.Transaction, error) {
	return nil, nil
}

func (f *fakeTonClient) GetStorageContractsInfo(context.Context, []string) ([]tonclient.StorageContract, error) {
	return nil, nil
}

func (f *fakeTonClient) GetProvidersInfo(context.Context, []string) ([]tonclient.StorageContractProviders, error) {
	return f.contractsProviders, nil
}

func TestUpdateRejectedContracts_KeepActiveOnLiteServerError(t *testing.T) {
	t.Parallel()

	repo := &fakeProvidersRepo{}
	w := &providersMasterWorker{
		providers: repo,
		system:    &fakeSystemRepo{},
		ton: &fakeTonClient{
			contractsProviders: []tonclient.StorageContractProviders{
				{
					Address:         "contract-1",
					Balance:         0,
					Providers:       nil,
					LiteServerError: true,
				},
			},
		},
		logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	}

	input := []db.ContractToProviderRelation{
		{
			ProviderPublicKey: "provider-pk-1",
			ProviderAddress:   "provider-addr-1",
			Address:           "contract-1",
			BagID:             "bag-1",
			Size:              1024,
		},
	}

	active, err := w.updateRejectedContracts(context.Background(), input)
	if err != nil {
		t.Fatalf("updateRejectedContracts() unexpected error: %v", err)
	}
	if len(active) != 1 {
		t.Fatalf("expected 1 active contract on LiteServerError, got %d", len(active))
	}
	if len(repo.closed) != 0 {
		t.Fatalf("expected no closed contracts on LiteServerError, got %d", len(repo.closed))
	}
}

func TestUpdateRejectedContracts_ClosesMissingProviderRelation(t *testing.T) {
	t.Parallel()

	repo := &fakeProvidersRepo{}
	w := &providersMasterWorker{
		providers: repo,
		system:    &fakeSystemRepo{},
		ton: &fakeTonClient{
			contractsProviders: []tonclient.StorageContractProviders{
				{
					Address: "contract-2",
					Balance: 1_000_000_000,
					Providers: []tonclient.Provider{
						{
							Key:           "another-provider-key",
							LastProofTime: time.Now(),
							RatePerMBDay:  1,
							MaxSpan:       1,
						},
					},
					LiteServerError: false,
				},
			},
		},
		logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	}

	input := []db.ContractToProviderRelation{
		{
			ProviderPublicKey: "provider-pk-2",
			ProviderAddress:   "provider-addr-2",
			Address:           "contract-2",
			BagID:             "bag-2",
			Size:              1024,
		},
	}

	active, err := w.updateRejectedContracts(context.Background(), input)
	if err != nil {
		t.Fatalf("updateRejectedContracts() unexpected error: %v", err)
	}
	if len(active) != 0 {
		t.Fatalf("expected 0 active contracts, got %d", len(active))
	}
	if len(repo.closed) != 1 {
		t.Fatalf("expected 1 closed contract, got %d", len(repo.closed))
	}
}


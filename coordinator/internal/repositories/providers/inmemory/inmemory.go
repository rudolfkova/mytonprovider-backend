package inmemory

import (
	"context"
	"strings"
	"sync"

	"mytonprovider-coordinator/internal/models/db"
)

// Repository is a minimal in-memory stand-in for providers.Repository, sufficient for
// providersMaster CollectNewProviders / CollectProvidersNewStorageContracts and dump-runchecks.
type Repository struct {
	mu sync.RWMutex

	// public_key (lowercase) -> row
	providers map[string]*providerRow
	// (contract_address, provider_address) -> row
	storage map[storageKey]*storageRow
}

type providerRow struct {
	publicKey string
	address   string
	lastLT    uint64
}

type storageKey struct {
	contractAddress string
	providerAddress string
}

type storageRow struct {
	address         string
	providerAddress string
	bagID           string
	ownerAddr       string
	size            uint64
	chunkSize       uint64
	lastLT          uint64
}

func NewRepository() *Repository {
	return &Repository{
		providers: make(map[string]*providerRow),
		storage:   make(map[storageKey]*storageRow),
	}
}

func normPub(s string) string {
	return strings.ToLower(strings.TrimSpace(s))
}

func (r *Repository) GetAllProvidersPubkeys(_ context.Context) ([]string, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]string, 0, len(r.providers))
	for _, row := range r.providers {
		out = append(out, row.publicKey)
	}
	return out, nil
}

func (r *Repository) GetAllProvidersWallets(_ context.Context) ([]db.ProviderWallet, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]db.ProviderWallet, 0, len(r.providers))
	for _, row := range r.providers {
		out = append(out, db.ProviderWallet{
			PubKey:  row.publicKey,
			Address: row.address,
			LT:      row.lastLT,
		})
	}
	return out, nil
}

func (r *Repository) UpdateProvidersLT(_ context.Context, updates []db.ProviderWalletLT) error {
	if len(updates) == 0 {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, u := range updates {
		pk := normPub(u.PubKey)
		if row, ok := r.providers[pk]; ok {
			row.lastLT = u.LT
		}
	}
	return nil
}

func (r *Repository) AddProviders(_ context.Context, providers []db.ProviderCreate) error {
	if len(providers) == 0 {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, p := range providers {
		pk := normPub(p.Pubkey)
		if pk == "" {
			continue
		}
		if _, exists := r.providers[pk]; exists {
			continue
		}
		r.providers[pk] = &providerRow{
			publicKey: pk,
			address:   strings.TrimSpace(p.Address),
			lastLT:    0,
		}
	}
	return nil
}

func (r *Repository) AddStorageContracts(_ context.Context, contracts []db.StorageContract) error {
	if len(contracts) == 0 {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, c := range contracts {
		for provAddr := range c.ProvidersAddresses {
			provAddr = strings.TrimSpace(provAddr)
			if provAddr == "" {
				continue
			}
			k := storageKey{
				contractAddress: strings.TrimSpace(c.Address),
				providerAddress: provAddr,
			}
			r.storage[k] = &storageRow{
				address:         k.contractAddress,
				providerAddress: provAddr,
				bagID:           strings.TrimSpace(c.BagID),
				ownerAddr:       strings.TrimSpace(c.OwnerAddr),
				size:            c.Size,
				chunkSize:       c.ChunkSize,
				lastLT:          c.LastLT,
			}
		}
	}
	return nil
}

func (r *Repository) GetStorageContracts(_ context.Context) ([]db.ContractToProviderRelation, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]db.ContractToProviderRelation, 0, len(r.storage))
	for _, row := range r.storage {
		var pub string
		for pk, prow := range r.providers {
			if prow.address == row.providerAddress {
				pub = pk
				break
			}
		}
		if pub == "" {
			continue
		}
		out = append(out, db.ContractToProviderRelation{
			ProviderPublicKey: pub,
			ProviderAddress:   row.providerAddress,
			Address:           row.address,
			BagID:             row.bagID,
			Size:              row.size,
		})
	}
	return out, nil
}

func (r *Repository) UpdateRejectedStorageContracts(_ context.Context, rels []db.ContractToProviderRelation) error {
	if len(rels) == 0 {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, rel := range rels {
		k := storageKey{
			contractAddress: strings.TrimSpace(rel.Address),
			providerAddress: strings.TrimSpace(rel.ProviderAddress),
		}
		delete(r.storage, k)
	}
	return nil
}

func (r *Repository) UpdateProvidersIPs(_ context.Context, _ []db.ProviderIP) error {
	return nil
}

func (r *Repository) UpdateProviders(_ context.Context, _ []db.ProviderUpdate) error {
	return nil
}

func (r *Repository) AddStatuses(_ context.Context, _ []db.ProviderStatusUpdate) error {
	return nil
}

func (r *Repository) UpdateContractProofsChecks(_ context.Context, _ []db.ContractProofsCheck) error {
	return nil
}

func (r *Repository) UpdateStatuses(_ context.Context) error {
	return nil
}

func (r *Repository) UpdateUptime(_ context.Context) error {
	return nil
}

func (r *Repository) UpdateRating(_ context.Context) error {
	return nil
}

func (r *Repository) GetProvidersIPs(_ context.Context) ([]db.ProviderIP, error) {
	return nil, nil
}

func (r *Repository) UpdateProvidersIPInfo(_ context.Context, _ []db.ProviderIPInfo) error {
	return nil
}

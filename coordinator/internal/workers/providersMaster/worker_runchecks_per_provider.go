package providersmaster

import (
	"context"
	"crypto/ed25519"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	providerchecksv1 "github.com/rudolfkova/mytonprovider-backend/contracts/gen/go/providerchecks/v1"
	"mytonprovider-coordinator/internal/clients/agentrpc"
	"mytonprovider-coordinator/internal/constants"
	"mytonprovider-coordinator/internal/models/db"
	"mytonprovider-coordinator/internal/pipelineevents"
)

const (
	defaultRunChecksMaxConcurrentProviders = 40
	defaultRunChecksMsPerBag               = 500
	defaultRunChecksMinTotalMs             = 3000
	defaultRunChecksRPCSlackMs             = 5000
)

// RunChecksPerProviderConfig toggles per-provider RunChecks RPCs and related limits.
type RunChecksPerProviderConfig struct {
	Enabled                bool
	MaxConcurrentProviders int
	MsPerBag               uint32
	MinTotalMs             uint32
	RpcSlackMs             uint32
	AgentRPCRetries        int
}

func normalizeRunChecksPerProviderConfig(cfg RunChecksPerProviderConfig) RunChecksPerProviderConfig {
	if cfg.MaxConcurrentProviders <= 0 {
		cfg.MaxConcurrentProviders = defaultRunChecksMaxConcurrentProviders
	}
	if cfg.MsPerBag == 0 {
		cfg.MsPerBag = defaultRunChecksMsPerBag
	}
	if cfg.MinTotalMs == 0 {
		cfg.MinTotalMs = defaultRunChecksMinTotalMs
	}
	if cfg.RpcSlackMs == 0 {
		cfg.RpcSlackMs = defaultRunChecksRPCSlackMs
	}
	if cfg.AgentRPCRetries < 0 {
		cfg.AgentRPCRetries = 0
	}
	return cfg
}

func providerTotalTimeoutMs(bagCount int, msPerBag, minTotalMs uint32) uint32 {
	if bagCount <= 0 {
		return minTotalMs
	}
	total := uint64(bagCount) * uint64(msPerBag)
	if total < uint64(minTotalMs) {
		return minTotalMs
	}
	if total > uint64(^uint32(0)) {
		return ^uint32(0)
	}
	return uint32(total)
}

func providerJobIDSuffix(pubkey string) string {
	if len(pubkey) <= 8 {
		return pubkey
	}
	return pubkey[:8]
}

type providerBatchBuildResult struct {
	batches           []*providerchecksv1.ProviderBatch
	checkedKeys       map[string]struct{}
	contractsByPubkey map[string][]db.ContractToProviderRelation
}

func (w *providersMasterWorker) buildProviderBatches(
	storageContracts []db.ContractToProviderRelation,
	availableProvidersIPs map[string]db.ProviderIP,
) providerBatchBuildResult {
	providersContracts := make(map[string][]db.ContractToProviderRelation)
	for _, sc := range storageContracts {
		providersContracts[sc.ProviderPublicKey] = append(providersContracts[sc.ProviderPublicKey], sc)
	}

	checkedKeys := make(map[string]struct{})
	providers := make([]*providerchecksv1.ProviderBatch, 0, len(providersContracts))
	for pubkey, contracts := range providersContracts {
		ip, ok := availableProvidersIPs[pubkey]
		if !ok || strings.TrimSpace(ip.Storage.IP) == "" || ip.Storage.Port <= 0 || len(ip.Storage.PublicKey) != ed25519.PublicKeySize {
			continue
		}
		contractRefs := make([]*providerchecksv1.ContractRef, 0, len(contracts))
		for _, c := range contracts {
			checkedKeys[contractRelationKey(c)] = struct{}{}
			contractRefs = append(contractRefs, &providerchecksv1.ContractRef{
				ContractAddress: c.Address,
				BagId:           c.BagID,
			})
		}
		providers = append(providers, &providerchecksv1.ProviderBatch{
			ProviderPubkey:  pubkey,
			ProviderAddress: contracts[0].ProviderAddress,
			StorageEndpoint: &providerchecksv1.Endpoint{
				Ip:         ip.Storage.IP,
				Port:       ip.Storage.Port,
				AdnlPubkey: append([]byte(nil), ip.Storage.PublicKey...),
			},
			Contracts: contractRefs,
		})
	}

	return providerBatchBuildResult{
		batches:           providers,
		checkedKeys:       checkedKeys,
		contractsByPubkey: providersContracts,
	}
}

func (w *providersMasterWorker) buildRunChecksRequestForProvider(
	storeProofRunID string,
	batch *providerchecksv1.ProviderBatch,
	totalMs uint32,
) *providerchecksv1.RunChecksRequest {
	return &providerchecksv1.RunChecksRequest{
		JobId:     fmt.Sprintf("%s-%s", storeProofRunID, providerJobIDSuffix(batch.GetProviderPubkey())),
		Providers: []*providerchecksv1.ProviderBatch{batch},
		Timeouts: &providerchecksv1.CheckTimeouts{
			PingMs:  w.timeouts.PingMs,
			RldpMs:  w.timeouts.RldpMs,
			TotalMs: totalMs,
		},
	}
}

func agentUnavailableResults(contracts []db.ContractToProviderRelation, agentCount int) map[string]mergedContractCheck {
	details := fmt.Sprintf("stage=%s agents_failed=%d", pipelineevents.StageAgentRunChecksUnavailable, agentCount)
	out := make(map[string]mergedContractCheck, len(contracts))
	for _, sc := range contracts {
		key := contractRelationKey(sc)
		out[key] = mergedContractCheck{
			ContractProofsCheck: db.ContractProofsCheck{
				ContractAddress: sc.Address,
				ProviderAddress: sc.ProviderAddress,
				Reason:          constants.NotFound,
			},
			Details: details,
			Stage:   pipelineevents.StageAgentRunChecksUnavailable,
		}
	}
	return out
}

func (w *providersMasterWorker) runChecksForProviderWithRetry(
	ctx context.Context,
	req *providerchecksv1.RunChecksRequest,
	contracts []db.ContractToProviderRelation,
	grpcTimeout time.Duration,
) (map[string]mergedContractCheck, bool, []agentrpc.AgentCallError) {
	attempts := w.runChecksPerProvider.AgentRPCRetries + 1
	var lastErrs []agentrpc.AgentCallError

	for attempt := 0; attempt < attempts; attempt++ {
		responses, callErrs := w.agentClient.RunChecksAllWithTimeout(ctx, req, grpcTimeout)
		lastErrs = callErrs
		if len(responses) > 0 {
			merged, _ := mergeRunChecksResponses(contracts, responses)
			return merged, false, callErrs
		}
	}

	return agentUnavailableResults(contracts, w.agentClient.AgentCount()), true, lastErrs
}

func (w *providersMasterWorker) updateActiveContractsPerProvider(
	ctx context.Context,
	storeProofRunID string,
	storageContracts []db.ContractToProviderRelation,
	availableProvidersIPs map[string]db.ProviderIP,
	recorder *pipelineevents.Recorder,
) error {
	log := w.logger.With(slog.String("worker", "StoreProof"), slog.String("function", "updateActiveContractsPerProvider"))

	build := w.buildProviderBatches(storageContracts, availableProvidersIPs)
	if len(build.batches) == 0 {
		if recorder != nil {
			for _, sc := range storageContracts {
				if _, ok := build.checkedKeys[contractRelationKey(sc)]; !ok {
					recorder.RecordBagEndpointUnresolved(sc)
				}
			}
		}
		return fmt.Errorf("no providers with resolved endpoints for RunChecks")
	}

	cfg := w.runChecksPerProvider
	merged := make(map[string]mergedContractCheck, len(build.checkedKeys))
	var mergedMu sync.Mutex

	var providersRPCFailed int
	var validTotal int

	sem := make(chan struct{}, cfg.MaxConcurrentProviders)
	var wg sync.WaitGroup

	for _, batch := range build.batches {
		if batch == nil {
			continue
		}
		batch := batch
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			contracts := build.contractsByPubkey[batch.GetProviderPubkey()]
			bagCount := len(batch.GetContracts())
			totalMs := providerTotalTimeoutMs(bagCount, cfg.MsPerBag, cfg.MinTotalMs)
			grpcTimeout := time.Duration(totalMs+cfg.RpcSlackMs) * time.Millisecond

			req := w.buildRunChecksRequestForProvider(storeProofRunID, batch, totalMs)
			providerMerged, rpcFailed, callErrs := w.runChecksForProviderWithRetry(ctx, req, contracts, grpcTimeout)
			for _, callErr := range callErrs {
				log.Warn(
					"RunChecks failed for agent",
					"provider_pubkey", batch.GetProviderPubkey(),
					"endpoint", callErr.Endpoint,
					"error", callErr.Err,
				)
			}

			mergedMu.Lock()
			if rpcFailed {
				providersRPCFailed++
			}
			for key, row := range providerMerged {
				merged[key] = row
				if row.Reason == constants.ValidStorageProof {
					validTotal++
				}
			}
			mergedMu.Unlock()
		}()
	}

	wg.Wait()

	if recorder != nil {
		for _, sc := range storageContracts {
			key := contractRelationKey(sc)
			if _, inBatch := build.checkedKeys[key]; !inBatch {
				recorder.RecordBagEndpointUnresolved(sc)
				continue
			}
			row, ok := merged[key]
			if !ok {
				recorder.RecordBagTransition(sc, constants.NotFound, pipelineevents.StageAgentRunChecksNoResult, "agent did not return result for contract")
				continue
			}
			recorder.RecordBagTransition(sc, row.Reason, row.Stage, row.Details)
		}
	}

	contractProofsChecks := make([]db.ContractProofsCheck, 0, len(merged))
	for _, row := range merged {
		contractProofsChecks = append(contractProofsChecks, row.ContractProofsCheck)
	}

	if err := w.providers.UpdateContractProofsChecks(ctx, contractProofsChecks); err != nil {
		log.Error("failed to update contract proofs checks", "error", err)
		return err
	}

	log.Info(
		"successfully updated contract proofs checks",
		"mode", "per_provider",
		"count", len(contractProofsChecks),
		"valid", validTotal,
		"providers_total", len(build.batches),
		"providers_rpc_failed", providersRPCFailed,
		"agents_total", w.agentClient.AgentCount(),
	)

	return nil
}

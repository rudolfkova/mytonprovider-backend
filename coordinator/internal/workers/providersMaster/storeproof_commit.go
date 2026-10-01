package providersmaster

import (
	"time"

	"mytonprovider-coordinator/internal/constants"
	"mytonprovider-coordinator/internal/models/db"
	"mytonprovider-coordinator/internal/pipelineevents"
)

const storeProofRetryBuffer = 5 * time.Minute

func isRetriableReason(code constants.ReasonCode) bool {
	switch code {
	case constants.IPNotFound,
		constants.NotFound,
		constants.UnavailableProvider,
		constants.CantCreatePeer,
		constants.UnknownPeer,
		constants.PingFailed,
		constants.FailedInitialPing,
		constants.GetInfoFailed,
		constants.CantGetPiece:
		return true
	default:
		return false
	}
}

func buildBagCheckResults(
	storageContracts []db.ContractToProviderRelation,
	checkedKeys map[string]struct{},
	merged map[string]mergedContractCheck,
) map[string]mergedContractCheck {
	out := make(map[string]mergedContractCheck, len(storageContracts))
	for _, sc := range storageContracts {
		key := contractRelationKey(sc)
		if _, inBatch := checkedKeys[key]; !inBatch {
			out[key] = mergedContractCheck{
				ContractProofsCheck: db.ContractProofsCheck{
					ContractAddress: sc.Address,
					ProviderAddress: sc.ProviderAddress,
					Reason:          constants.IPNotFound,
				},
				Details: "storage endpoint not resolved for RunChecks",
				Stage:   pipelineevents.StageEndpointUnresolved,
			}
			continue
		}
		if row, ok := merged[key]; ok {
			out[key] = row
			continue
		}
		out[key] = mergedContractCheck{
			ContractProofsCheck: db.ContractProofsCheck{
				ContractAddress: sc.Address,
				ProviderAddress: sc.ProviderAddress,
				Reason:          constants.NotFound,
			},
			Details: "agent did not return result for contract",
			Stage:   pipelineevents.StageAgentRunChecksNoResult,
		}
	}
	return out
}

// selectMainProofsCommit returns rows to write after the main RunChecks pass
// and the set of retriable-fail keys whose public reason is deferred.
func selectMainProofsCommit(results map[string]mergedContractCheck) (toCommit []db.ContractProofsCheck, pendingKeys map[string]struct{}) {
	pendingKeys = make(map[string]struct{})
	toCommit = make([]db.ContractProofsCheck, 0, len(results))
	for key, row := range results {
		if row.Reason == constants.ValidStorageProof || !isRetriableReason(row.Reason) {
			toCommit = append(toCommit, row.ContractProofsCheck)
			continue
		}
		pendingKeys[key] = struct{}{}
	}
	return toCommit, pendingKeys
}

// selectFinalProofsCommit commits deferred bags after retry (or skip).
// For each pending key, prefer retryMerged when present; otherwise use mainResults.
func selectFinalProofsCommit(
	mainResults map[string]mergedContractCheck,
	pendingKeys map[string]struct{},
	retryMerged map[string]mergedContractCheck,
) []db.ContractProofsCheck {
	if len(pendingKeys) == 0 {
		return nil
	}
	out := make([]db.ContractProofsCheck, 0, len(pendingKeys))
	for key := range pendingKeys {
		if retryMerged != nil {
			if row, ok := retryMerged[key]; ok {
				out = append(out, row.ContractProofsCheck)
				continue
			}
		}
		if row, ok := mainResults[key]; ok {
			out = append(out, row.ContractProofsCheck)
		}
	}
	return out
}

func pendingContracts(
	storageContracts []db.ContractToProviderRelation,
	pendingKeys map[string]struct{},
) []db.ContractToProviderRelation {
	if len(pendingKeys) == 0 {
		return nil
	}
	out := make([]db.ContractToProviderRelation, 0, len(pendingKeys))
	for _, sc := range storageContracts {
		if _, ok := pendingKeys[contractRelationKey(sc)]; ok {
			out = append(out, sc)
		}
	}
	return out
}

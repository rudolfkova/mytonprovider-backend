package providersmaster

import (
	"crypto/ed25519"
	"strings"

	"mytonprovider-coordinator/internal/clients/agentrpc"
	"mytonprovider-coordinator/internal/constants"
	"mytonprovider-coordinator/internal/models/db"
)

func isValidRunChecksEndpoint(ip db.IPInfo) bool {
	return strings.TrimSpace(ip.IP) != "" &&
		ip.Port > 0 &&
		len(ip.PublicKey) == ed25519.PublicKeySize
}

func selectStorageEndpoint(ip db.ProviderIP) (db.IPInfo, bool) {
	if isValidRunChecksEndpoint(ip.Storage) {
		return ip.Storage, true
	}
	return db.IPInfo{}, false
}

func selectProviderEndpoint(ip db.ProviderIP) (db.IPInfo, bool) {
	if isValidRunChecksEndpoint(ip.Provider) {
		return ip.Provider, true
	}
	return db.IPInfo{}, false
}

func endpointsDiffer(a, b db.IPInfo) bool {
	if strings.TrimSpace(a.IP) != strings.TrimSpace(b.IP) {
		return true
	}
	if a.Port != b.Port {
		return true
	}
	return !pubkeysEqual(a.PublicKey, b.PublicKey)
}

func pubkeysEqual(a, b ed25519.PublicKey) bool {
	if len(a) != ed25519.PublicKeySize || len(b) != ed25519.PublicKeySize {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// shouldRetryWithProviderPort: phase-2 targets storage initial ping failures (likely wrong port).
func shouldRetryWithProviderPort(reason constants.ReasonCode) bool {
	return reason == constants.FailedInitialPing
}

func contractRelationKey(providerAddress, contractAddress string) string {
	return providerAddress + "|" + contractAddress
}

// indexValidContractKeys returns contract keys with VALID_STORAGE_PROOF from agent responses.
func indexValidContractKeys(responses []agentrpc.RunChecksResult) map[string]struct{} {
	valid := make(map[string]struct{})
	for _, agentResp := range responses {
		if agentResp.Response == nil {
			continue
		}
		for _, row := range agentResp.Response.GetResults() {
			if row == nil {
				continue
			}
			if reasonFromProto(row.GetReasonCode()) != constants.ValidStorageProof {
				continue
			}
			valid[contractRelationKey(row.GetProviderAddress(), row.GetContractAddress())] = struct{}{}
		}
	}
	return valid
}

// collectProviderPortRetryContracts picks contracts for phase 2 (provider endpoint):
// - phase 1 used storage endpoint and got FAILED_INITIAL_PING on at least one agent
// - no agent returned VALID in phase 1
// - storage and provider endpoints differ
func collectProviderPortRetryContracts(
	storageContracts []db.ContractToProviderRelation,
	phase1Responses []agentrpc.RunChecksResult,
	availableProvidersIPs map[string]db.ProviderIP,
) []db.ContractToProviderRelation {
	phase1Valid := indexValidContractKeys(phase1Responses)

	storagePingFailed := make(map[string]struct{})
	for _, agentResp := range phase1Responses {
		if agentResp.Response == nil {
			continue
		}
		for _, row := range agentResp.Response.GetResults() {
			if row == nil {
				continue
			}
			reason := reasonFromProto(row.GetReasonCode())
			if !shouldRetryWithProviderPort(reason) {
				continue
			}
			storagePingFailed[contractRelationKey(row.GetProviderAddress(), row.GetContractAddress())] = struct{}{}
		}
	}

	retry := make([]db.ContractToProviderRelation, 0)
	seen := make(map[string]struct{}, len(storagePingFailed))
	for _, sc := range storageContracts {
		key := contractRelationKey(sc.ProviderAddress, sc.Address)
		if _, ok := phase1Valid[key]; ok {
			continue
		}
		if _, ok := storagePingFailed[key]; !ok {
			continue
		}
		ip, ok := availableProvidersIPs[sc.ProviderPublicKey]
		if !ok {
			continue
		}
		if !isValidRunChecksEndpoint(ip.Storage) || !isValidRunChecksEndpoint(ip.Provider) {
			continue
		}
		if !endpointsDiffer(ip.Storage, ip.Provider) {
			continue
		}
		if _, dup := seen[key]; dup {
			continue
		}
		seen[key] = struct{}{}
		retry = append(retry, sc)
	}
	return retry
}

// countPhase2RescuedContracts counts retry-set contracts that became VALID only in phase-2 responses.
func countPhase2RescuedContracts(
	retryContracts []db.ContractToProviderRelation,
	phase2Responses []agentrpc.RunChecksResult,
) int {
	if len(retryContracts) == 0 {
		return 0
	}
	phase2Valid := indexValidContractKeys(phase2Responses)
	rescued := 0
	for _, sc := range retryContracts {
		if _, ok := phase2Valid[contractRelationKey(sc.ProviderAddress, sc.Address)]; ok {
			rescued++
		}
	}
	return rescued
}

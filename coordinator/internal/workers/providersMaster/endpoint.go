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

// shouldRetryWithProviderPort decides if phase-1 failure may be retried on provider endpoint.
// Broad for now: any non-valid; narrow later by filtering reason codes here.
func shouldRetryWithProviderPort(reason constants.ReasonCode) bool {
	return reason != constants.ValidStorageProof
}

func contractRelationKey(providerAddress, contractAddress string) string {
	return providerAddress + "|" + contractAddress
}

// collectProviderPortRetryContracts returns contracts to re-check via provider DHT endpoint
// after phase 1 used storage endpoint and failed.
func collectProviderPortRetryContracts(
	storageContracts []db.ContractToProviderRelation,
	responses []agentrpc.RunChecksResult,
	availableProvidersIPs map[string]db.ProviderIP,
) []db.ContractToProviderRelation {
	failedKeys := make(map[string]struct{})
	for _, agentResp := range responses {
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
			failedKeys[contractRelationKey(row.GetProviderAddress(), row.GetContractAddress())] = struct{}{}
		}
	}

	retry := make([]db.ContractToProviderRelation, 0)
	seen := make(map[string]struct{}, len(failedKeys))
	for _, sc := range storageContracts {
		key := contractRelationKey(sc.ProviderAddress, sc.Address)
		if _, failed := failedKeys[key]; !failed {
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

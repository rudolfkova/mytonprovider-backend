package providersmaster

import (
	"crypto/ed25519"
	"strings"

	"mytonprovider-coordinator/internal/models/db"
)

func isValidRunChecksEndpoint(ip db.IPInfo) bool {
	return strings.TrimSpace(ip.IP) != "" &&
		ip.Port > 0 &&
		len(ip.PublicKey) == ed25519.PublicKeySize
}

// selectRunChecksEndpoint picks the ADNL endpoint for StoreProof checks.
// Provider (DHT storage-provider, same path as resolver/rates) is preferred;
// Storage (from VerifyStorageADNLProof) is used when provider is unavailable.
func selectRunChecksEndpoint(ip db.ProviderIP) (db.IPInfo, bool) {
	if isValidRunChecksEndpoint(ip.Provider) {
		return ip.Provider, true
	}
	if isValidRunChecksEndpoint(ip.Storage) {
		return ip.Storage, true
	}
	return db.IPInfo{}, false
}

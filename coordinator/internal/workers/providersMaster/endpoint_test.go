package providersmaster

import (
	"crypto/ed25519"
	"testing"

	providerchecksv1 "mytonprovider-contracts/gen/go/providerchecks/v1"
	"mytonprovider-coordinator/internal/clients/agentrpc"
	"mytonprovider-coordinator/internal/constants"
	"mytonprovider-coordinator/internal/models/db"
)

func TestSelectStorageAndProviderEndpoints(t *testing.T) {
	_, priv, _ := ed25519.GenerateKey(nil)
	pub := priv.Public().(ed25519.PublicKey)

	provider := db.IPInfo{IP: "1.2.3.4", Port: 39626, PublicKey: pub}
	storage := db.IPInfo{IP: "5.6.7.8", Port: 16015, PublicKey: pub}

	got, ok := selectStorageEndpoint(db.ProviderIP{Provider: provider, Storage: storage})
	if !ok || got.Port != 16015 {
		t.Fatalf("storage: got %+v ok=%v", got, ok)
	}

	got, ok = selectProviderEndpoint(db.ProviderIP{Provider: provider, Storage: storage})
	if !ok || got.Port != 39626 {
		t.Fatalf("provider: got %+v ok=%v", got, ok)
	}
}

func TestEndpointsDiffer(t *testing.T) {
	_, priv, _ := ed25519.GenerateKey(nil)
	pub := priv.Public().(ed25519.PublicKey)

	same := db.IPInfo{IP: "157.250.201.151", Port: 16015, PublicKey: pub}
	if endpointsDiffer(same, same) {
		t.Fatal("identical endpoints should not differ")
	}

	otherPort := db.IPInfo{IP: "157.250.201.151", Port: 39626, PublicKey: pub}
	if !endpointsDiffer(same, otherPort) {
		t.Fatal("different ports should differ")
	}
}

func TestCollectProviderPortRetryContracts(t *testing.T) {
	_, priv, _ := ed25519.GenerateKey(nil)
	pub := priv.Public().(ed25519.PublicKey)

	contracts := []db.ContractToProviderRelation{
		{
			ProviderPublicKey: "pub1",
			ProviderAddress:   "prov-addr-1",
			Address:           "contract-1",
			BagID:             "bag1",
		},
		{
			ProviderPublicKey: "pub1",
			ProviderAddress:   "prov-addr-1",
			Address:           "contract-2",
			BagID:             "bag2",
		},
		{
			ProviderPublicKey: "pub2",
			ProviderAddress:   "prov-addr-2",
			Address:           "contract-3",
			BagID:             "bag3",
		},
	}

	ips := map[string]db.ProviderIP{
		"pub1": {
			Storage:  db.IPInfo{IP: "10.0.0.1", Port: 16015, PublicKey: pub},
			Provider: db.IPInfo{IP: "10.0.0.1", Port: 39626, PublicKey: pub},
		},
		"pub2": {
			Storage:  db.IPInfo{IP: "10.0.0.2", Port: 9000, PublicKey: pub},
			Provider: db.IPInfo{IP: "10.0.0.2", Port: 9000, PublicKey: pub},
		},
	}

	responses := []agentrpc.RunChecksResult{
		{
			Response: &providerchecksv1.RunChecksResponse{
				Results: []*providerchecksv1.ContractCheckResult{
					{
						ProviderAddress: "prov-addr-1",
						ContractAddress: "contract-1",
						ReasonCode:      providerchecksv1.ReasonCode_FAILED_INITIAL_PING,
					},
					{
						ProviderAddress: "prov-addr-1",
						ContractAddress: "contract-2",
						ReasonCode:      providerchecksv1.ReasonCode_VALID_STORAGE_PROOF,
					},
					{
						ProviderAddress: "prov-addr-2",
						ContractAddress: "contract-3",
						ReasonCode:      providerchecksv1.ReasonCode_GET_INFO_FAILED,
					},
				},
			},
		},
	}

	retry := collectProviderPortRetryContracts(contracts, responses, ips)
	if len(retry) != 1 {
		t.Fatalf("expected 1 retry contract, got %d", len(retry))
	}
	if retry[0].Address != "contract-1" {
		t.Fatalf("expected contract-1, got %s", retry[0].Address)
	}
}

func TestCollectProviderPortRetryContracts_SkipsPhase1ValidFromOtherAgent(t *testing.T) {
	_, priv, _ := ed25519.GenerateKey(nil)
	pub := priv.Public().(ed25519.PublicKey)

	contracts := []db.ContractToProviderRelation{
		{
			ProviderPublicKey: "pub1",
			ProviderAddress:   "prov-addr-1",
			Address:           "contract-1",
			BagID:             "bag1",
		},
	}
	ips := map[string]db.ProviderIP{
		"pub1": {
			Storage:  db.IPInfo{IP: "10.0.0.1", Port: 16015, PublicKey: pub},
			Provider: db.IPInfo{IP: "10.0.0.1", Port: 39626, PublicKey: pub},
		},
	}

	responses := []agentrpc.RunChecksResult{
		{
			Response: &providerchecksv1.RunChecksResponse{
				Results: []*providerchecksv1.ContractCheckResult{
					{
						ProviderAddress: "prov-addr-1",
						ContractAddress: "contract-1",
						ReasonCode:      providerchecksv1.ReasonCode_FAILED_INITIAL_PING,
					},
				},
			},
		},
		{
			Response: &providerchecksv1.RunChecksResponse{
				Results: []*providerchecksv1.ContractCheckResult{
					{
						ProviderAddress: "prov-addr-1",
						ContractAddress: "contract-1",
						ReasonCode:      providerchecksv1.ReasonCode_VALID_STORAGE_PROOF,
					},
				},
			},
		},
	}

	retry := collectProviderPortRetryContracts(contracts, responses, ips)
	if len(retry) != 0 {
		t.Fatalf("expected no retry when any agent returned VALID in phase 1, got %d", len(retry))
	}
}

func TestCollectProviderPortRetryContracts_OnlyFailedInitialPing(t *testing.T) {
	_, priv, _ := ed25519.GenerateKey(nil)
	pub := priv.Public().(ed25519.PublicKey)

	contracts := []db.ContractToProviderRelation{
		{
			ProviderPublicKey: "pub1",
			ProviderAddress:   "prov-addr-1",
			Address:           "contract-1",
			BagID:             "bag1",
		},
	}
	ips := map[string]db.ProviderIP{
		"pub1": {
			Storage:  db.IPInfo{IP: "10.0.0.1", Port: 16015, PublicKey: pub},
			Provider: db.IPInfo{IP: "10.0.0.1", Port: 39626, PublicKey: pub},
		},
	}

	responses := []agentrpc.RunChecksResult{
		{
			Response: &providerchecksv1.RunChecksResponse{
				Results: []*providerchecksv1.ContractCheckResult{
					{
						ProviderAddress: "prov-addr-1",
						ContractAddress: "contract-1",
						ReasonCode:      providerchecksv1.ReasonCode_GET_INFO_FAILED,
					},
				},
			},
		},
	}

	retry := collectProviderPortRetryContracts(contracts, responses, ips)
	if len(retry) != 0 {
		t.Fatalf("expected no retry for non-203 failures, got %d", len(retry))
	}
}

func TestShouldRetryWithProviderPort(t *testing.T) {
	if !shouldRetryWithProviderPort(constants.FailedInitialPing) {
		t.Fatal("expected retry on 203")
	}
	if shouldRetryWithProviderPort(constants.ValidStorageProof) {
		t.Fatal("expected no retry on valid")
	}
	if shouldRetryWithProviderPort(constants.GetInfoFailed) {
		t.Fatal("expected no retry on non-203 errors")
	}
}

func TestCountPhase2RescuedContracts(t *testing.T) {
	retry := []db.ContractToProviderRelation{
		{ProviderAddress: "p1", Address: "c1"},
		{ProviderAddress: "p1", Address: "c2"},
	}
	phase2 := []agentrpc.RunChecksResult{
		{
			Response: &providerchecksv1.RunChecksResponse{
				Results: []*providerchecksv1.ContractCheckResult{
					{
						ProviderAddress: "p1",
						ContractAddress: "c2",
						ReasonCode:      providerchecksv1.ReasonCode_VALID_STORAGE_PROOF,
					},
					{
						ProviderAddress: "p1",
						ContractAddress: "c1",
						ReasonCode:      providerchecksv1.ReasonCode_FAILED_INITIAL_PING,
					},
				},
			},
		},
	}
	if got := countPhase2RescuedContracts(retry, phase2); got != 1 {
		t.Fatalf("expected 1 rescued, got %d", got)
	}
}

package providersmaster

import (
	"testing"

	providerchecksv1 "mytonprovider-contracts/gen/go/providerchecks/v1"
	"mytonprovider-coordinator/internal/clients/agentrpc"
	"mytonprovider-coordinator/internal/constants"
	"mytonprovider-coordinator/internal/models/db"
)

func TestMergeRunChecksResponses_PrefersValidProof(t *testing.T) {
	contracts := []db.ContractToProviderRelation{
		{
			ProviderPublicKey: "pub-1",
			ProviderAddress:   "provider-addr-1",
			Address:           "contract-1",
			BagID:             "bag-1",
		},
	}

	responses := []agentrpc.RunChecksResult{
		{
			Endpoint: "agent-1",
			Response: &providerchecksv1.RunChecksResponse{
				Results: []*providerchecksv1.ContractCheckResult{
					{
						ProviderAddress: "provider-addr-1",
						ContractAddress: "contract-1",
						ReasonCode:      providerchecksv1.ReasonCode_UNAVAILABLE_PROVIDER,
					},
				},
			},
		},
		{
			Endpoint: "agent-2",
			Response: &providerchecksv1.RunChecksResponse{
				Results: []*providerchecksv1.ContractCheckResult{
					{
						ProviderAddress: "provider-addr-1",
						ContractAddress: "contract-1",
						ReasonCode:      providerchecksv1.ReasonCode_VALID_STORAGE_PROOF,
					},
				},
			},
		},
	}

	merged, valid := mergeRunChecksResponses(contracts, responses)
	if len(merged) != 1 {
		t.Fatalf("expected 1 merged row, got %d", len(merged))
	}
	if merged[0].Reason != constants.ValidStorageProof {
		t.Fatalf("expected valid proof reason, got %d", merged[0].Reason)
	}
	if valid != 1 {
		t.Fatalf("expected valid counter=1, got %d", valid)
	}
}

func TestMergeStorageRatesResponses_PrefersOkResult(t *testing.T) {
	pubkeys := []string{"pub-1"}
	responses := []agentrpc.RunStorageRatesResult{
		{
			Endpoint: "agent-1",
			Response: &providerchecksv1.RunStorageRatesResponse{
				Results: []*providerchecksv1.StorageRatesResult{
					{
						ProviderPubkey: "pub-1",
						Ok:             false,
						Details:        "timeout",
					},
				},
			},
		},
		{
			Endpoint: "agent-2",
			Response: &providerchecksv1.RunStorageRatesResponse{
				Results: []*providerchecksv1.StorageRatesResult{
					{
						ProviderPubkey: "pub-1",
						Ok:             true,
						MinSpan:        10,
						MaxSpan:        20,
					},
				},
			},
		},
	}

	merged := mergeStorageRatesResponses(pubkeys, responses)
	row, ok := merged["pub-1"]
	if !ok {
		t.Fatalf("expected merged row for pub-1")
	}
	if !row.GetOk() {
		t.Fatalf("expected merged row to be ok=true")
	}
	if row.GetMinSpan() != 10 || row.GetMaxSpan() != 20 {
		t.Fatalf("unexpected merged spans: min=%d max=%d", row.GetMinSpan(), row.GetMaxSpan())
	}
}

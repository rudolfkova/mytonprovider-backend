package providersmaster

import (
	"testing"

	"mytonprovider-coordinator/internal/constants"
	"mytonprovider-coordinator/internal/models/db"
	"mytonprovider-coordinator/internal/pipelineevents"
)

func TestIsRetriableReason(t *testing.T) {
	retriable := []constants.ReasonCode{
		constants.IPNotFound,
		constants.NotFound,
		constants.UnavailableProvider,
		constants.CantCreatePeer,
		constants.UnknownPeer,
		constants.PingFailed,
		constants.FailedInitialPing,
		constants.GetInfoFailed,
		constants.CantGetPiece,
	}
	for _, code := range retriable {
		if !isRetriableReason(code) {
			t.Fatalf("expected retriable reason %d", code)
		}
	}

	nonRetriable := []constants.ReasonCode{
		constants.ValidStorageProof,
		constants.InvalidBagID,
		constants.InvalidHeader,
		constants.CantParseBoC,
		constants.ProofCheckFailed,
	}
	for _, code := range nonRetriable {
		if isRetriableReason(code) {
			t.Fatalf("expected non-retriable reason %d", code)
		}
	}
}

func TestSelectMainProofsCommit(t *testing.T) {
	results := map[string]mergedContractCheck{
		"p|ok": {
			ContractProofsCheck: db.ContractProofsCheck{
				ContractAddress: "ok",
				ProviderAddress: "p",
				Reason:          constants.ValidStorageProof,
			},
		},
		"p|retriable": {
			ContractProofsCheck: db.ContractProofsCheck{
				ContractAddress: "retriable",
				ProviderAddress: "p",
				Reason:          constants.UnavailableProvider,
			},
		},
		"p|dead": {
			ContractProofsCheck: db.ContractProofsCheck{
				ContractAddress: "dead",
				ProviderAddress: "p",
				Reason:          constants.ProofCheckFailed,
			},
		},
		"p|nullish": {
			ContractProofsCheck: db.ContractProofsCheck{
				ContractAddress: "nullish",
				ProviderAddress: "p",
				Reason:          constants.IPNotFound,
			},
		},
	}

	toCommit, pending := selectMainProofsCommit(results)
	if len(toCommit) != 2 {
		t.Fatalf("expected 2 immediate commits, got %d", len(toCommit))
	}
	if len(pending) != 2 {
		t.Fatalf("expected 2 pending keys, got %d", len(pending))
	}
	if _, ok := pending["p|retriable"]; !ok {
		t.Fatalf("expected retriable pending")
	}
	if _, ok := pending["p|nullish"]; !ok {
		t.Fatalf("expected IPNotFound pending")
	}

	committed := map[string]constants.ReasonCode{}
	for _, row := range toCommit {
		committed[row.ProviderAddress+"|"+row.ContractAddress] = row.Reason
	}
	if committed["p|ok"] != constants.ValidStorageProof {
		t.Fatalf("ok bag not committed")
	}
	if committed["p|dead"] != constants.ProofCheckFailed {
		t.Fatalf("non-retriable fail not committed")
	}
}

func TestSelectFinalProofsCommit(t *testing.T) {
	mainResults := map[string]mergedContractCheck{
		"p|a": {
			ContractProofsCheck: db.ContractProofsCheck{
				ContractAddress: "a",
				ProviderAddress: "p",
				Reason:          constants.PingFailed,
			},
		},
		"p|b": {
			ContractProofsCheck: db.ContractProofsCheck{
				ContractAddress: "b",
				ProviderAddress: "p",
				Reason:          constants.UnavailableProvider,
			},
		},
	}
	pending := map[string]struct{}{
		"p|a": {},
		"p|b": {},
	}

	// skip retry: commit main fails
	final := selectFinalProofsCommit(mainResults, pending, nil)
	if len(final) != 2 {
		t.Fatalf("expected 2 final commits on skip, got %d", len(final))
	}

	retry := map[string]mergedContractCheck{
		"p|a": {
			ContractProofsCheck: db.ContractProofsCheck{
				ContractAddress: "a",
				ProviderAddress: "p",
				Reason:          constants.ValidStorageProof,
			},
		},
		// p|b missing -> fall back to main
	}
	final = selectFinalProofsCommit(mainResults, pending, retry)
	byKey := map[string]constants.ReasonCode{}
	for _, row := range final {
		byKey[row.ProviderAddress+"|"+row.ContractAddress] = row.Reason
	}
	if byKey["p|a"] != constants.ValidStorageProof {
		t.Fatalf("expected recovery for a")
	}
	if byKey["p|b"] != constants.UnavailableProvider {
		t.Fatalf("expected main fail kept for b")
	}
}

func TestBuildBagCheckResults(t *testing.T) {
	contracts := []db.ContractToProviderRelation{
		{ProviderAddress: "p", Address: "resolved", BagID: "b1"},
		{ProviderAddress: "p", Address: "missing", BagID: "b2"},
		{ProviderAddress: "p", Address: "unresolved", BagID: "b3"},
	}
	checked := map[string]struct{}{
		"p|resolved": {},
		"p|missing":  {},
	}
	merged := map[string]mergedContractCheck{
		"p|resolved": {
			ContractProofsCheck: db.ContractProofsCheck{
				ContractAddress: "resolved",
				ProviderAddress: "p",
				Reason:          constants.ValidStorageProof,
			},
			Stage: "ok",
		},
	}

	out := buildBagCheckResults(contracts, checked, merged)
	if out["p|resolved"].Reason != constants.ValidStorageProof {
		t.Fatalf("resolved reason")
	}
	if out["p|missing"].Reason != constants.NotFound {
		t.Fatalf("missing agent result should be NotFound")
	}
	if out["p|missing"].Stage != pipelineevents.StageAgentRunChecksNoResult {
		t.Fatalf("missing stage")
	}
	if out["p|unresolved"].Reason != constants.IPNotFound {
		t.Fatalf("unresolved should be IPNotFound")
	}
	if out["p|unresolved"].Stage != pipelineevents.StageEndpointUnresolved {
		t.Fatalf("unresolved stage")
	}
}

func TestPendingContracts(t *testing.T) {
	contracts := []db.ContractToProviderRelation{
		{ProviderAddress: "p", Address: "a"},
		{ProviderAddress: "p", Address: "b"},
		{ProviderAddress: "p", Address: "c"},
	}
	pending := map[string]struct{}{"p|a": {}, "p|c": {}}
	got := pendingContracts(contracts, pending)
	if len(got) != 2 {
		t.Fatalf("expected 2 pending contracts, got %d", len(got))
	}
}

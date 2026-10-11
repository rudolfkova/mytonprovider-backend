package providersmaster

import (
	"testing"

	providerchecksv1 "github.com/rudolfkova/mytonprovider-backend/contracts/gen/go/providerchecks/v1"
)

func TestAssignProvidersToAgents_BalancesByBags(t *testing.T) {
	providers := []*providerchecksv1.ProviderBatch{
		batchWithContracts("p-small", 2),
		batchWithContracts("p-large", 800),
		batchWithContracts("p-mid", 100),
	}
	endpoints := []string{"agent-a", "agent-b"}

	got := assignProvidersToAgents(providers, endpoints, nil, nil)

	if got.byProvider["p-large"] == "" {
		t.Fatal("expected p-large assigned")
	}
	// largest first to agent-a (first in list at equal load), then p-mid to agent-b, then p-small to agent-b (lower load)
	if got.byProvider["p-large"] != "agent-a" {
		t.Fatalf("expected p-large on agent-a, got %q", got.byProvider["p-large"])
	}
	if got.byProvider["p-mid"] != "agent-b" {
		t.Fatalf("expected p-mid on agent-b, got %q", got.byProvider["p-mid"])
	}
	if got.byProvider["p-small"] != "agent-b" {
		t.Fatalf("expected p-small on agent-b, got %q", got.byProvider["p-small"])
	}

	loadA := bagLoad(got.byEndpoint["agent-a"])
	loadB := bagLoad(got.byEndpoint["agent-b"])
	if loadA != 800 || loadB != 102 {
		t.Fatalf("unexpected loads a=%d b=%d", loadA, loadB)
	}
}

func TestAssignProvidersToAgents_StickyWins(t *testing.T) {
	providers := []*providerchecksv1.ProviderBatch{
		batchWithContracts("p1", 10),
		batchWithContracts("p2", 10),
	}
	endpoints := []string{"agent-a", "agent-b"}
	sticky := map[string]string{
		"p1": "agent-b",
		"p2": "agent-b",
	}

	got := assignProvidersToAgents(providers, endpoints, sticky, nil)

	if got.byProvider["p1"] != "agent-b" || got.byProvider["p2"] != "agent-b" {
		t.Fatalf("sticky should keep both on agent-b, got %#v", got.byProvider)
	}
	if bagLoad(got.byEndpoint["agent-a"]) != 0 {
		t.Fatalf("agent-a should have no bags, got %d", bagLoad(got.byEndpoint["agent-a"]))
	}
}

func TestAssignProvidersToAgents_Deterministic(t *testing.T) {
	providers := []*providerchecksv1.ProviderBatch{
		batchWithContracts("z", 5),
		batchWithContracts("a", 5),
		batchWithContracts("m", 5),
	}
	endpoints := []string{"agent-a", "agent-b"}

	first := assignProvidersToAgents(providers, endpoints, nil, nil)
	second := assignProvidersToAgents(providers, endpoints, nil, nil)

	for _, key := range []string{"a", "m", "z"} {
		if first.byProvider[key] != second.byProvider[key] {
			t.Fatalf("non-deterministic assignment for %s: %q vs %q", key, first.byProvider[key], second.byProvider[key])
		}
	}
	// equal bag counts: sorted by pubkey asc among equals after desc bag sort → a, m, z
	// a→agent-a, m→agent-b, z→agent-a
	if first.byProvider["a"] != "agent-a" || first.byProvider["m"] != "agent-b" || first.byProvider["z"] != "agent-a" {
		t.Fatalf("unexpected assignment %#v", first.byProvider)
	}
}

func TestAssignProvidersToAgents_RetryExcludesMainAgent(t *testing.T) {
	providers := []*providerchecksv1.ProviderBatch{
		batchWithContracts("p1", 50),
		batchWithContracts("p2", 50),
	}
	endpoints := []string{"agent-a", "agent-b"}
	exclude := map[string]string{
		"p1": "agent-a",
		"p2": "agent-b",
	}
	sticky := map[string]string{
		"p1": "agent-a", // sticky toward excluded must be ignored when alternatives exist
		"p2": "agent-b",
	}

	got := assignProvidersToAgents(providers, endpoints, sticky, exclude)

	if got.byProvider["p1"] != "agent-b" {
		t.Fatalf("p1 should retry on agent-b, got %q", got.byProvider["p1"])
	}
	if got.byProvider["p2"] != "agent-a" {
		t.Fatalf("p2 should retry on agent-a, got %q", got.byProvider["p2"])
	}
}

func TestAssignProvidersToAgents_SingleAgentIgnoresExclude(t *testing.T) {
	providers := []*providerchecksv1.ProviderBatch{
		batchWithContracts("p1", 3),
	}
	endpoints := []string{"agent-a"}
	exclude := map[string]string{"p1": "agent-a"}

	got := assignProvidersToAgents(providers, endpoints, nil, exclude)
	if got.byProvider["p1"] != "agent-a" {
		t.Fatalf("single agent must still get work, got %q", got.byProvider["p1"])
	}
}

func batchWithContracts(pubkey string, n int) *providerchecksv1.ProviderBatch {
	contracts := make([]*providerchecksv1.ContractRef, n)
	for i := 0; i < n; i++ {
		contracts[i] = &providerchecksv1.ContractRef{ContractAddress: pubkey}
	}
	return &providerchecksv1.ProviderBatch{
		ProviderPubkey: pubkey,
		Contracts:      contracts,
	}
}

func bagLoad(batches []*providerchecksv1.ProviderBatch) int {
	total := 0
	for _, b := range batches {
		total += len(b.GetContracts())
	}
	return total
}

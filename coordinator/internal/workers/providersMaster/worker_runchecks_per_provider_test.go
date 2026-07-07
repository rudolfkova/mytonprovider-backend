package providersmaster

import (
	"context"
	"crypto/ed25519"
	"errors"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	providerchecksv1 "github.com/rudolfkova/mytonprovider-backend/contracts/gen/go/providerchecks/v1"
	"mytonprovider-coordinator/internal/clients/agentrpc"
	"mytonprovider-coordinator/internal/constants"
	"mytonprovider-coordinator/internal/models/db"
	"mytonprovider-coordinator/internal/pipelineevents"
)

func TestProviderTotalTimeoutMs(t *testing.T) {
	const msPerBag uint32 = 500
	const minTotalMs uint32 = 3000

	tests := []struct {
		name     string
		bagCount int
		want     uint32
	}{
		{name: "zero bags uses floor", bagCount: 0, want: 3000},
		{name: "one bag uses floor", bagCount: 1, want: 3000},
		{name: "ten bags above floor", bagCount: 10, want: 5000},
		{name: "large provider", bagCount: 500, want: 250000},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := providerTotalTimeoutMs(tt.bagCount, msPerBag, minTotalMs); got != tt.want {
				t.Fatalf("providerTotalTimeoutMs(%d) = %d, want %d", tt.bagCount, got, tt.want)
			}
		})
	}
}

type fakeRunChecksAgentClient struct {
	mu sync.Mutex

	agentCount int
	handler    func(call int, req *providerchecksv1.RunChecksRequest) ([]agentrpc.RunChecksResult, []agentrpc.AgentCallError)
	calls      int
}

func (f *fakeRunChecksAgentClient) AgentCount() int {
	if f.agentCount <= 0 {
		return 3
	}
	return f.agentCount
}

func (f *fakeRunChecksAgentClient) RunChecksAll(ctx context.Context, req *providerchecksv1.RunChecksRequest) ([]agentrpc.RunChecksResult, []agentrpc.AgentCallError) {
	return f.RunChecksAllWithTimeout(ctx, req, 0)
}

func (f *fakeRunChecksAgentClient) RunChecksAllWithTimeout(ctx context.Context, req *providerchecksv1.RunChecksRequest, timeout time.Duration) ([]agentrpc.RunChecksResult, []agentrpc.AgentCallError) {
	f.mu.Lock()
	f.calls++
	call := f.calls
	f.mu.Unlock()

	if f.handler != nil {
		return f.handler(call, req)
	}
	return nil, []agentrpc.AgentCallError{{Endpoint: "agent-1", Err: errors.New("unavailable")}}
}

func (f *fakeRunChecksAgentClient) RunStorageRatesAll(context.Context, *providerchecksv1.RunStorageRatesRequest) ([]agentrpc.RunStorageRatesResult, []agentrpc.AgentCallError) {
	return nil, nil
}

type capturingProofsRepo struct {
	fakeProvidersRepo
	updates []db.ContractProofsCheck
}

func (r *capturingProofsRepo) UpdateContractProofsChecks(_ context.Context, checks []db.ContractProofsCheck) error {
	r.updates = append([]db.ContractProofsCheck(nil), checks...)
	return nil
}

func testPerProviderWorker(agent agentclient) *providersMasterWorker {
	return &providersMasterWorker{
		providers:   &capturingProofsRepo{},
		agentClient: agent,
		timeouts: RunChecksTimeouts{
			PingMs:  7000,
			RldpMs:  2000,
			TotalMs: 1200000,
		},
		runChecksPerProvider: RunChecksPerProviderConfig{
			Enabled:                true,
			MaxConcurrentProviders: 40,
			MsPerBag:               500,
			MinTotalMs:             3000,
			RpcSlackMs:             5000,
			AgentRPCRetries:        1,
		},
		logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
}

func validRunChecksResponse(providerAddress, contractAddress string) []agentrpc.RunChecksResult {
	return []agentrpc.RunChecksResult{
		{
			Endpoint: "agent-1",
			Response: &providerchecksv1.RunChecksResponse{
				Results: []*providerchecksv1.ContractCheckResult{
					{
						ProviderAddress: providerAddress,
						ContractAddress: contractAddress,
						ReasonCode:      providerchecksv1.ReasonCode_VALID_STORAGE_PROOF,
					},
				},
			},
		},
	}
}

func TestRunChecksForProviderWithRetry_SucceedsOnSecondAttempt(t *testing.T) {
	agent := &fakeRunChecksAgentClient{}
	agent.handler = func(call int, req *providerchecksv1.RunChecksRequest) ([]agentrpc.RunChecksResult, []agentrpc.AgentCallError) {
		if call == 1 {
			return nil, []agentrpc.AgentCallError{{Endpoint: "agent-1", Err: errors.New("down")}}
		}
		return validRunChecksResponse("provider-1", "contract-1"), nil
	}

	w := testPerProviderWorker(agent)
	contracts := []db.ContractToProviderRelation{
		{ProviderPublicKey: "pub-1", ProviderAddress: "provider-1", Address: "contract-1", BagID: "bag-1"},
	}
	req := w.buildRunChecksRequestForProvider("storeproof-test", &providerchecksv1.ProviderBatch{
		ProviderPubkey:  "pub-1",
		ProviderAddress: "provider-1",
		Contracts:       []*providerchecksv1.ContractRef{{ContractAddress: "contract-1", BagId: "bag-1"}},
	}, 3000)

	merged, rpcFailed, _ := w.runChecksForProviderWithRetry(context.Background(), req, contracts, time.Minute)
	if rpcFailed {
		t.Fatal("expected rpc success on retry")
	}
	row, ok := merged["provider-1|contract-1"]
	if !ok {
		t.Fatal("expected merged row")
	}
	if row.Reason != constants.ValidStorageProof {
		t.Fatalf("expected valid proof, got %d", row.Reason)
	}
	if agent.calls != 2 {
		t.Fatalf("expected 2 RPC attempts, got %d", agent.calls)
	}
}

func TestRunChecksForProviderWithRetry_AllAgentsFail(t *testing.T) {
	agent := &fakeRunChecksAgentClient{
		handler: func(int, *providerchecksv1.RunChecksRequest) ([]agentrpc.RunChecksResult, []agentrpc.AgentCallError) {
			return nil, []agentrpc.AgentCallError{{Endpoint: "agent-1", Err: errors.New("down")}}
		},
	}

	w := testPerProviderWorker(agent)
	contracts := []db.ContractToProviderRelation{
		{ProviderPublicKey: "pub-1", ProviderAddress: "provider-1", Address: "contract-1", BagID: "bag-1"},
		{ProviderPublicKey: "pub-1", ProviderAddress: "provider-1", Address: "contract-2", BagID: "bag-2"},
	}
	req := w.buildRunChecksRequestForProvider("storeproof-test", &providerchecksv1.ProviderBatch{
		ProviderPubkey:  "pub-1",
		ProviderAddress: "provider-1",
		Contracts: []*providerchecksv1.ContractRef{
			{ContractAddress: "contract-1", BagId: "bag-1"},
			{ContractAddress: "contract-2", BagId: "bag-2"},
		},
	}, 3000)

	merged, rpcFailed, _ := w.runChecksForProviderWithRetry(context.Background(), req, contracts, time.Minute)
	if !rpcFailed {
		t.Fatal("expected rpc failure")
	}
	if len(merged) != 2 {
		t.Fatalf("expected 2 synthesized rows, got %d", len(merged))
	}
	for key, row := range merged {
		if row.Reason != constants.NotFound {
			t.Fatalf("%s: expected NotFound, got %d", key, row.Reason)
		}
		if row.Stage != pipelineevents.StageAgentRunChecksUnavailable {
			t.Fatalf("%s: expected unavailable stage, got %q", key, row.Stage)
		}
	}
	if agent.calls != 2 {
		t.Fatalf("expected 2 RPC attempts, got %d", agent.calls)
	}
}

func testProviderIP(pubkey string) db.ProviderIP {
	pub, _, _ := ed25519.GenerateKey(nil)
	return db.ProviderIP{
		PublicKey: pubkey,
		Storage: db.IPInfo{
			IP:        "203.0.113.10",
			Port:      12345,
			PublicKey: pub,
		},
	}
}

func TestUpdateActiveContractsPerProvider_PartialFailure(t *testing.T) {
	agent := &fakeRunChecksAgentClient{}
	agent.handler = func(_ int, req *providerchecksv1.RunChecksRequest) ([]agentrpc.RunChecksResult, []agentrpc.AgentCallError) {
		if len(req.GetProviders()) == 0 {
			return nil, nil
		}
		pubkey := req.GetProviders()[0].GetProviderPubkey()
		switch pubkey {
		case "pub-ok":
			return validRunChecksResponse("provider-ok", "contract-ok"), nil
		default:
			return nil, []agentrpc.AgentCallError{{Endpoint: "agent-1", Err: errors.New("down")}}
		}
	}

	repo := &capturingProofsRepo{}
	w := testPerProviderWorker(agent)
	w.providers = repo

	storageContracts := []db.ContractToProviderRelation{
		{ProviderPublicKey: "pub-ok", ProviderAddress: "provider-ok", Address: "contract-ok", BagID: "bag-ok"},
		{ProviderPublicKey: "pub-fail", ProviderAddress: "provider-fail", Address: "contract-fail", BagID: "bag-fail"},
	}
	ips := map[string]db.ProviderIP{
		"pub-ok":   testProviderIP("pub-ok"),
		"pub-fail": testProviderIP("pub-fail"),
	}

	err := w.updateActiveContractsPerProvider(context.Background(), "storeproof-test", storageContracts, ips, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(repo.updates) != 2 {
		t.Fatalf("expected 2 DB updates, got %d", len(repo.updates))
	}

	byKey := make(map[string]db.ContractProofsCheck, len(repo.updates))
	for _, row := range repo.updates {
		byKey[row.ProviderAddress+"|"+row.ContractAddress] = row
	}
	if byKey["provider-ok|contract-ok"].Reason != constants.ValidStorageProof {
		t.Fatalf("expected valid proof for ok provider, got %d", byKey["provider-ok|contract-ok"].Reason)
	}
	if byKey["provider-fail|contract-fail"].Reason != constants.NotFound {
		t.Fatalf("expected NotFound for failed provider, got %d", byKey["provider-fail|contract-fail"].Reason)
	}
}

func TestUpdateActiveContractsPerProvider_MergesAcrossProviders(t *testing.T) {
	agent := &fakeRunChecksAgentClient{}
	agent.handler = func(_ int, req *providerchecksv1.RunChecksRequest) ([]agentrpc.RunChecksResult, []agentrpc.AgentCallError) {
		pubkey := req.GetProviders()[0].GetProviderPubkey()
		switch pubkey {
		case "pub-a":
			return validRunChecksResponse("provider-a", "contract-a"), nil
		case "pub-b":
			return validRunChecksResponse("provider-b", "contract-b"), nil
		default:
			return nil, nil
		}
	}

	repo := &capturingProofsRepo{}
	w := testPerProviderWorker(agent)
	w.providers = repo

	storageContracts := []db.ContractToProviderRelation{
		{ProviderPublicKey: "pub-a", ProviderAddress: "provider-a", Address: "contract-a", BagID: "bag-a"},
		{ProviderPublicKey: "pub-b", ProviderAddress: "provider-b", Address: "contract-b", BagID: "bag-b"},
	}
	ips := map[string]db.ProviderIP{
		"pub-a": testProviderIP("pub-a"),
		"pub-b": testProviderIP("pub-b"),
	}

	err := w.updateActiveContractsPerProvider(context.Background(), "storeproof-test", storageContracts, ips, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(repo.updates) != 2 {
		t.Fatalf("expected 2 DB updates, got %d", len(repo.updates))
	}
}

func TestAgentUnavailableResults(t *testing.T) {
	contracts := []db.ContractToProviderRelation{
		{ProviderAddress: "provider-1", Address: "contract-1"},
	}
	out := agentUnavailableResults(contracts, 3)
	row := out["provider-1|contract-1"]
	if row.Stage != pipelineevents.StageAgentRunChecksUnavailable {
		t.Fatalf("unexpected stage: %q", row.Stage)
	}
	if row.Reason != constants.NotFound {
		t.Fatalf("unexpected reason: %d", row.Reason)
	}
}

func TestBuildRunChecksRequestForProvider_JobID(t *testing.T) {
	w := testPerProviderWorker(nil)
	req := w.buildRunChecksRequestForProvider("storeproof-123", &providerchecksv1.ProviderBatch{
		ProviderPubkey: "abcdef0123456789",
	}, 5000)
	if req.GetJobId() != "storeproof-123-abcdef01" {
		t.Fatalf("unexpected job id: %q", req.GetJobId())
	}
	if req.GetTimeouts().GetTotalMs() != 5000 {
		t.Fatalf("unexpected total ms: %d", req.GetTimeouts().GetTotalMs())
	}
}

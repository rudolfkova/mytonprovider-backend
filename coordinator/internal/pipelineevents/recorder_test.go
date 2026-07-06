package pipelineevents

import (
	"testing"

	"mytonprovider-coordinator/internal/constants"
	"mytonprovider-coordinator/internal/models/db"
)

func TestRecordBagTransition(t *testing.T) {
	rec := NewRecorder(nil, nil, "run-1", "StoreProof")

	okReason := int(constants.ValidStorageProof)
	errReason := int(constants.ProofCheckFailed)

	rel := db.ContractToProviderRelation{
		ProviderPublicKey: "aa",
		ProviderAddress:   "pa",
		Address:           "ca",
		BagID:             "bag1",
		Reason:            nil,
	}

	rec.RecordBagTransition(rel, constants.ProofCheckFailed, "check_proof", "stage=check_proof error=x")
	if len(rec.bagEvents) != 1 {
		t.Fatalf("expected 1 bag error event, got %d", len(rec.bagEvents))
	}
	if rec.bagEvents[0].Status != db.PipelineEventError {
		t.Fatalf("expected error status, got %s", rec.bagEvents[0].Status)
	}

	rec.RecordBagTransition(rel, constants.ProofCheckFailed, "check_proof", "again")
	if len(rec.bagEvents) != 2 {
		t.Fatalf("expected 2 bag error events on repeated error, got %d", len(rec.bagEvents))
	}

	rel.Reason = &errReason
	rec.RecordBagTransition(rel, constants.ValidStorageProof, "check_proof", "")
	if len(rec.bagEvents) != 3 {
		t.Fatalf("expected ok transition event, got %d events", len(rec.bagEvents))
	}
	if rec.bagEvents[2].Status != db.PipelineEventOK {
		t.Fatalf("expected ok status, got %s", rec.bagEvents[2].Status)
	}

	rel.Reason = &okReason
	rec.RecordBagTransition(rel, constants.ValidStorageProof, "check_proof", "")
	if len(rec.bagEvents) != 3 {
		t.Fatalf("expected silence after ok, got %d events", len(rec.bagEvents))
	}
}

func TestRecordProviderResolve(t *testing.T) {
	rec := NewRecorder(nil, nil, "run-1", "StoreProof")

	rec.RecordProviderResolve("pk1", StageDHTFindProviderRecord, false, errTest("timeout"))
	if len(rec.providerEvents) != 1 {
		t.Fatalf("expected provider error event")
	}
	if rec.lastProviderStatus["pk1"] != db.PipelineEventError {
		t.Fatalf("expected error state tracked")
	}

	rec.RecordProviderResolve("pk1", StageDHTFindProviderRecord, false, errTest("timeout again"))
	if len(rec.providerEvents) != 2 {
		t.Fatalf("expected repeated error rows, got %d", len(rec.providerEvents))
	}

	rec.RecordProviderResolve("pk1", StageEndpointResolve, true, nil)
	if len(rec.providerEvents) != 3 {
		t.Fatalf("expected ok transition, got %d", len(rec.providerEvents))
	}
	if rec.providerEvents[2].Status != db.PipelineEventOK {
		t.Fatalf("expected ok event")
	}

	rec.RecordProviderResolve("pk1", StageEndpointResolve, true, nil)
	if len(rec.providerEvents) != 3 {
		t.Fatalf("expected silence while healthy, got %d", len(rec.providerEvents))
	}
}

func TestParseStageFromDetails(t *testing.T) {
	if got := ParseStageFromDetails("stage=check_proof root_hash=abc error=bad"); got != "check_proof" {
		t.Fatalf("unexpected stage: %s", got)
	}
	if got := ParseStageFromDetails(""); got != StageUnknown {
		t.Fatalf("expected unknown stage")
	}
}

func TestIsErrorReason(t *testing.T) {
	if IsErrorReason(constants.ValidStorageProof) {
		t.Fatal("valid should not be error")
	}
	if !IsErrorReason(constants.ProofCheckFailed) {
		t.Fatal("403 should be error")
	}
}

type errTest string

func (e errTest) Error() string { return string(e) }

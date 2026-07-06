package pipelineevents

import (
	"context"
	"log/slog"
	"strings"

	"mytonprovider-coordinator/internal/constants"
	"mytonprovider-coordinator/internal/models/db"
)

const maxErrorMessageLen = 1024

type repository interface {
	InsertProviderPipelineEvents(ctx context.Context, events []db.ProviderPipelineEvent) error
	InsertBagPipelineEvents(ctx context.Context, events []db.BagPipelineEvent) error
	GetLastProviderPipelineEventStatus(ctx context.Context, pubkeys []string) (map[string]db.PipelineEventStatus, error)
}

type Recorder struct {
	repo                 repository
	logger               *slog.Logger
	runID                string
	worker               string
	lastProviderStatus   map[string]db.PipelineEventStatus
	providerEvents       []db.ProviderPipelineEvent
	bagEvents            []db.BagPipelineEvent
}

func NewRecorder(repo repository, logger *slog.Logger, runID, worker string) *Recorder {
	if logger == nil {
		logger = slog.Default()
	}
	return &Recorder{
		repo:               repo,
		logger:             logger,
		runID:              runID,
		worker:             worker,
		lastProviderStatus: make(map[string]db.PipelineEventStatus),
	}
}

func (r *Recorder) LoadProviderStatuses(ctx context.Context, pubkeys []string) error {
	if r.repo == nil || len(pubkeys) == 0 {
		return nil
	}
	statuses, err := r.repo.GetLastProviderPipelineEventStatus(ctx, pubkeys)
	if err != nil {
		return err
	}
	r.lastProviderStatus = statuses
	return nil
}

func (r *Recorder) RecordProviderResolve(pubkey, stage string, resolved bool, resolveErr error) {
	if r == nil || pubkey == "" {
		return
	}

	last := r.lastProviderStatus[pubkey]
	if resolved {
		if last == db.PipelineEventError {
			rc := int(constants.ValidStorageProof)
			r.appendProviderEvent(db.ProviderPipelineEvent{
				ProviderPubkey: pubkey,
				Status:         db.PipelineEventOK,
				Stage:          StageEndpointResolve,
				ReasonCode:     &rc,
				RunID:          r.runID,
				Worker:         r.worker,
			})
			r.lastProviderStatus[pubkey] = db.PipelineEventOK
		}
		return
	}

	if stage == "" {
		stage = StageEndpointResolve
	}
	msg := truncateMessage(errorText(resolveErr))
	rc := int(constants.IPNotFound)
	r.appendProviderEvent(db.ProviderPipelineEvent{
		ProviderPubkey: pubkey,
		Status:         db.PipelineEventError,
		Stage:          stage,
		ReasonCode:     &rc,
		ErrorMessage:   msg,
		RunID:          r.runID,
		Worker:         r.worker,
	})
	r.lastProviderStatus[pubkey] = db.PipelineEventError
}

func (r *Recorder) RecordBagTransition(rel db.ContractToProviderRelation, newReason constants.ReasonCode, stage, details string) {
	if r == nil {
		return
	}

	prevHadError := reasonIsError(rel.Reason)
	newHasError := IsErrorReason(newReason)

	if !prevHadError && !newHasError {
		return
	}
	if prevHadError && !newHasError {
		rc := int(constants.ValidStorageProof)
		okStage := stage
		if okStage == "" {
			okStage = StageRecovered
		}
		r.appendBagEvent(db.BagPipelineEvent{
			ProviderPubkey:  rel.ProviderPublicKey,
			ContractAddress: rel.Address,
			BagID:           rel.BagID,
			Status:          db.PipelineEventOK,
			Stage:           okStage,
			ReasonCode:      &rc,
			RunID:           r.runID,
			Worker:          r.worker,
		})
		return
	}
	if newHasError {
		if stage == "" {
			stage = StageUnknown
		}
		rc := int(newReason)
		var msg *string
		if strings.TrimSpace(details) != "" {
			msg = truncateMessage(&details)
		}
		r.appendBagEvent(db.BagPipelineEvent{
			ProviderPubkey:  rel.ProviderPublicKey,
			ContractAddress: rel.Address,
			BagID:           rel.BagID,
			Status:          db.PipelineEventError,
			Stage:           stage,
			ReasonCode:      &rc,
			ErrorMessage:    msg,
			RunID:           r.runID,
			Worker:          r.worker,
		})
	}
}

func (r *Recorder) RecordBagEndpointUnresolved(rel db.ContractToProviderRelation) {
	r.RecordBagTransition(rel, constants.IPNotFound, StageEndpointUnresolved, "storage endpoint not resolved for RunChecks")
}

func IsErrorReason(code constants.ReasonCode) bool {
	return code != constants.ValidStorageProof
}

func reasonIsError(reason *int) bool {
	if reason == nil {
		return false
	}
	return *reason != int(constants.ValidStorageProof)
}

func ParseStageFromDetails(details string) string {
	details = strings.TrimSpace(details)
	if details == "" {
		return StageUnknown
	}
	for _, part := range strings.Fields(details) {
		if strings.HasPrefix(part, "stage=") {
			stage := strings.TrimPrefix(part, "stage=")
			if stage != "" {
				return stage
			}
		}
	}
	return StageUnknown
}

func (r *Recorder) Flush(ctx context.Context) {
	if r == nil || r.repo == nil {
		return
	}

	if len(r.providerEvents) > 0 {
		if err := r.repo.InsertProviderPipelineEvents(ctx, r.providerEvents); err != nil {
			r.logger.Warn("failed to insert provider pipeline events", "error", err, "count", len(r.providerEvents))
		}
		r.providerEvents = nil
	}

	if len(r.bagEvents) > 0 {
		if err := r.repo.InsertBagPipelineEvents(ctx, r.bagEvents); err != nil {
			r.logger.Warn("failed to insert bag pipeline events", "error", err, "count", len(r.bagEvents))
		}
		r.bagEvents = nil
	}
}

func (r *Recorder) appendProviderEvent(event db.ProviderPipelineEvent) {
	r.providerEvents = append(r.providerEvents, event)
}

func (r *Recorder) appendBagEvent(event db.BagPipelineEvent) {
	r.bagEvents = append(r.bagEvents, event)
}

func truncateMessage(msg *string) *string {
	if msg == nil {
		return nil
	}
	s := strings.TrimSpace(*msg)
	if s == "" {
		return nil
	}
	if len(s) > maxErrorMessageLen {
		s = s[:maxErrorMessageLen]
	}
	return &s
}

func errorText(err error) *string {
	if err == nil {
		return nil
	}
	s := err.Error()
	return &s
}

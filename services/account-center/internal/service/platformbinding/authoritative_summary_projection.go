package platformbinding

import (
	"context"

	"paigram/internal/model"
)

func (s *OrchestrationService) applyAuthoritativeSummary(ctx context.Context, operationID, operationKind string, binding *model.PlatformAccountBinding, expectedGeneration uint64, summary *RuntimeSummary) (*model.PlatformAccountBinding, error) {
	if binding == nil || !validOperationSummary(operationKind, expectedGeneration, summary) {
		s.markCredentialInvariantViolation(ctx, operationID, "terminal_summary_mismatch")
		return nil, ErrBindingGenerationConflict
	}
	if err := s.markCredentialProjectionPending(ctx, operationID); err != nil {
		return nil, err
	}
	updated, err := s.bindingReader.PersistRuntimeSummary(binding.ID, *summary)
	if err != nil {
		return nil, err
	}
	if err := s.syncProfiles(binding, updated, summary); err != nil {
		return nil, err
	}
	if err := s.completeCredentialOperation(ctx, operationID); err != nil {
		return nil, err
	}
	return updated, nil
}

func validOperationSummary(operationKind string, expectedGeneration uint64, summary *RuntimeSummary) bool {
	if summary == nil || summary.Generation != expectedGeneration {
		return false
	}
	if summary.ProfileSnapshotComplete {
		return summary.ProfileObservedRevision >= summary.ProfileRevision
	}
	return operationKind == "OPERATION_KIND_REFRESH_CREDENTIAL"
}

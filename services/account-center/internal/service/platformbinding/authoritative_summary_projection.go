package platformbinding

import (
	"context"

	"paigram/internal/model"
)

func (s *OrchestrationService) applyAuthoritativeSummary(ctx context.Context, operationID, operationKind string, binding *model.PlatformAccountBinding, expectedGeneration, expectedProfileRevision uint64, expectedProfileRef string, summary *RuntimeSummary) (*model.PlatformAccountBinding, error) {
	if binding == nil || !validOperationSummary(operationKind, expectedGeneration, expectedProfileRevision, expectedProfileRef, summary) {
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
	if s.profileSyncer != nil {
		updated, err = s.bindingReader.GetBindingByID(binding.ID)
		if err != nil {
			return nil, err
		}
	}
	if err := s.completeCredentialOperation(ctx, operationID); err != nil {
		return nil, err
	}
	return updated, nil
}

func validOperationSummary(operationKind string, expectedGeneration, expectedProfileRevision uint64, expectedProfileRef string, summary *RuntimeSummary) bool {
	if summary == nil || summary.Generation != expectedGeneration {
		return false
	}
	if summary.ProfileSnapshotComplete {
		if summary.ProfileObservedRevision < summary.ProfileRevision {
			return false
		}
		return operationKind != "OPERATION_KIND_SET_PRIMARY_PROFILE" ||
			(summary.ProfileRevision == expectedProfileRevision+1 && summaryHasExactPrimary(summary, expectedProfileRef))
	}
	return operationKind == "OPERATION_KIND_REFRESH_CREDENTIAL"
}

func summaryHasExactPrimary(summary *RuntimeSummary, expectedProfileRef string) bool {
	if summary == nil || expectedProfileRef == "" {
		return false
	}
	primaryCount := 0
	for _, profile := range summary.Profiles {
		if !mapBool(profile["is_default"]) {
			continue
		}
		primaryCount++
		if mapString(profile["profile_ref"]) != expectedProfileRef {
			return false
		}
	}
	return primaryCount == 1
}

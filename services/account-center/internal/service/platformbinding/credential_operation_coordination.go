package platformbinding

import (
	"context"
	"errors"
	"time"

	platformv2 "github.com/PaiGramTeam/paigram-account-center/contracts/gen/go/platform/v2"
	"github.com/PaiGramTeam/paigram-account-center/contracts/runtime/go/operationid"
	"google.golang.org/grpc/codes"
	grpcstatus "google.golang.org/grpc/status"

	"paigram/internal/model"
)

func newCredentialOperationReference(binding *model.PlatformAccountBinding, operationID string, replace bool) CredentialOperationReference {
	kind := platformv2.OperationKind_OPERATION_KIND_BIND_CREDENTIAL
	if replace {
		kind = platformv2.OperationKind_OPERATION_KIND_REPLACE_CREDENTIAL
	}
	return newCredentialOperationReferenceForKind(binding, operationID, kind)
}

func newCredentialOperationReferenceForKind(binding *model.PlatformAccountBinding, operationID string, kind platformv2.OperationKind) CredentialOperationReference {
	preGeneration := binding.Generation
	targetGeneration := preGeneration + 1
	return CredentialOperationReference{
		OperationID: operationID, Kind: kind.String(), BindingRef: binding.BindingRef,
		PreGeneration: preGeneration, TargetGeneration: targetGeneration,
		RequestFingerprint: operationid.Fingerprint(kind.String(), binding.BindingRef, preGeneration, targetGeneration),
	}
}

func (s *OrchestrationService) admitCredentialOperation(ctx context.Context, binding *model.PlatformAccountBinding, input PutCredentialInput, reference CredentialOperationReference) error {
	if s.operationIntents == nil {
		return nil
	}
	_, err := s.operationIntents.Admit(ctx, CredentialOperationIntentInput{
		OperationID: reference.OperationID, BindingID: binding.ID, BindingRef: reference.BindingRef, Kind: reference.Kind,
		PreGeneration: reference.PreGeneration, TargetGeneration: reference.TargetGeneration, RequestFingerprint: reference.RequestFingerprint,
		ActorType: input.ActorType, ActorID: input.ActorID,
	})
	return err
}

func (s *OrchestrationService) admitNonSensitiveCredentialOperation(ctx context.Context, binding *model.PlatformAccountBinding, actorType, actorID string, reference CredentialOperationReference) error {
	if s.operationIntents == nil {
		return nil
	}
	_, err := s.operationIntents.Admit(ctx, credentialOperationIntentInput(binding.ID, actorType, actorID, reference))
	return err
}

func credentialOperationIntentInput(bindingID uint64, actorType, actorID string, reference CredentialOperationReference) CredentialOperationIntentInput {
	return CredentialOperationIntentInput{
		OperationID: reference.OperationID, BindingID: bindingID, BindingRef: reference.BindingRef, Kind: reference.Kind,
		PreGeneration: reference.PreGeneration, TargetGeneration: reference.TargetGeneration, RequestFingerprint: reference.RequestFingerprint,
		ActorType: actorType, ActorID: actorID,
	}
}

func (s *OrchestrationService) handleNonSensitiveCredentialDeliveryError(ctx context.Context, binding *model.PlatformAccountBinding, reference CredentialOperationReference, deliveryErr error) error {
	if s.operationIntents == nil {
		return deliveryErr
	}
	if IsCredentialOperationOutcomeUnknown(deliveryErr) {
		_ = s.operationIntents.MarkUncertain(ctx, reference.OperationID, "delivery_outcome_unknown")
		_ = s.operationIntents.Reschedule(ctx, reference.OperationID, "delivery_outcome_unknown", time.Now().UTC().Add(credentialOperationRetryDelay))
		return &CredentialOperationPendingError{OperationID: reference.OperationID, BindingID: binding.ID, State: model.PlatformOperationIntentStateUncertain}
	}
	if reference.Kind == "OPERATION_KIND_DELETE_CREDENTIAL" {
		if _, err := s.bindingReader.UpdateBindingFailure(binding.ID, model.PlatformAccountBindingStatusDeleteFailed, reasonCode(deliveryErr), deliveryErr.Error()); err != nil {
			_ = s.operationIntents.MarkProjectionPending(ctx, reference.OperationID, "delete_failure_projection_pending")
			_ = s.operationIntents.Reschedule(ctx, reference.OperationID, "delete_failure_projection_pending", time.Now().UTC().Add(credentialOperationRetryDelay))
			return errors.Join(deliveryErr, err)
		}
	}
	_ = s.operationIntents.MarkFailed(ctx, reference.OperationID, reasonCode(deliveryErr))
	return deliveryErr
}

func (s *OrchestrationService) handleCredentialDeliveryError(ctx context.Context, binding *model.PlatformAccountBinding, reference CredentialOperationReference, deliveryErr error) error {
	if s.operationIntents == nil {
		return s.handlePutCredentialError(binding, deliveryErr)
	}
	if !IsCredentialOperationOutcomeUnknown(deliveryErr) {
		mapped := s.handlePutCredentialError(binding, deliveryErr)
		if reference.Kind == "OPERATION_KIND_BIND_CREDENTIAL" && (IsCredentialValidationError(deliveryErr) || isCredentialOwnershipConflict(deliveryErr)) {
			if _, err := s.bindingReader.DeleteBinding(binding.ID); err != nil {
				_ = s.operationIntents.MarkInvariantViolation(ctx, reference.OperationID, "failed_bind_projection_delete_failed")
				return errors.Join(mapped, err)
			}
		}
		if reference.Kind == "OPERATION_KIND_REPLACE_CREDENTIAL" && IsCredentialValidationError(deliveryErr) {
			if _, err := s.bindingReader.UpdateBindingFailure(binding.ID, model.PlatformAccountBindingStatusCredentialInvalid, "credential_validation_failed", deliveryErr.Error()); err != nil {
				_ = s.operationIntents.MarkProjectionPending(ctx, reference.OperationID, "replace_failure_projection_pending")
				return errors.Join(mapped, err)
			}
		}
		_ = s.operationIntents.MarkFailed(ctx, reference.OperationID, reasonCode(deliveryErr))
		if isCredentialOwnershipConflict(deliveryErr) {
			return ErrBindingAlreadyOwned
		}
		return mapped
	}
	state := model.PlatformOperationIntentStateUncertain
	if err := s.operationIntents.MarkUncertain(ctx, reference.OperationID, "delivery_outcome_unknown"); err != nil {
		state = model.PlatformOperationIntentStatePendingDelivery
	}
	return &CredentialOperationPendingError{OperationID: reference.OperationID, BindingID: binding.ID, State: state}
}

func isCredentialOwnershipConflict(err error) bool {
	return grpcstatus.Code(err) == codes.AlreadyExists
}

func (s *OrchestrationService) markCredentialProjectionPending(ctx context.Context, operationID string) error {
	if s.operationIntents == nil {
		return nil
	}
	if err := s.operationIntents.MarkProjectionPending(ctx, operationID, "projection_pending"); err != nil {
		intent, getErr := s.operationIntents.Get(ctx, operationID)
		if getErr != nil {
			return err
		}
		return &CredentialOperationPendingError{OperationID: operationID, BindingID: intent.BindingID, State: intent.State}
	}
	return nil
}

func (s *OrchestrationService) completeCredentialOperation(ctx context.Context, operationID string) error {
	if s.operationIntents == nil {
		return nil
	}
	if err := s.operationIntents.MarkSucceeded(ctx, operationID); err != nil {
		intent, getErr := s.operationIntents.Get(ctx, operationID)
		if getErr != nil {
			return err
		}
		return &CredentialOperationPendingError{OperationID: operationID, BindingID: intent.BindingID, State: intent.State}
	}
	return nil
}

func (s *OrchestrationService) failCredentialOperation(ctx context.Context, operationID, reason string) {
	if s.operationIntents != nil {
		_ = s.operationIntents.MarkFailed(ctx, operationID, reason)
	}
}

func (s *OrchestrationService) markCredentialInvariantViolation(ctx context.Context, operationID, reason string) {
	if s.operationIntents != nil {
		_ = s.operationIntents.MarkInvariantViolation(ctx, operationID, reason)
	}
}

func IsCredentialOperationOutcomeUnknown(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	if status, ok := grpcstatus.FromError(err); ok {
		switch status.Code() {
		case codes.Canceled, codes.Unknown, codes.DeadlineExceeded, codes.Unavailable:
			return true
		}
	}
	return IsExecutionPlaneUnavailableError(err)
}

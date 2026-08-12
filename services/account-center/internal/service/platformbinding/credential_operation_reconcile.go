package platformbinding

import (
	"context"
	"errors"
	"time"

	"github.com/PaiGramTeam/paigram-account-center/contracts/runtime/go/operationid"
	"github.com/PaiGramTeam/paigram-account-center/contracts/runtime/go/platformaction"

	"paigram/internal/model"
)

const credentialOperationRetryDelay = 30 * time.Second

func (s *OrchestrationService) ReconcileCredentialOperation(ctx context.Context, operationID string) error {
	if s.operationIntents == nil || s.gateway == nil {
		return ErrCredentialGatewayUnavailable
	}
	resolver, ok := s.gateway.(credentialOperationResolver)
	if !ok {
		return ErrCredentialGatewayUnavailable
	}
	intent, err := s.operationIntents.Get(ctx, operationID)
	if err != nil {
		return err
	}
	if intent.State == model.PlatformOperationIntentStateInputRequired {
		if intent.InputExpiresAt != nil && !time.Now().UTC().Before(*intent.InputExpiresAt) {
			return s.operationIntents.ExpireInputRequired(ctx, intent.OperationID, time.Now().UTC())
		}
		if intent.InputExpiresAt != nil {
			return s.operationIntents.Reschedule(ctx, intent.OperationID, "credential_input_required", *intent.InputExpiresAt)
		}
		return s.operationIntents.MarkInputRequired(ctx, intent.OperationID, "credential_resubmission_required")
	}
	if intent.State == model.PlatformOperationIntentStateInvariantViolation {
		return s.operationIntents.Reschedule(ctx, intent.OperationID, "manual_invariant_reconciliation_required", time.Now().UTC().Add(time.Hour))
	}
	if !intent.State.ReservesBinding() {
		return nil
	}
	binding, err := s.bindingReader.GetBindingByID(intent.BindingID)
	if err != nil {
		if loader, ok := s.bindingReader.(interface {
			GetBindingByIDUnscoped(uint64) (*model.PlatformAccountBinding, error)
		}); ok {
			binding, err = loader.GetBindingByIDUnscoped(intent.BindingID)
		}
		if err != nil {
			return err
		}
	}
	if binding.DeletedAt.Valid && intent.Kind == "OPERATION_KIND_DELETE_CREDENTIAL" {
		if err := s.operationIntents.MarkProjectionPending(ctx, intent.OperationID, "projection_already_deleted"); err != nil {
			return err
		}
		return s.operationIntents.MarkSucceeded(ctx, intent.OperationID)
	}
	platformRow, err := s.platformService.GetEnabledPlatform(binding.Platform)
	if err != nil {
		return s.rescheduleCredentialOperation(ctx, intent.OperationID, "platform_service_unavailable", err)
	}
	operationBinding := bindingAtGeneration(binding, intent.PreGeneration)
	if intent.State == model.PlatformOperationIntentStatePendingDelivery && isNonSensitiveCredentialOperation(intent.Kind) {
		return s.deliverNonSensitiveCredentialOperation(ctx, intent, operationBinding, platformRow.ControlEndpoint)
	}
	ticket, _, err := s.platformService.IssueBindingScopedOperationTicket(intent.ActorType, intent.ActorID, operationBinding, intent.OperationID, []string{platformaction.MihomoOperationResolve})
	if err != nil {
		return s.rescheduleCredentialOperation(ctx, intent.OperationID, "ticket_issue_failed", err)
	}

	resolution, err := resolver.ResolveCredentialOperation(ctx, platformRow.ControlEndpoint, ticket, credentialOperationReferenceFromIntent(intent))
	if err != nil {
		if IsCredentialOperationOutcomeUnknown(err) {
			_ = s.operationIntents.MarkUncertain(ctx, intent.OperationID, "resolve_outcome_unknown")
			return s.rescheduleCredentialOperation(ctx, intent.OperationID, "resolve_outcome_unknown", err)
		}
		_ = s.operationIntents.MarkInvariantViolation(ctx, intent.OperationID, "resolve_rejected")
		return err
	}
	if resolution == nil {
		_ = s.operationIntents.MarkInvariantViolation(ctx, intent.OperationID, "resolve_result_missing")
		return errGenericCredentialSummaryRequired
	}

	switch resolution.State {
	case CredentialRemoteOperationPending:
		_ = s.operationIntents.MarkUncertain(ctx, intent.OperationID, "remote_operation_pending")
		return s.operationIntents.Reschedule(ctx, intent.OperationID, "remote_operation_pending", time.Now().UTC().Add(credentialOperationRetryDelay))
	case CredentialRemoteOperationSucceeded:
		if intent.Kind == "OPERATION_KIND_DELETE_CREDENTIAL" {
			return s.finalizeNonSensitiveCredentialOperation(ctx, intent, binding)
		}
		return s.applyResolvedCredentialOperation(ctx, intent, binding, resolution.Summary, platformRow.ControlEndpoint)
	case CredentialRemoteOperationFailed:
		reason := nonemptyReason(resolution.ReasonCode, "remote_operation_failed")
		if err := s.persistTerminalOperationFailure(intent, binding, reason); err != nil {
			return s.rescheduleCredentialOperation(ctx, intent.OperationID, "failure_projection_pending", err)
		}
		if err := s.operationIntents.MarkFailed(ctx, intent.OperationID, reason); err != nil {
			return err
		}
		return nil
	case CredentialRemoteOperationNotReceived, CredentialRemoteOperationFailedInputRequired:
		if isNonSensitiveCredentialOperation(intent.Kind) {
			return s.retryNonSensitiveCredentialOperationAfterProof(ctx, resolver, platformRow.ControlEndpoint, intent, operationBinding)
		}
		return s.confirmCredentialInputRequired(ctx, resolver, platformRow.ControlEndpoint, intent, operationBinding, binding)
	default:
		_ = s.operationIntents.MarkInvariantViolation(ctx, intent.OperationID, "unsupported_remote_operation_state")
		return errGenericCredentialOperationFailed
	}
}

func (s *OrchestrationService) deliverNonSensitiveCredentialOperation(ctx context.Context, intent *model.PlatformOperationIntent, binding *model.PlatformAccountBinding, endpoint string) error {
	var scope string
	switch intent.Kind {
	case "OPERATION_KIND_REFRESH_CREDENTIAL":
		scope = platformaction.MihomoCredentialRefresh
	case "OPERATION_KIND_DELETE_CREDENTIAL":
		scope = platformaction.MihomoCredentialDelete
	case "OPERATION_KIND_SET_PRIMARY_PROFILE":
		scope = platformaction.MihomoProfileWrite
	default:
		return ErrInvalidBindingMutation
	}
	var ticket string
	var err error
	if intent.Kind == "OPERATION_KIND_SET_PRIMARY_PROFILE" {
		ticket, _, err = s.platformService.IssueProfileScopedOperationTicket(intent.ActorType, intent.ActorID, binding, intent.ProfileRef, intent.OperationID, []string{scope})
	} else {
		ticket, _, err = s.platformService.IssueBindingScopedOperationTicket(intent.ActorType, intent.ActorID, binding, intent.OperationID, []string{scope})
	}
	if err != nil {
		return s.rescheduleCredentialOperation(ctx, intent.OperationID, "ticket_issue_failed", err)
	}
	switch intent.Kind {
	case "OPERATION_KIND_REFRESH_CREDENTIAL":
		var summary *RuntimeSummary
		summary, err = s.gateway.RefreshCredential(ctx, endpoint, ticket, intent.OperationID, binding)
		if err == nil {
			_, err = s.applyAuthoritativeSummary(ctx, intent.OperationID, intent.Kind, binding, intent.TargetGeneration, 0, "", summary)
			return err
		}
	case "OPERATION_KIND_DELETE_CREDENTIAL":
		err = s.gateway.DeleteCredential(ctx, endpoint, ticket, intent.OperationID, binding)
	case "OPERATION_KIND_SET_PRIMARY_PROFILE":
		var summary *RuntimeSummary
		operationBinding := bindingAtProfileRevision(binding, intent.ProfileRevision)
		summary, err = s.gateway.SetPrimaryProfile(ctx, endpoint, ticket, intent.OperationID, operationBinding, intent.ProfileRef)
		if err == nil {
			_, err = s.applyAuthoritativeSummary(ctx, intent.OperationID, intent.Kind, binding, intent.TargetGeneration, intent.ProfileRevision, intent.ProfileRef, summary)
			return err
		}
	}
	if err != nil {
		return s.handleNonSensitiveCredentialDeliveryError(ctx, binding, credentialOperationReferenceFromIntent(intent), err)
	}
	return s.finalizeNonSensitiveCredentialOperation(ctx, intent, binding)
}

func (s *OrchestrationService) finalizeNonSensitiveCredentialOperation(ctx context.Context, intent *model.PlatformOperationIntent, binding *model.PlatformAccountBinding) error {
	if err := s.operationIntents.MarkProjectionPending(ctx, intent.OperationID, "projection_pending"); err != nil {
		return err
	}
	switch intent.Kind {
	case "OPERATION_KIND_DELETE_CREDENTIAL":
		if _, err := s.bindingReader.DeleteBinding(binding.ID); err != nil {
			return err
		}
	default:
		return ErrInvalidBindingMutation
	}
	return s.operationIntents.MarkSucceeded(ctx, intent.OperationID)
}

func (s *OrchestrationService) retryNonSensitiveCredentialOperationAfterProof(ctx context.Context, resolver credentialOperationResolver, endpoint string, intent *model.PlatformOperationIntent, operationBinding *model.PlatformAccountBinding) error {
	ticket, _, err := s.platformService.IssueBindingScopedTicket(intent.ActorType, intent.ActorID, operationBinding, []string{platformaction.MihomoBindingRead})
	if err != nil {
		return s.rescheduleCredentialOperation(ctx, intent.OperationID, "binding_state_ticket_failed", err)
	}
	state, err := resolver.GetCredentialBindingState(ctx, endpoint, ticket, intent.BindingRef)
	if err != nil {
		return s.rescheduleCredentialOperation(ctx, intent.OperationID, "binding_state_unavailable", err)
	}
	if !credentialStateAllowsNonSensitiveRetry(intent, state) {
		if intent.Kind == "OPERATION_KIND_DELETE_CREDENTIAL" && state != nil && !state.Exists {
			if _, err := s.bindingReader.DeleteBinding(intent.BindingID); err != nil && !errors.Is(err, ErrBindingNotFound) {
				return s.rescheduleCredentialOperation(ctx, intent.OperationID, "delete_projection_pending", err)
			}
			if err := s.operationIntents.MarkProjectionPending(ctx, intent.OperationID, "delete_already_absent"); err != nil {
				return err
			}
			return s.operationIntents.MarkSucceeded(ctx, intent.OperationID)
		}
		_ = s.operationIntents.MarkInvariantViolation(ctx, intent.OperationID, "operation_terminal_state_inconsistent")
		return ErrBindingGenerationConflict
	}
	newOperationID, err := operationid.NewID()
	if err != nil {
		return err
	}
	reference := credentialOperationReferenceFromIntent(intent)
	reference.OperationID = newOperationID
	retryInput := credentialOperationIntentInput(intent.BindingID, intent.ActorType, intent.ActorID, reference)
	retryInput.ProfileRef = intent.ProfileRef
	retryInput.ProfileRevision = intent.ProfileRevision
	_, err = s.operationIntents.RetryNonSensitive(ctx, intent.OperationID, retryInput)
	return err
}

func credentialStateAllowsNonSensitiveRetry(intent *model.PlatformOperationIntent, state *CredentialBindingState) bool {
	if intent == nil || state == nil || !state.Exists || state.Summary == nil {
		return false
	}
	return state.Summary.Generation == intent.PreGeneration
}

func isNonSensitiveCredentialOperation(kind string) bool {
	return kind == "OPERATION_KIND_REFRESH_CREDENTIAL" || kind == "OPERATION_KIND_DELETE_CREDENTIAL" || kind == "OPERATION_KIND_SET_PRIMARY_PROFILE"
}

func (s *OrchestrationService) applyResolvedCredentialOperation(ctx context.Context, intent *model.PlatformOperationIntent, binding *model.PlatformAccountBinding, summary *RuntimeSummary, endpoint string) error {
	if !validOperationSummary(intent.Kind, intent.TargetGeneration, intent.ProfileRevision, intent.ProfileRef, summary) {
		_ = s.operationIntents.MarkInvariantViolation(ctx, intent.OperationID, "terminal_generation_mismatch")
		return ErrBindingGenerationConflict
	}
	if err := s.operationIntents.MarkProjectionPending(ctx, intent.OperationID, "projection_pending"); err != nil {
		return err
	}
	updatedBinding, err := s.bindingReader.PersistRuntimeSummary(binding.ID, *summary)
	if err != nil {
		if errors.Is(err, ErrBindingAlreadyOwned) {
			cleanupErr := s.compensateDeleteCredential(ctx, binding, summary.PlatformAccountID, summary.Generation, intent.ActorType, intent.ActorID, endpoint)
			if cleanupErr != nil {
				_ = s.operationIntents.MarkInvariantViolation(ctx, intent.OperationID, "compensation_delete_failed")
				return cleanupErr
			}
			if _, deleteErr := s.bindingReader.DeleteBinding(binding.ID); deleteErr != nil {
				_ = s.operationIntents.MarkInvariantViolation(ctx, intent.OperationID, "duplicate_owner_projection_delete_failed")
				return deleteErr
			}
			_ = s.operationIntents.MarkFailed(ctx, intent.OperationID, "duplicate_owner")
		}
		return err
	}
	if err := s.syncProfiles(binding, updatedBinding, summary); err != nil {
		return err
	}
	return s.operationIntents.MarkSucceeded(ctx, intent.OperationID)
}

func (s *OrchestrationService) confirmCredentialInputRequired(ctx context.Context, resolver credentialOperationResolver, endpoint string, intent *model.PlatformOperationIntent, operationBinding, localBinding *model.PlatformAccountBinding) error {
	ticket, _, err := s.platformService.IssueBindingScopedTicket(intent.ActorType, intent.ActorID, operationBinding, []string{platformaction.MihomoBindingRead})
	if err != nil {
		return s.rescheduleCredentialOperation(ctx, intent.OperationID, "binding_state_ticket_failed", err)
	}
	state, err := resolver.GetCredentialBindingState(ctx, endpoint, ticket, intent.BindingRef)
	if err != nil {
		return s.rescheduleCredentialOperation(ctx, intent.OperationID, "binding_state_unavailable", err)
	}
	if credentialStateAllowsResubmission(intent, state) {
		if _, err := s.bindingReader.UpdateBindingFailure(localBinding.ID, model.PlatformAccountBindingStatusCredentialInvalid, "credential_input_required", "credential must be submitted again"); err != nil {
			return s.rescheduleCredentialOperation(ctx, intent.OperationID, "input_projection_pending", err)
		}
		if err := s.operationIntents.MarkInputRequired(ctx, intent.OperationID, "credential_resubmission_required"); err != nil {
			return err
		}
		return nil
	}
	if state != nil && state.Exists && state.Summary != nil && state.Summary.Generation == intent.TargetGeneration {
		_ = s.operationIntents.MarkInvariantViolation(ctx, intent.OperationID, "terminal_result_missing_at_target_generation")
		return ErrBindingGenerationConflict
	}
	if _, err := s.bindingReader.UpdateBindingFailure(localBinding.ID, model.PlatformAccountBindingStatusCredentialInvalid, "operation_invariant_violation", "platform operation state requires manual reconciliation"); err != nil {
		return s.rescheduleCredentialOperation(ctx, intent.OperationID, "invariant_projection_pending", err)
	}
	_ = s.operationIntents.MarkInvariantViolation(ctx, intent.OperationID, "operation_terminal_state_inconsistent")
	return ErrBindingGenerationConflict
}

func (s *OrchestrationService) persistTerminalOperationFailure(intent *model.PlatformOperationIntent, binding *model.PlatformAccountBinding, reason string) error {
	if intent == nil || binding == nil {
		return ErrInvalidBindingMutation
	}
	switch intent.Kind {
	case "OPERATION_KIND_BIND_CREDENTIAL":
		_, err := s.bindingReader.DeleteBinding(binding.ID)
		if errors.Is(err, ErrBindingNotFound) {
			return nil
		}
		return err
	case "OPERATION_KIND_REPLACE_CREDENTIAL":
		_, err := s.bindingReader.UpdateBindingFailure(binding.ID, model.PlatformAccountBindingStatusCredentialInvalid, reason, "platform credential operation failed")
		return err
	case "OPERATION_KIND_REFRESH_CREDENTIAL":
		_, err := s.bindingReader.UpdateBindingFailure(binding.ID, model.PlatformAccountBindingStatusRefreshRequired, reason, "platform credential refresh failed")
		return err
	case "OPERATION_KIND_DELETE_CREDENTIAL":
		_, err := s.bindingReader.UpdateBindingFailure(binding.ID, model.PlatformAccountBindingStatusDeleteFailed, reason, "platform credential delete failed")
		return err
	case "OPERATION_KIND_SET_PRIMARY_PROFILE":
		return nil
	default:
		return ErrInvalidBindingMutation
	}
}

func credentialStateAllowsResubmission(intent *model.PlatformOperationIntent, state *CredentialBindingState) bool {
	if intent == nil || state == nil {
		return false
	}
	switch intent.Kind {
	case "OPERATION_KIND_BIND_CREDENTIAL":
		return !state.Exists
	case "OPERATION_KIND_REPLACE_CREDENTIAL":
		return state.Exists && state.Summary != nil && state.Summary.Generation == intent.PreGeneration
	default:
		return false
	}
}

func credentialOperationReferenceFromIntent(intent *model.PlatformOperationIntent) CredentialOperationReference {
	return CredentialOperationReference{
		OperationID: intent.OperationID, Kind: intent.Kind, BindingRef: intent.BindingRef,
		PreGeneration: intent.PreGeneration, TargetGeneration: intent.TargetGeneration, RequestFingerprint: intent.RequestFingerprint,
	}
}

func bindingAtGeneration(binding *model.PlatformAccountBinding, generation uint64) *model.PlatformAccountBinding {
	if binding == nil {
		return nil
	}
	clone := *binding
	clone.Generation = generation
	return &clone
}

func bindingAtProfileRevision(binding *model.PlatformAccountBinding, revision uint64) *model.PlatformAccountBinding {
	if binding == nil {
		return nil
	}
	clone := *binding
	clone.ProfileRevision = revision
	return &clone
}

func (s *OrchestrationService) rescheduleCredentialOperation(ctx context.Context, operationID, reason string, cause error) error {
	if err := s.operationIntents.Reschedule(ctx, operationID, reason, time.Now().UTC().Add(credentialOperationRetryDelay)); err != nil {
		return err
	}
	return cause
}

func nonemptyReason(value, fallback string) string {
	if value != "" {
		return value
	}
	return fallback
}

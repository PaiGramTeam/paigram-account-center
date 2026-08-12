package platformbinding

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	platformv2 "github.com/PaiGramTeam/paigram-account-center/contracts/gen/go/platform/v2"
	"github.com/PaiGramTeam/paigram-account-center/contracts/runtime/go/operationid"
	"github.com/PaiGramTeam/paigram-account-center/contracts/runtime/go/platformaction"
	"gorm.io/gorm"
	"paigram/internal/model"
	serviceaudit "paigram/internal/service/audit"
)

type orchestrationBindingReader interface {
	CreateBinding(input CreateBindingInput) (*model.PlatformAccountBinding, error)
	DeleteBinding(bindingID uint64) (*model.PlatformAccountBinding, error)
	GetBindingByID(bindingID uint64) (*model.PlatformAccountBinding, error)
	GetBindingForOwner(ownerUserID, bindingID uint64) (*model.PlatformAccountBinding, error)
	PersistRuntimeSummary(bindingID uint64, summary RuntimeSummary) (*model.PlatformAccountBinding, error)
	UpdateBindingStatus(bindingID uint64, status model.PlatformAccountBindingStatus) (*model.PlatformAccountBinding, error)
	UpdateBindingFailure(bindingID uint64, status model.PlatformAccountBindingStatus, reasonCode, reasonMessage string) (*model.PlatformAccountBinding, error)
}

type orchestrationProfileSyncer interface {
	SyncProfiles(input SyncProfilesInput) ([]model.PlatformAccountProfile, error)
	DeleteProfiles(bindingID uint64) error
	GetProfile(bindingID, profileID uint64) (*model.PlatformAccountProfile, error)
}

type orchestrationGrantCleaner interface {
	DeleteGrants(bindingID uint64) error
}

type orchestrationPlatformService interface {
	GetEnabledPlatform(platformKey string) (*model.PlatformService, error)
	IssueBindingScopedTicket(actorType, actorID string, binding *model.PlatformAccountBinding, scopes []string) (string, time.Time, error)
	IssueBindingScopedOperationTicket(actorType, actorID string, binding *model.PlatformAccountBinding, operationID string, scopes []string) (string, time.Time, error)
	IssueProfileScopedOperationTicket(actorType, actorID string, binding *model.PlatformAccountBinding, profileRef, operationID string, scopes []string) (string, time.Time, error)
}

type credentialGateway interface {
	BindCredential(ctx context.Context, endpoint, ticket, operationID string, binding *model.PlatformAccountBinding, payload json.RawMessage) (map[string]any, error)
	ReplaceCredential(ctx context.Context, endpoint, ticket, operationID string, binding *model.PlatformAccountBinding, payload json.RawMessage) (map[string]any, error)
	RefreshCredential(ctx context.Context, endpoint, ticket, operationID string, binding *model.PlatformAccountBinding) (*RuntimeSummary, error)
	DeleteCredential(ctx context.Context, endpoint, ticket, operationID string, binding *model.PlatformAccountBinding) error
	SetPrimaryProfile(ctx context.Context, endpoint, ticket, operationID string, binding *model.PlatformAccountBinding, profileRef string) (*RuntimeSummary, error)
}

type OrchestrationService struct {
	bindingReader    orchestrationBindingReader
	platformService  orchestrationPlatformService
	gateway          credentialGateway
	operationIntents credentialOperationIntentStore
	profileSyncer    orchestrationProfileSyncer
	grantCleaner     orchestrationGrantCleaner
	auditWriter      orchestrationAuditWriter
}

type orchestrationAuditWriter interface {
	Record(context.Context, serviceaudit.WriteInput) error
}

func NewOrchestrationService(bindingReader orchestrationBindingReader, platformService orchestrationPlatformService, gateway credentialGateway, dependencies ...any) *OrchestrationService {
	service := &OrchestrationService{bindingReader: bindingReader, platformService: platformService, gateway: gateway}
	for _, dependency := range dependencies {
		switch typed := dependency.(type) {
		case orchestrationProfileSyncer:
			service.profileSyncer = typed
		case orchestrationGrantCleaner:
			service.grantCleaner = typed
		case orchestrationAuditWriter:
			service.auditWriter = typed
		case credentialOperationIntentStore:
			service.operationIntents = typed
		}
	}
	return service
}

func (s *OrchestrationService) CreateBindingForOwner(ctx context.Context, input CreateAndBindInput) (*model.PlatformAccountBinding, error) {
	platformRow, err := s.platformService.GetEnabledPlatform(input.Platform)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrPlatformServiceUnavailable
		}
		return nil, err
	}
	if s.operationIntents != nil {
		pending, pendingErr := s.operationIntents.FindPendingBindForOwner(ctx, input.OwnerUserID, input.Platform)
		if pendingErr != nil {
			return nil, pendingErr
		}
		if pending != nil {
			return nil, &CredentialOperationPendingError{OperationID: pending.OperationID, BindingID: pending.BindingID, State: pending.State}
		}
	}

	createInput := CreateBindingInput{
		OwnerUserID:        input.OwnerUserID,
		Platform:           input.Platform,
		PlatformServiceKey: platformRow.ServiceKey,
		DisplayName:        input.DisplayName,
	}
	var binding *model.PlatformAccountBinding
	preadmittedOperationID := ""
	if s.operationIntents != nil {
		preadmittedOperationID, err = operationid.NewID()
		if err == nil {
			binding, _, err = s.operationIntents.CreateBindingAndAdmit(ctx, createInput, input.ActorType, input.ActorID, preadmittedOperationID)
		}
	} else {
		binding, err = s.bindingReader.CreateBinding(createInput)
	}
	if err != nil {
		s.recordBindingAudit(ctx, nil, "binding_create", "failure", reasonCode(err), &input.OwnerUserID, input.ActorType, input.ActorID, map[string]any{"platform": input.Platform})
		return nil, err
	}

	putInput := PutCredentialInput{
		OwnerUserID:       input.OwnerUserID,
		BindingID:         binding.ID,
		ActorType:         input.ActorType,
		ActorID:           input.ActorID,
		CredentialPayload: input.CredentialPayload,
	}
	var updatedBinding *model.PlatformAccountBinding
	if preadmittedOperationID != "" {
		_, updatedBinding, err = s.putCredentialWithOperation(ctx, binding, putInput, preadmittedOperationID, true)
	} else {
		_, updatedBinding, err = s.putCredential(ctx, binding, putInput)
	}
	if err != nil {
		s.recordBindingAudit(ctx, binding, "binding_create", auditResult(err), reasonCode(err), &input.OwnerUserID, input.ActorType, input.ActorID, map[string]any{"platform": input.Platform})
		if updatedBinding != nil {
			if updatedBinding.ID != binding.ID {
				if _, deleteErr := s.bindingReader.DeleteBinding(binding.ID); deleteErr != nil {
					return nil, deleteErr
				}
			}
			return updatedBinding, nil
		}
		return nil, err
	}
	if updatedBinding != nil {
		s.recordBindingAudit(ctx, updatedBinding, "binding_create", "success", "", &input.OwnerUserID, input.ActorType, input.ActorID, map[string]any{"platform": input.Platform})
		if updatedBinding.ID != binding.ID {
			if _, deleteErr := s.bindingReader.DeleteBinding(binding.ID); deleteErr != nil {
				return nil, deleteErr
			}
		}
		return updatedBinding, nil
	}
	resolvedBinding, err := s.bindingReader.GetBindingForOwner(input.OwnerUserID, binding.ID)
	if err == nil {
		s.recordBindingAudit(ctx, resolvedBinding, "binding_create", "success", "", &input.OwnerUserID, input.ActorType, input.ActorID, map[string]any{"platform": input.Platform})
	}
	return resolvedBinding, err
}

func (s *OrchestrationService) PutCredentialForOwner(ctx context.Context, input PutCredentialInput) (*RuntimeSummary, error) {
	binding, err := s.bindingReader.GetBindingForOwner(input.OwnerUserID, input.BindingID)
	if err != nil {
		return nil, err
	}

	summary, _, err := s.putCredential(ctx, binding, input)
	if err != nil {
		s.recordBindingAudit(ctx, binding, "credential_update", auditResult(err), reasonCode(err), uint64Ptr(binding.OwnerUserID), input.ActorType, input.ActorID, nil)
		if errors.Is(err, ErrCredentialValidationFailed) {
			s.recordBindingAudit(ctx, binding, "platform_validation_failure", "failure", reasonCode(err), uint64Ptr(binding.OwnerUserID), input.ActorType, input.ActorID, nil)
		}
		return summary, err
	}
	s.recordBindingAudit(ctx, binding, "credential_update", "success", "", uint64Ptr(binding.OwnerUserID), input.ActorType, input.ActorID, nil)
	return summary, err
}

func (s *OrchestrationService) PutCredentialAsAdmin(ctx context.Context, input PutCredentialInput) (*RuntimeSummary, error) {
	binding, err := s.bindingReader.GetBindingByID(input.BindingID)
	if err != nil {
		return nil, err
	}

	summary, _, err := s.putCredential(ctx, binding, input)
	if err != nil {
		s.recordBindingAudit(ctx, binding, "credential_update", auditResult(err), reasonCode(err), uint64Ptr(binding.OwnerUserID), input.ActorType, input.ActorID, nil)
		if errors.Is(err, ErrCredentialValidationFailed) {
			s.recordBindingAudit(ctx, binding, "platform_validation_failure", "failure", reasonCode(err), uint64Ptr(binding.OwnerUserID), input.ActorType, input.ActorID, nil)
		}
		return summary, err
	}
	s.recordBindingAudit(ctx, binding, "credential_update", "success", "", uint64Ptr(binding.OwnerUserID), input.ActorType, input.ActorID, nil)
	return summary, err
}

func (s *OrchestrationService) RefreshBindingForOwner(ctx context.Context, ownerUserID, bindingID uint64) (*model.PlatformAccountBinding, error) {
	binding, err := s.bindingReader.GetBindingForOwner(ownerUserID, bindingID)
	if err != nil {
		return nil, err
	}

	updated, err := s.refreshBinding(ctx, binding, "user", "binding-refresh")
	if err != nil {
		s.recordBindingAudit(ctx, binding, "binding_refresh", auditResult(err), reasonCode(err), uint64Ptr(binding.OwnerUserID), "user", "binding-refresh", nil)
		return nil, err
	}
	s.recordBindingAudit(ctx, updated, "binding_refresh", "success", "", uint64Ptr(binding.OwnerUserID), "user", "binding-refresh", nil)
	return updated, nil
}

func (s *OrchestrationService) RefreshBindingAsAdmin(ctx context.Context, bindingID, adminUserID uint64) (*model.PlatformAccountBinding, error) {
	binding, err := s.bindingReader.GetBindingByID(bindingID)
	if err != nil {
		return nil, err
	}

	actorID := "admin:" + strconv.FormatUint(adminUserID, 10)
	updated, err := s.refreshBinding(ctx, binding, "admin", actorID)
	if err != nil {
		s.recordBindingAudit(ctx, binding, "binding_refresh", auditResult(err), reasonCode(err), uint64Ptr(binding.OwnerUserID), "admin", actorID, nil)
		return nil, err
	}
	s.recordBindingAudit(ctx, updated, "binding_refresh", "success", "", uint64Ptr(binding.OwnerUserID), "admin", actorID, nil)
	return updated, nil
}

func (s *OrchestrationService) DeleteBindingForOwner(ctx context.Context, ownerUserID, bindingID uint64) error {
	binding, err := s.bindingReader.GetBindingForOwner(ownerUserID, bindingID)
	if err != nil {
		return err
	}

	err = s.deleteBinding(ctx, binding, "user", "binding-delete")
	if err != nil {
		s.recordBindingAudit(ctx, binding, "binding_delete", auditResult(err), reasonCode(err), uint64Ptr(binding.OwnerUserID), "user", "binding-delete", nil)
		return err
	}
	s.recordBindingAudit(ctx, binding, "binding_delete", "success", "", uint64Ptr(binding.OwnerUserID), "user", "binding-delete", nil)
	return nil
}

func (s *OrchestrationService) DeleteBindingAsAdmin(ctx context.Context, bindingID, adminUserID uint64) error {
	binding, err := s.bindingReader.GetBindingByID(bindingID)
	if err != nil {
		return err
	}

	actorID := "admin:" + strconv.FormatUint(adminUserID, 10)
	err = s.deleteBinding(ctx, binding, "admin", actorID)
	if err != nil {
		s.recordBindingAudit(ctx, binding, "binding_delete", auditResult(err), reasonCode(err), uint64Ptr(binding.OwnerUserID), "admin", actorID, nil)
		return err
	}
	s.recordBindingAudit(ctx, binding, "binding_delete", "success", "", uint64Ptr(binding.OwnerUserID), "admin", actorID, nil)
	return nil
}

func (s *OrchestrationService) RepairDeleteFailedBinding(ctx context.Context, bindingID uint64) error {
	binding, err := s.bindingReader.GetBindingByID(bindingID)
	if err != nil {
		return err
	}
	if binding == nil || binding.Status != model.PlatformAccountBindingStatusDeleteFailed {
		return nil
	}
	return s.deleteBinding(ctx, binding, "consumer", "platform-binding-delete-repair")
}

func (s *OrchestrationService) refreshBinding(ctx context.Context, binding *model.PlatformAccountBinding, actorType, actorID string) (*model.PlatformAccountBinding, error) {
	if s.gateway == nil {
		return nil, ErrCredentialGatewayUnavailable
	}

	platformRow, err := s.platformService.GetEnabledPlatform(binding.Platform)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrPlatformServiceUnavailable
		}
		return nil, err
	}

	operationID, err := operationid.NewID()
	if err != nil {
		return nil, err
	}
	reference := newCredentialOperationReferenceForKind(binding, operationID, platformv2.OperationKind_OPERATION_KIND_REFRESH_CREDENTIAL)
	if err := s.admitNonSensitiveCredentialOperation(ctx, binding, actorType, actorID, reference); err != nil {
		return nil, err
	}
	ticket, _, err := s.platformService.IssueBindingScopedOperationTicket(actorType, actorID, binding, operationID, []string{platformaction.MihomoCredentialRefresh})
	if err != nil {
		if s.operationIntents != nil {
			_ = s.operationIntents.Reschedule(ctx, operationID, "ticket_issue_failed", time.Now().UTC().Add(credentialOperationRetryDelay))
			return nil, &CredentialOperationPendingError{OperationID: operationID, BindingID: binding.ID, State: model.PlatformOperationIntentStatePendingDelivery}
		}
		return nil, err
	}

	summary, err := s.gateway.RefreshCredential(ctx, platformRow.Endpoint, ticket, operationID, binding)
	if err != nil {
		return nil, s.handleNonSensitiveCredentialDeliveryError(ctx, binding, reference, err)
	}
	return s.applyAuthoritativeSummary(ctx, operationID, reference.Kind, binding, reference.TargetGeneration, 0, "", summary)
}

func (s *OrchestrationService) deleteBinding(ctx context.Context, binding *model.PlatformAccountBinding, actorType, actorID string) error {
	if binding == nil {
		return ErrBindingNotFound
	}
	wasCleanupFailed := binding.Status == model.PlatformAccountBindingStatusDeleteFailed && binding.StatusReasonCode == "control_plane_cleanup_failed"
	if _, err := s.bindingReader.UpdateBindingStatus(binding.ID, model.PlatformAccountBindingStatusDeleting); err != nil {
		return err
	}
	if wasCleanupFailed {
		_, err := s.bindingReader.DeleteBinding(binding.ID)
		if err != nil {
			return s.markDeleteFailed(binding.ID, err, "control_plane_cleanup_failed")
		}
		return nil
	}

	if !binding.ExternalAccountKey.Valid || binding.ExternalAccountKey.String == "" {
		_, err := s.bindingReader.DeleteBinding(binding.ID)
		if err != nil {
			return s.markDeleteFailed(binding.ID, err, "control_plane_cleanup_failed")
		}
		return nil
	}
	if s.gateway == nil {
		return s.markDeleteFailed(binding.ID, ErrCredentialGatewayUnavailable, "credential_delete_failed")
	}

	platformRow, err := s.platformService.GetEnabledPlatform(binding.Platform)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			err = ErrPlatformServiceUnavailable
		}
		return s.markDeleteFailed(binding.ID, err, "credential_delete_failed")
	}

	operationID, err := operationid.NewID()
	if err != nil {
		return s.markDeleteFailed(binding.ID, err, "credential_delete_failed")
	}
	reference := newCredentialOperationReferenceForKind(binding, operationID, platformv2.OperationKind_OPERATION_KIND_DELETE_CREDENTIAL)
	if err := s.admitNonSensitiveCredentialOperation(ctx, binding, actorType, actorID, reference); err != nil {
		return err
	}
	ticket, _, err := s.platformService.IssueBindingScopedOperationTicket(actorType, actorID, binding, operationID, []string{platformaction.MihomoCredentialDelete})
	if err != nil {
		if s.operationIntents != nil {
			_ = s.operationIntents.Reschedule(ctx, operationID, "ticket_issue_failed", time.Now().UTC().Add(credentialOperationRetryDelay))
			return &CredentialOperationPendingError{OperationID: operationID, BindingID: binding.ID, State: model.PlatformOperationIntentStatePendingDelivery}
		}
		return s.markDeleteFailed(binding.ID, err, "credential_delete_failed")
	}

	if err := s.gateway.DeleteCredential(ctx, platformRow.Endpoint, ticket, operationID, binding); err != nil {
		if s.operationIntents != nil {
			return s.handleNonSensitiveCredentialDeliveryError(ctx, binding, reference, err)
		}
		return s.markDeleteFailed(binding.ID, err, "credential_delete_failed")
	}
	if err := s.markCredentialProjectionPending(ctx, operationID); err != nil {
		return err
	}

	_, err = s.bindingReader.DeleteBinding(binding.ID)
	if err != nil {
		return s.markDeleteFailed(binding.ID, err, "control_plane_cleanup_failed")
	}
	return s.completeCredentialOperation(ctx, operationID)
}

func (s *OrchestrationService) markDeleteFailed(bindingID uint64, err error, reasonCode string) error {
	if err == nil {
		return nil
	}
	_, _ = s.bindingReader.UpdateBindingFailure(bindingID, model.PlatformAccountBindingStatusDeleteFailed, reasonCode, err.Error())
	return err
}

func (s *OrchestrationService) putCredential(ctx context.Context, binding *model.PlatformAccountBinding, input PutCredentialInput) (*RuntimeSummary, *model.PlatformAccountBinding, error) {
	return s.putCredentialWithOperation(ctx, binding, input, "", false)
}

func (s *OrchestrationService) putCredentialWithOperation(ctx context.Context, binding *model.PlatformAccountBinding, input PutCredentialInput, operationID string, alreadyAdmitted bool) (*RuntimeSummary, *model.PlatformAccountBinding, error) {
	if s.gateway == nil {
		return nil, nil, ErrCredentialGatewayUnavailable
	}

	platformRow, err := s.platformService.GetEnabledPlatform(binding.Platform)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil, ErrPlatformServiceUnavailable
		}
		return nil, nil, err
	}

	hasResolvedAccount := binding.ExternalAccountKey.Valid && binding.ExternalAccountKey.String != ""
	scopes := []string{platformaction.MihomoCredentialBind}
	if hasResolvedAccount {
		scopes = []string{platformaction.MihomoCredentialUpdate}
	}

	if operationID == "" {
		operationID, err = operationid.NewID()
		if err != nil {
			return nil, nil, err
		}
	}
	reference := newCredentialOperationReference(binding, operationID, hasResolvedAccount)
	if !alreadyAdmitted {
		if err := s.admitCredentialOperation(ctx, binding, input, reference); err != nil {
			return nil, nil, err
		}
	}
	ticket, _, err := s.platformService.IssueBindingScopedOperationTicket(input.ActorType, input.ActorID, binding, operationID, scopes)
	if err != nil {
		if reference.Kind == "OPERATION_KIND_BIND_CREDENTIAL" {
			if _, deleteErr := s.bindingReader.DeleteBinding(binding.ID); deleteErr != nil {
				s.markCredentialInvariantViolation(ctx, operationID, "ticket_failure_projection_delete_failed")
				return nil, nil, errors.Join(err, deleteErr)
			}
		}
		s.failCredentialOperation(ctx, operationID, "ticket_issue_failed")
		return nil, nil, err
	}

	var summary map[string]any
	if hasResolvedAccount {
		summary, err = s.gateway.ReplaceCredential(ctx, platformRow.Endpoint, ticket, operationID, binding, input.CredentialPayload)
	} else {
		summary, err = s.gateway.BindCredential(ctx, platformRow.Endpoint, ticket, operationID, binding, input.CredentialPayload)
	}
	if err != nil {
		return nil, nil, s.handleCredentialDeliveryError(ctx, binding, reference, err)
	}
	if err := s.markCredentialProjectionPending(ctx, reference.OperationID); err != nil {
		return nil, nil, err
	}

	runtimeSummary, err := decodeRuntimeSummary(summary)
	if err != nil {
		s.markCredentialInvariantViolation(ctx, reference.OperationID, "invalid_operation_summary")
		return nil, nil, err
	}
	updatedBinding, err := s.bindingReader.PersistRuntimeSummary(binding.ID, *runtimeSummary)
	if err != nil {
		if errors.Is(err, ErrBindingAlreadyOwned) {
			cleanupErr := s.compensateDeleteCredential(ctx, binding, runtimeSummary.PlatformAccountID, runtimeSummary.Generation, input.ActorType, input.ActorID, platformRow.Endpoint)
			if cleanupErr != nil {
				s.markCredentialInvariantViolation(ctx, reference.OperationID, "compensation_delete_failed")
				_, _ = s.bindingReader.UpdateBindingFailure(binding.ID, model.PlatformAccountBindingStatusDeleteFailed, "compensation_delete_failed", cleanupErr.Error())
				return nil, nil, fmt.Errorf("%w: cleanup failed: %v", ErrBindingAlreadyOwned, cleanupErr)
			}
			if _, deleteErr := s.bindingReader.DeleteBinding(binding.ID); deleteErr != nil {
				s.markCredentialInvariantViolation(ctx, reference.OperationID, "duplicate_owner_projection_delete_failed")
				return nil, nil, errors.Join(err, deleteErr)
			}
		}
		if errors.Is(err, ErrBindingAlreadyOwned) {
			s.failCredentialOperation(ctx, reference.OperationID, reasonCode(err))
		} else if s.operationIntents != nil {
			_ = s.operationIntents.Reschedule(ctx, reference.OperationID, "projection_persist_failed", time.Now().UTC().Add(credentialOperationRetryDelay))
		}
		return nil, nil, err
	}
	if err := s.syncProfiles(binding, updatedBinding, runtimeSummary); err != nil {
		return runtimeSummary, updatedBinding, err
	}
	if err := s.completeCredentialOperation(ctx, reference.OperationID); err != nil {
		return runtimeSummary, updatedBinding, err
	}

	return runtimeSummary, updatedBinding, nil
}

func (s *OrchestrationService) handlePutCredentialError(binding *model.PlatformAccountBinding, err error) error {
	if !IsCredentialValidationError(err) {
		return err
	}
	if binding != nil {
		_, _ = s.bindingReader.UpdateBindingFailure(binding.ID, model.PlatformAccountBindingStatusCredentialInvalid, "credential_validation_failed", err.Error())
	}
	return fmt.Errorf("%w: %v", ErrCredentialValidationFailed, err)
}

func (s *OrchestrationService) compensateDeleteCredential(ctx context.Context, binding *model.PlatformAccountBinding, resolvedAccountKey string, resolvedGeneration uint64, actorType, actorID, endpoint string) error {
	if s.gateway == nil {
		return ErrCredentialGatewayUnavailable
	}
	resolvedBinding := binding
	if binding != nil && resolvedAccountKey != "" {
		clone := *binding
		clone.ExternalAccountKey = sql.NullString{String: resolvedAccountKey, Valid: true}
		clone.Generation = resolvedGeneration
		resolvedBinding = &clone
	}

	operationID, err := operationid.NewID()
	if err != nil {
		return err
	}
	ticket, _, err := s.platformService.IssueBindingScopedOperationTicket(actorType, actorID, resolvedBinding, operationID, []string{platformaction.MihomoCredentialDelete})
	if err != nil {
		return err
	}

	return s.gateway.DeleteCredential(ctx, endpoint, ticket, operationID, resolvedBinding)
}

func (s *OrchestrationService) syncProfiles(binding, updatedBinding *model.PlatformAccountBinding, summary *RuntimeSummary) error {
	if s.profileSyncer == nil || binding == nil || summary == nil || !summary.ProfileSnapshotComplete || summary.ProfileObservedRevision < summary.ProfileRevision {
		return nil
	}
	syncedAt := time.Now().UTC()
	bindingID := binding.ID
	if updatedBinding != nil {
		bindingID = updatedBinding.ID
	}
	profiles := buildProfileProjectionInputs(binding.Platform, summary.Profiles)
	for i := range profiles {
		if !profiles[i].SourceUpdatedAt.Valid {
			profiles[i].SourceUpdatedAt = sql.NullTime{Time: syncedAt, Valid: true}
		}
	}
	_, err := s.profileSyncer.SyncProfiles(SyncProfilesInput{
		BindingID:        bindingID,
		Profiles:         profiles,
		SyncedAt:         syncedAt,
		Revision:         summary.ProfileRevision,
		ObservedRevision: summary.ProfileObservedRevision,
	})
	return err
}

func buildProfileProjectionInputs(platform string, rawProfiles []map[string]any) []ProfileProjectionInput {
	profiles := make([]ProfileProjectionInput, 0, len(rawProfiles))
	for _, raw := range rawProfiles {
		playerUID := mapString(raw["player_id"])
		gameBiz := mapString(raw["game_biz"])
		region := mapString(raw["region"])
		nickname := mapString(raw["nickname"])
		platformProfileKey := derivePlatformProfileKey(platform, raw, playerUID)
		if platformProfileKey == "" || playerUID == "" || gameBiz == "" || region == "" || nickname == "" {
			continue
		}
		profiles = append(profiles, ProfileProjectionInput{
			PlatformProfileKey: platformProfileKey,
			ProfileRef:         mapString(raw["profile_ref"]),
			GameBiz:            gameBiz,
			Region:             region,
			PlayerUID:          playerUID,
			Nickname:           nickname,
			Level:              nullableLevel(raw["level"]),
			IsPrimary:          mapBool(raw["is_default"]),
			SourceUpdatedAt:    nullableSourceUpdatedAt(raw["source_updated_at"]),
		})
	}
	return profiles
}

func nullableSourceUpdatedAt(value any) sql.NullTime {
	text, ok := value.(string)
	if !ok || text == "" {
		return sql.NullTime{}
	}
	timestamp, err := time.Parse(time.RFC3339, text)
	if err != nil {
		return sql.NullTime{}
	}
	return sql.NullTime{Time: timestamp.UTC(), Valid: true}
}

func derivePlatformProfileKey(platform string, raw map[string]any, playerUID string) string {
	if profileRef := mapString(raw["profile_ref"]); profileRef != "" {
		return profileRef
	}
	if id := mapUint64(raw["id"]); id != 0 {
		return platform + ":" + strconv.FormatUint(id, 10)
	}
	if playerUID != "" {
		return platform + ":" + playerUID
	}
	return ""
}

func mapString(value any) string {
	if text, ok := value.(string); ok {
		return text
	}
	return ""
}

func mapBool(value any) bool {
	flag, ok := value.(bool)
	return ok && flag
}

func mapUint64(value any) uint64 {
	switch v := value.(type) {
	case uint64:
		return v
	case uint32:
		return uint64(v)
	case int:
		if v > 0 {
			return uint64(v)
		}
	case int32:
		if v > 0 {
			return uint64(v)
		}
	case int64:
		if v > 0 {
			return uint64(v)
		}
	case float64:
		if v > 0 {
			return uint64(v)
		}
	}
	return 0
}

func nullableLevel(value any) sql.NullInt64 {
	switch v := value.(type) {
	case int:
		return sql.NullInt64{Int64: int64(v), Valid: true}
	case int32:
		return sql.NullInt64{Int64: int64(v), Valid: true}
	case int64:
		return sql.NullInt64{Int64: v, Valid: true}
	case float64:
		return sql.NullInt64{Int64: int64(v), Valid: true}
	}
	return sql.NullInt64{}
}

type unavailableCredentialGateway struct{}

func (unavailableCredentialGateway) BindCredential(context.Context, string, string, string, *model.PlatformAccountBinding, json.RawMessage) (map[string]any, error) {
	return nil, ErrCredentialGatewayUnavailable
}

func (unavailableCredentialGateway) ReplaceCredential(context.Context, string, string, string, *model.PlatformAccountBinding, json.RawMessage) (map[string]any, error) {
	return nil, ErrCredentialGatewayUnavailable
}

func (unavailableCredentialGateway) RefreshCredential(context.Context, string, string, string, *model.PlatformAccountBinding) (*RuntimeSummary, error) {
	return nil, ErrCredentialGatewayUnavailable
}

func (unavailableCredentialGateway) SetPrimaryProfile(context.Context, string, string, string, *model.PlatformAccountBinding, string) (*RuntimeSummary, error) {
	return nil, ErrCredentialGatewayUnavailable
}

func (unavailableCredentialGateway) DeleteCredential(context.Context, string, string, string, *model.PlatformAccountBinding) error {
	return ErrCredentialGatewayUnavailable
}

func (s *OrchestrationService) recordBindingAudit(ctx context.Context, binding *model.PlatformAccountBinding, action, result, reason string, ownerUserID *uint64, actorType, actorID string, metadata map[string]any) {
	if s == nil || s.auditWriter == nil {
		return
	}
	var bindingID *uint64
	targetID := ""
	if binding != nil {
		bindingID = &binding.ID
		targetID = strconv.FormatUint(binding.ID, 10)
		if ownerUserID == nil && binding.OwnerUserID != 0 {
			ownerUserID = uint64Ptr(binding.OwnerUserID)
		}
	}
	payload := map[string]any{"actor_id": actorID}
	for key, value := range metadata {
		payload[key] = value
	}
	_ = s.auditWriter.Record(ctx, serviceaudit.WriteInput{
		Category:    "platform_binding",
		ActorType:   actorType,
		ActorUserID: actorUserIDFromAuditContext(actorType, actorID, ownerUserID),
		Action:      action,
		TargetType:  "binding",
		TargetID:    targetID,
		BindingID:   bindingID,
		OwnerUserID: ownerUserID,
		Result:      result,
		ReasonCode:  reason,
		Metadata:    payload,
	})
}

func actorUserIDFromAuditContext(actorType, actorID string, ownerUserID *uint64) *uint64 {
	switch actorType {
	case "user":
		if ownerUserID != nil {
			return ownerUserID
		}
	case "admin":
		const prefix = "admin:"
		if strings.HasPrefix(actorID, prefix) {
			if value, err := strconv.ParseUint(strings.TrimPrefix(actorID, prefix), 10, 64); err == nil && value != 0 {
				return &value
			}
		}
	}
	return nil
}

func reasonCode(err error) string {
	if err == nil {
		return ""
	}
	switch {
	case errors.Is(err, ErrCredentialValidationFailed):
		return "credential_validation_failed"
	case errors.Is(err, ErrCredentialOperationPending):
		return "operation_pending"
	case errors.Is(err, ErrGrantPropagationPending):
		return "authorization_propagation_pending"
	case errors.Is(err, ErrCredentialGatewayUnavailable):
		return "credential_gateway_unavailable"
	case errors.Is(err, ErrPlatformServiceUnavailable):
		return "platform_service_unavailable"
	case errors.Is(err, ErrBindingNotFound):
		return "binding_not_found"
	default:
		return "operation_failed"
	}
}

func auditResult(err error) string {
	if errors.Is(err, ErrCredentialOperationPending) {
		return "pending"
	}
	return "failure"
}

func uint64Ptr(value uint64) *uint64 {
	if value == 0 {
		return nil
	}
	return &value
}

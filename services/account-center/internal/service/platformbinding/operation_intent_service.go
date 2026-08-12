package platformbinding

import (
	"context"
	"errors"
	"strconv"
	"time"

	"go.uber.org/zap"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"paigram/internal/dberror"
	"paigram/internal/model"
	serviceaudit "paigram/internal/service/audit"
)

const (
	credentialOperationInitialDeliveryLease = 15 * time.Second
	credentialOperationDispatchLease        = 2 * time.Minute
	credentialOperationInputTTL             = 24 * time.Hour
	credentialOperationDeadLetterAttempts   = 100
)

type CredentialOperationIntentInput struct {
	OperationID        string
	BindingID          uint64
	BindingRef         string
	Kind               string
	PreGeneration      uint64
	TargetGeneration   uint64
	RequestFingerprint string
	ProfileRef         string
	ProfileRevision    uint64
	ActorType          string
	ActorID            string
}

type OperationIntentService struct {
	db *gorm.DB
}

type credentialOperationIntentStore interface {
	Admit(context.Context, CredentialOperationIntentInput) (*model.PlatformOperationIntent, error)
	Get(context.Context, string) (*model.PlatformOperationIntent, error)
	FindPendingBindForOwner(context.Context, uint64, string) (*model.PlatformOperationIntent, error)
	CreateBindingAndAdmit(context.Context, CreateBindingInput, string, string, string) (*model.PlatformAccountBinding, *model.PlatformOperationIntent, error)
	RetryNonSensitive(context.Context, string, CredentialOperationIntentInput) (*model.PlatformOperationIntent, error)
	MarkUncertain(context.Context, string, string) error
	MarkProjectionPending(context.Context, string, string) error
	MarkInputRequired(context.Context, string, string) error
	MarkInvariantViolation(context.Context, string, string) error
	MarkSucceeded(context.Context, string) error
	MarkFailed(context.Context, string, string) error
	ExpireInputRequired(context.Context, string, time.Time) error
	Reschedule(context.Context, string, string, time.Time) error
	ClaimDueOperationIDs(context.Context, time.Time, int) ([]string, error)
}

func NewOperationIntentService(db *gorm.DB) *OperationIntentService {
	return &OperationIntentService{db: db}
}

func (s *OperationIntentService) Admit(ctx context.Context, input CredentialOperationIntentInput) (*model.PlatformOperationIntent, error) {
	if s == nil || s.db == nil || input.OperationID == "" || input.BindingID == 0 || input.BindingRef == "" || input.Kind == "" || input.RequestFingerprint == "" || input.ActorType == "" || input.ActorID == "" || !validIntentGeneration(input) || !validIntentProfile(input) {
		return nil, ErrInvalidBindingMutation
	}

	var admitted model.PlatformOperationIntent
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var parent struct {
			ID          uint64
			OwnerUserID uint64
			Platform    string
		}
		if err := tx.Table("platform_account_bindings").Select("id", "owner_user_id", "platform").Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", input.BindingID).Take(&parent).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrBindingNotFound
			}
			return err
		}

		var active model.PlatformOperationIntent
		err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("binding_id = ? AND state IN ?", input.BindingID, reservingOperationIntentStates()).
			Order("created_at DESC").Take(&active).Error
		if err == nil {
			if active.State != model.PlatformOperationIntentStateInputRequired {
				return &CredentialOperationPendingError{OperationID: active.OperationID, BindingID: active.BindingID, State: active.State}
			}
			now := time.Now().UTC()
			if err := tx.Model(&model.PlatformOperationIntent{}).Where("operation_id = ? AND state = ?", active.OperationID, model.PlatformOperationIntentStateInputRequired).
				Updates(map[string]any{"state": model.PlatformOperationIntentStateSuperseded, "resolved_at": now, "updated_at": now}).Error; err != nil {
				return err
			}
			if err := tx.Model(&model.PlatformOperationOutbox{}).Where("operation_id = ?", active.OperationID).
				Updates(map[string]any{"status": model.PlatformOperationOutboxStatusCompleted, "last_reason_code": "credential_resubmitted", "updated_at": now}).Error; err != nil {
				return err
			}
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}

		now := time.Now().UTC()
		admitted = model.PlatformOperationIntent{
			OperationID: input.OperationID, BindingID: input.BindingID, BindingRef: input.BindingRef,
			OwnerUserID: parent.OwnerUserID, Platform: parent.Platform,
			Kind: input.Kind, PreGeneration: input.PreGeneration, TargetGeneration: input.TargetGeneration,
			RequestFingerprint: input.RequestFingerprint, ProfileRef: input.ProfileRef, ProfileRevision: input.ProfileRevision,
			DeliveryMode: deliveryModeForOperation(input.Kind), State: model.PlatformOperationIntentStatePendingDelivery,
			ActorType: input.ActorType, ActorID: input.ActorID,
		}
		if err := tx.Create(&admitted).Error; err != nil {
			return err
		}
		if err := tx.Create(&model.PlatformOperationOutbox{
			OperationID: input.OperationID, Status: model.PlatformOperationOutboxStatusPending, AvailableAt: now.Add(credentialOperationInitialDeliveryLease),
		}).Error; err != nil {
			return err
		}
		return writeOperationAdmissionAudit(tx, &admitted)
	})
	if err != nil {
		return nil, err
	}
	return &admitted, nil
}

func (s *OperationIntentService) Get(ctx context.Context, operationID string) (*model.PlatformOperationIntent, error) {
	var intent model.PlatformOperationIntent
	err := s.db.WithContext(ctx).Where("operation_id = ?", operationID).Take(&intent).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrBindingNotFound
	}
	if err != nil {
		return nil, err
	}
	return &intent, nil
}

func (s *OperationIntentService) FindPendingBindForOwner(ctx context.Context, ownerUserID uint64, platform string) (*model.PlatformOperationIntent, error) {
	var intent model.PlatformOperationIntent
	err := s.db.WithContext(ctx).Model(&model.PlatformOperationIntent{}).
		Joins("JOIN platform_account_bindings ON platform_account_bindings.id = platform_operation_intents.binding_id").
		Where("platform_operation_intents.owner_user_id = ? AND platform_operation_intents.platform = ? AND platform_account_bindings.deleted_at IS NULL", ownerUserID, platform).
		Where("platform_operation_intents.kind = ? AND platform_operation_intents.state IN ?", "OPERATION_KIND_BIND_CREDENTIAL", reservingOperationIntentStates()).
		Order("platform_operation_intents.created_at DESC").Take(&intent).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &intent, nil
}

func (s *OperationIntentService) CreateBindingAndAdmit(ctx context.Context, bindingInput CreateBindingInput, actorType, actorID, operationID string) (*model.PlatformAccountBinding, *model.PlatformOperationIntent, error) {
	if bindingInput.OwnerUserID == 0 || bindingInput.Platform == "" || bindingInput.PlatformServiceKey == "" || actorType == "" || actorID == "" || operationID == "" {
		return nil, nil, ErrInvalidBindingMutation
	}
	var binding model.PlatformAccountBinding
	var intent model.PlatformOperationIntent
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		binding = model.PlatformAccountBinding{
			OwnerUserID: bindingInput.OwnerUserID, Platform: bindingInput.Platform, ExternalAccountKey: bindingInput.ExternalAccountKey,
			PlatformServiceKey: bindingInput.PlatformServiceKey, DisplayName: bindingInput.DisplayName, Status: model.PlatformAccountBindingStatusPendingBind,
		}
		if err := tx.Create(&binding).Error; err != nil {
			return err
		}
		reference := newCredentialOperationReference(&binding, operationID, false)
		intent = model.PlatformOperationIntent{
			OperationID: reference.OperationID, BindingID: binding.ID, BindingRef: reference.BindingRef,
			OwnerUserID: binding.OwnerUserID, Platform: binding.Platform,
			Kind: reference.Kind, PreGeneration: reference.PreGeneration, TargetGeneration: reference.TargetGeneration,
			RequestFingerprint: reference.RequestFingerprint, DeliveryMode: deliveryModeForOperation(reference.Kind), State: model.PlatformOperationIntentStatePendingDelivery,
			ActorType: actorType, ActorID: actorID,
		}
		if err := tx.Create(&intent).Error; err != nil {
			return err
		}
		now := time.Now().UTC()
		if err := tx.Create(&model.PlatformOperationOutbox{
			OperationID: operationID, Status: model.PlatformOperationOutboxStatusPending, AvailableAt: now.Add(credentialOperationInitialDeliveryLease),
		}).Error; err != nil {
			return err
		}
		return writeOperationAdmissionAudit(tx, &intent)
	})
	if err != nil {
		if dberror.IsUniqueViolation(err) {
			pending, findErr := s.FindPendingBindForOwner(ctx, bindingInput.OwnerUserID, bindingInput.Platform)
			if findErr != nil {
				return nil, nil, findErr
			}
			if pending != nil {
				return nil, nil, &CredentialOperationPendingError{OperationID: pending.OperationID, BindingID: pending.BindingID, State: pending.State}
			}
		}
		return nil, nil, err
	}
	return &binding, &intent, nil
}

func (s *OperationIntentService) RetryNonSensitive(ctx context.Context, previousOperationID string, input CredentialOperationIntentInput) (*model.PlatformOperationIntent, error) {
	if input.Kind != "OPERATION_KIND_REFRESH_CREDENTIAL" && input.Kind != "OPERATION_KIND_DELETE_CREDENTIAL" && input.Kind != "OPERATION_KIND_SET_PRIMARY_PROFILE" {
		return nil, ErrInvalidBindingMutation
	}
	var retry model.PlatformOperationIntent
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var previous model.PlatformOperationIntent
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("operation_id = ?", previousOperationID).Take(&previous).Error; err != nil {
			return err
		}
		if !previous.State.ReservesBinding() || previous.BindingID != input.BindingID || previous.BindingRef != input.BindingRef || previous.Kind != input.Kind || previous.PreGeneration != input.PreGeneration || previous.TargetGeneration != input.TargetGeneration || previous.RequestFingerprint != input.RequestFingerprint || previous.ProfileRef != input.ProfileRef || previous.ProfileRevision != input.ProfileRevision {
			return ErrInvalidBindingMutation
		}
		now := time.Now().UTC()
		if err := tx.Model(&model.PlatformOperationIntent{}).Where("operation_id = ?", previousOperationID).
			Updates(map[string]any{"state": model.PlatformOperationIntentStateSuperseded, "reason_code": "not_received_retry", "resolved_at": now, "updated_at": now}).Error; err != nil {
			return err
		}
		if err := tx.Model(&model.PlatformOperationOutbox{}).Where("operation_id = ?", previousOperationID).
			Updates(map[string]any{"status": model.PlatformOperationOutboxStatusCompleted, "last_reason_code": "not_received_retry", "updated_at": now}).Error; err != nil {
			return err
		}
		retry = model.PlatformOperationIntent{
			OperationID: input.OperationID, BindingID: input.BindingID, BindingRef: input.BindingRef,
			OwnerUserID: previous.OwnerUserID, Platform: previous.Platform,
			Kind: input.Kind, PreGeneration: input.PreGeneration, TargetGeneration: input.TargetGeneration,
			RequestFingerprint: input.RequestFingerprint, ProfileRef: input.ProfileRef, ProfileRevision: input.ProfileRevision,
			DeliveryMode: model.PlatformOperationDeliveryModeOutbox, State: model.PlatformOperationIntentStatePendingDelivery,
			ActorType: input.ActorType, ActorID: input.ActorID,
		}
		if err := tx.Create(&retry).Error; err != nil {
			return err
		}
		if err := tx.Create(&model.PlatformOperationOutbox{
			OperationID: retry.OperationID, Status: model.PlatformOperationOutboxStatusPending, AvailableAt: now,
		}).Error; err != nil {
			return err
		}
		return writeOperationAdmissionAudit(tx, &retry)
	})
	if err != nil {
		return nil, err
	}
	return &retry, nil
}

func (s *OperationIntentService) MarkUncertain(ctx context.Context, operationID, reasonCode string) error {
	return s.transition(ctx, operationID, model.PlatformOperationIntentStateUncertain, reasonCode, false,
		model.PlatformOperationIntentStatePendingDelivery, model.PlatformOperationIntentStateUncertain)
}

func (s *OperationIntentService) MarkProjectionPending(ctx context.Context, operationID, reasonCode string) error {
	return s.transition(ctx, operationID, model.PlatformOperationIntentStateProjectionPending, reasonCode, false,
		model.PlatformOperationIntentStatePendingDelivery, model.PlatformOperationIntentStateUncertain, model.PlatformOperationIntentStateProjectionPending)
}

func (s *OperationIntentService) MarkInputRequired(ctx context.Context, operationID, reasonCode string) error {
	expiresAt := time.Now().UTC().Add(credentialOperationInputTTL)
	return s.transitionWithExpiry(ctx, operationID, model.PlatformOperationIntentStateInputRequired, reasonCode, expiresAt,
		model.PlatformOperationIntentStatePendingDelivery, model.PlatformOperationIntentStateUncertain)
}

func (s *OperationIntentService) MarkInvariantViolation(ctx context.Context, operationID, reasonCode string) error {
	zap.L().Error("platform credential operation invariant violation", zap.String("operation_id", operationID), zap.String("reason_code", reasonCode))
	return s.transition(ctx, operationID, model.PlatformOperationIntentStateInvariantViolation, reasonCode, false,
		model.PlatformOperationIntentStatePendingDelivery, model.PlatformOperationIntentStateUncertain, model.PlatformOperationIntentStateProjectionPending)
}

func (s *OperationIntentService) MarkSucceeded(ctx context.Context, operationID string) error {
	return s.transition(ctx, operationID, model.PlatformOperationIntentStateSucceeded, "", true,
		model.PlatformOperationIntentStateProjectionPending)
}

func (s *OperationIntentService) MarkFailed(ctx context.Context, operationID, reasonCode string) error {
	return s.transition(ctx, operationID, model.PlatformOperationIntentStateFailed, reasonCode, true,
		model.PlatformOperationIntentStatePendingDelivery, model.PlatformOperationIntentStateUncertain, model.PlatformOperationIntentStateProjectionPending)
}

func (s *OperationIntentService) ExpireInputRequired(ctx context.Context, operationID string, now time.Time) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		result := tx.Model(&model.PlatformOperationIntent{}).
			Where("operation_id = ? AND state = ? AND input_expires_at <= ?", operationID, model.PlatformOperationIntentStateInputRequired, now.UTC()).
			Updates(map[string]any{"state": model.PlatformOperationIntentStateSuperseded, "reason_code": "credential_input_expired", "resolved_at": now.UTC(), "updated_at": now.UTC()})
		if result.Error != nil || result.RowsAffected == 0 {
			return result.Error
		}
		return tx.Model(&model.PlatformOperationOutbox{}).Where("operation_id = ?", operationID).
			Updates(map[string]any{"status": model.PlatformOperationOutboxStatusCompleted, "last_reason_code": "credential_input_expired", "updated_at": now.UTC()}).Error
	})
}

func (s *OperationIntentService) Reschedule(ctx context.Context, operationID, reasonCode string, availableAt time.Time) error {
	return s.db.WithContext(ctx).Model(&model.PlatformOperationOutbox{}).
		Where("operation_id = ? AND status = ?", operationID, model.PlatformOperationOutboxStatusPending).
		Updates(map[string]any{
			"attempt_count":    gorm.Expr("attempt_count + 1"),
			"last_reason_code": reasonCode,
			"available_at":     availableAt.UTC(),
			"updated_at":       time.Now().UTC(),
		}).Error
}

func (s *OperationIntentService) ClaimDueOperationIDs(ctx context.Context, now time.Time, limit int) ([]string, error) {
	if limit <= 0 {
		limit = 100
	}
	var operationIDs []string
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		deadLetters := tx.Model(&model.PlatformOperationOutbox{}).
			Where("status = ? AND available_at <= ? AND attempt_count >= ?", model.PlatformOperationOutboxStatusPending, now.UTC(), credentialOperationDeadLetterAttempts).
			Updates(map[string]any{"status": model.PlatformOperationOutboxStatusDeadLetter, "last_reason_code": "retry_exhausted", "updated_at": now.UTC()})
		if deadLetters.Error != nil {
			return deadLetters.Error
		}
		if deadLetters.RowsAffected > 0 {
			zap.L().Error("platform credential operation outbox moved to dead letter", zap.Int64("count", deadLetters.RowsAffected))
			if err := serviceaudit.RecordTx(tx, serviceaudit.WriteInput{
				Category: "platform_binding", ActorType: "system", Action: "platform_operation_dead_lettered",
				TargetType: "outbox", Result: "failure", ReasonCode: "retry_exhausted",
				Metadata: map[string]any{"count": deadLetters.RowsAffected},
			}); err != nil {
				return err
			}
		}
		var rows []model.PlatformOperationOutbox
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"}).
			Where("status = ? AND available_at <= ? AND attempt_count < ?", model.PlatformOperationOutboxStatusPending, now.UTC(), credentialOperationDeadLetterAttempts).
			Order("available_at ASC, id ASC").Limit(limit).Find(&rows).Error; err != nil {
			return err
		}
		if len(rows) == 0 {
			return nil
		}
		ids := make([]uint64, 0, len(rows))
		operationIDs = make([]string, 0, len(rows))
		for _, row := range rows {
			ids = append(ids, row.ID)
			operationIDs = append(operationIDs, row.OperationID)
		}
		return tx.Model(&model.PlatformOperationOutbox{}).Where("id IN ?", ids).Updates(map[string]any{
			"available_at": now.UTC().Add(credentialOperationDispatchLease), "attempt_count": gorm.Expr("attempt_count + 1"), "updated_at": now.UTC(),
		}).Error
	})
	return operationIDs, err
}

func (s *OperationIntentService) transition(ctx context.Context, operationID string, state model.PlatformOperationIntentState, reasonCode string, completeOutbox bool, allowedFrom ...model.PlatformOperationIntentState) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		now := time.Now().UTC()
		updates := map[string]any{"state": state, "reason_code": reasonCode, "updated_at": now}
		if completeOutbox {
			updates["resolved_at"] = now
		}
		result := tx.Model(&model.PlatformOperationIntent{}).Where("operation_id = ? AND state IN ?", operationID, allowedFrom).Updates(updates)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			var current model.PlatformOperationIntent
			if err := tx.Where("operation_id = ?", operationID).Take(&current).Error; err != nil {
				return err
			}
			if current.State == state || !current.State.ReservesBinding() {
				return nil
			}
			return ErrBindingGenerationConflict
		}
		if !completeOutbox {
			return tx.Model(&model.PlatformOperationOutbox{}).Where("operation_id = ?", operationID).Updates(map[string]any{
				"status": model.PlatformOperationOutboxStatusPending, "available_at": now.Add(credentialOperationRetryDelay), "last_reason_code": reasonCode, "updated_at": now,
			}).Error
		}
		return tx.Model(&model.PlatformOperationOutbox{}).Where("operation_id = ?", operationID).
			Updates(map[string]any{"status": model.PlatformOperationOutboxStatusCompleted, "last_reason_code": reasonCode, "updated_at": now}).Error
	})
}

func (s *OperationIntentService) transitionWithExpiry(ctx context.Context, operationID string, state model.PlatformOperationIntentState, reasonCode string, expiresAt time.Time, allowedFrom ...model.PlatformOperationIntentState) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		now := time.Now().UTC()
		result := tx.Model(&model.PlatformOperationIntent{}).Where("operation_id = ? AND state IN ?", operationID, allowedFrom).
			Updates(map[string]any{"state": state, "reason_code": reasonCode, "input_expires_at": expiresAt.UTC(), "updated_at": now})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return ErrBindingGenerationConflict
		}
		return tx.Model(&model.PlatformOperationOutbox{}).Where("operation_id = ?", operationID).Updates(map[string]any{
			"status": model.PlatformOperationOutboxStatusPending, "available_at": expiresAt.UTC(), "attempt_count": 0, "last_reason_code": reasonCode, "updated_at": now,
		}).Error
	})
}

func deliveryModeForOperation(kind string) model.PlatformOperationDeliveryMode {
	if kind == "OPERATION_KIND_BIND_CREDENTIAL" || kind == "OPERATION_KIND_REPLACE_CREDENTIAL" {
		return model.PlatformOperationDeliveryModeSyncSecret
	}
	return model.PlatformOperationDeliveryModeOutbox
}

func validIntentGeneration(input CredentialOperationIntentInput) bool {
	if input.Kind == "OPERATION_KIND_SET_PRIMARY_PROFILE" {
		return input.TargetGeneration == input.PreGeneration
	}
	return input.TargetGeneration == input.PreGeneration+1
}

func validIntentProfile(input CredentialOperationIntentInput) bool {
	if input.Kind == "OPERATION_KIND_SET_PRIMARY_PROFILE" {
		return input.ProfileRef != "" && input.ProfileRevision > 0
	}
	return input.ProfileRef == "" && input.ProfileRevision == 0
}

func writeOperationAdmissionAudit(tx *gorm.DB, intent *model.PlatformOperationIntent) error {
	if intent == nil {
		return ErrInvalidBindingMutation
	}
	return serviceaudit.RecordTx(tx, serviceaudit.WriteInput{
		Category: "platform_binding", ActorType: intent.ActorType, ActorUserID: actorUserIDFromAuditContext(intent.ActorType, intent.ActorID, &intent.OwnerUserID), Action: "platform_operation_admitted",
		TargetType: "binding", TargetID: strconv.FormatUint(intent.BindingID, 10), BindingID: &intent.BindingID,
		OwnerUserID: &intent.OwnerUserID, Result: "pending", ReasonCode: "operation_admitted",
		Metadata: map[string]any{"operation_id": intent.OperationID, "operation_kind": intent.Kind, "delivery_mode": intent.DeliveryMode, "actor_id": intent.ActorID},
	})
}

func reservingOperationIntentStates() []model.PlatformOperationIntentState {
	return []model.PlatformOperationIntentState{
		model.PlatformOperationIntentStatePendingDelivery,
		model.PlatformOperationIntentStateUncertain,
		model.PlatformOperationIntentStateProjectionPending,
		model.PlatformOperationIntentStateInputRequired,
		model.PlatformOperationIntentStateInvariantViolation,
	}
}

var _ credentialOperationIntentStore = (*OperationIntentService)(nil)

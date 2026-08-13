package platformbinding

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"paigram/internal/model"
	serviceaudit "paigram/internal/service/audit"
)

// OperationRecoveryRecord is the non-sensitive admin view of an operation and its wake-up state.
type OperationRecoveryRecord struct {
	OperationID      string
	Kind             string
	State            model.PlatformOperationIntentState
	ReasonCode       string
	DeliveryMode     model.PlatformOperationDeliveryMode
	PreGeneration    uint64
	TargetGeneration uint64
	ProfileRevision  uint64
	OutboxStatus     model.PlatformOperationOutboxStatus
	AttemptCount     uint32
	LastReasonCode   string
	AvailableAt      time.Time
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

// OperationRecoveryService exposes safe dead-letter inspection and recovery controls.
type OperationRecoveryService struct {
	db *gorm.DB
}

// NewOperationRecoveryService creates an operation recovery service.
func NewOperationRecoveryService(db *gorm.DB) *OperationRecoveryService {
	return &OperationRecoveryService{db: db}
}

// ListForBinding returns non-sensitive operation state for one binding.
func (s *OperationRecoveryService) ListForBinding(ctx context.Context, bindingID uint64, params ListParams) ([]OperationRecoveryRecord, int64, error) {
	if s == nil || s.db == nil || bindingID == 0 {
		return nil, 0, ErrInvalidBindingMutation
	}
	params = normalizeListParams(params)
	query := s.db.WithContext(ctx).Model(&model.PlatformOperationIntent{}).Where("binding_id = ?", bindingID)
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var intents []model.PlatformOperationIntent
	if err := query.Order("created_at DESC, operation_id DESC").Limit(params.PageSize).Offset((params.Page - 1) * params.PageSize).Find(&intents).Error; err != nil {
		return nil, 0, err
	}
	if len(intents) == 0 {
		return []OperationRecoveryRecord{}, total, nil
	}
	operationIDs := make([]string, 0, len(intents))
	for index := range intents {
		operationIDs = append(operationIDs, intents[index].OperationID)
	}
	var outboxes []model.PlatformOperationOutbox
	if err := s.db.WithContext(ctx).Where("operation_id IN ?", operationIDs).Find(&outboxes).Error; err != nil {
		return nil, 0, err
	}
	outboxByOperation := make(map[string]model.PlatformOperationOutbox, len(outboxes))
	for _, outbox := range outboxes {
		outboxByOperation[outbox.OperationID] = outbox
	}
	items := make([]OperationRecoveryRecord, 0, len(intents))
	for _, intent := range intents {
		items = append(items, buildOperationRecoveryRecord(intent, outboxByOperation[intent.OperationID]))
	}
	return items, total, nil
}

// RequeueDeadLetter reactivates only the payload-free wake-up for a reserving operation.
func (s *OperationRecoveryService) RequeueDeadLetter(ctx context.Context, bindingID uint64, operationID string, adminUserID uint64) (*OperationRecoveryRecord, error) {
	if s == nil || s.db == nil || bindingID == 0 || operationID == "" || adminUserID == 0 {
		return nil, ErrInvalidBindingMutation
	}
	var record OperationRecoveryRecord
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var intent model.PlatformOperationIntent
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("binding_id = ? AND operation_id = ?", bindingID, operationID).Take(&intent).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrCredentialOperationNotFound
			}
			return err
		}
		var outbox model.PlatformOperationOutbox
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("operation_id = ?", operationID).Take(&outbox).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrCredentialOperationNotFound
			}
			return err
		}
		if !intent.State.ReservesBinding() || outbox.Status != model.PlatformOperationOutboxStatusDeadLetter {
			return ErrCredentialOperationNotRecoverable
		}
		now := time.Now().UTC()
		result := tx.Model(&model.PlatformOperationOutbox{}).Where("id = ? AND status = ?", outbox.ID, model.PlatformOperationOutboxStatusDeadLetter).Updates(map[string]any{
			"status":           model.PlatformOperationOutboxStatusPending,
			"attempt_count":    0,
			"last_reason_code": "manual_requeue",
			"available_at":     now,
			"updated_at":       now,
		})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return ErrCredentialOperationNotRecoverable
		}
		outbox.Status = model.PlatformOperationOutboxStatusPending
		outbox.AttemptCount = 0
		outbox.LastReasonCode = "manual_requeue"
		outbox.AvailableAt = now
		outbox.UpdatedAt = now
		if err := serviceaudit.RecordTx(tx, serviceaudit.WriteInput{
			Category: "platform_binding", ActorType: "admin", ActorUserID: &adminUserID,
			Action: "platform_operation_requeued", TargetType: "platform_operation", TargetID: operationID,
			BindingID: &intent.BindingID, OwnerUserID: &intent.OwnerUserID, Result: "pending", ReasonCode: "manual_requeue",
			Metadata: map[string]any{"operation_id": operationID, "operation_kind": intent.Kind, "intent_state": intent.State},
		}); err != nil {
			return err
		}
		record = buildOperationRecoveryRecord(intent, outbox)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &record, nil
}

func buildOperationRecoveryRecord(intent model.PlatformOperationIntent, outbox model.PlatformOperationOutbox) OperationRecoveryRecord {
	return OperationRecoveryRecord{
		OperationID: intent.OperationID, Kind: intent.Kind, State: intent.State, ReasonCode: intent.ReasonCode,
		DeliveryMode: intent.DeliveryMode, PreGeneration: intent.PreGeneration, TargetGeneration: intent.TargetGeneration,
		ProfileRevision: intent.ProfileRevision, OutboxStatus: outbox.Status, AttemptCount: outbox.AttemptCount,
		LastReasonCode: outbox.LastReasonCode, AvailableAt: outbox.AvailableAt, CreatedAt: intent.CreatedAt, UpdatedAt: intent.UpdatedAt,
	}
}

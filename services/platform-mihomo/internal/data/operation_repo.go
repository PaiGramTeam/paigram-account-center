package data

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"platform-mihomo-service/internal/biz"
	"platform-mihomo-service/internal/data/model"
)

const (
	operationStatePending             = "pending"
	operationStateNotReceived         = "not_received"
	operationStateFailedInputRequired = "failed_input_required"
	operationExecutionLease           = 30 * time.Second
)

type OperationRepo struct {
	db *gorm.DB
}

func NewOperationRepo(db *gorm.DB) *OperationRepo {
	return &OperationRepo{db: db}
}

func (r *OperationRepo) WithinTransaction(ctx context.Context, fn func(context.Context) error) error {
	if txFromContext(ctx) != nil {
		return fn(ctx)
	}
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return fn(withTx(ctx, tx))
	})
}

func (r *OperationRepo) Admit(ctx context.Context, operation biz.OperationRef) (*biz.OperationResult, bool, error) {
	var output *biz.OperationResult
	admitted := false
	err := r.WithinTransaction(ctx, func(txCtx context.Context) error {
		now := time.Now().UTC()
		record := model.PlatformOperation{
			OperationID: operation.OperationID, Kind: operation.Kind, BindingRef: operation.BindingRef,
			PreGeneration: operation.PreGeneration, TargetGeneration: operation.TargetGeneration,
			RequestFingerprint: operation.RequestFingerprint, ExecutionToken: uuid.NewString(),
			LeaseExpiresAt: now.Add(operationExecutionLease), State: operationStatePending, SnapshotJSON: "{}",
		}
		result := dbFromContext(txCtx, r.db).Clauses(clause.OnConflict{DoNothing: true}).Create(&record)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 1 {
			output = operationFromRecord(record)
			admitted = true
			return nil
		}
		existing, err := r.get(txCtx, operation.OperationID, true)
		if err != nil {
			return err
		}
		if existing == nil || existing.Operation != operation {
			return biz.ErrOperationConflict
		}
		output = existing
		return nil
	})
	return output, admitted, err
}

func (r *OperationRepo) Complete(ctx context.Context, result biz.OperationResult) error {
	if result.State == "" || result.State == operationStatePending || result.State == operationStateNotReceived {
		return biz.ErrOperationState
	}
	updates := map[string]any{
		"state":             result.State,
		"reason_code":       result.ReasonCode,
		"account_key":       result.AccountKey,
		"credential_status": result.Status,
		"snapshot_json":     result.SnapshotJSON,
	}
	write := dbFromContext(ctx, r.db).Model(&model.PlatformOperation{}).
		Where("operation_id = ? AND state = ? AND execution_token = ?", result.Operation.OperationID, operationStatePending, result.ExecutionToken).
		Updates(updates)
	if write.Error != nil {
		return write.Error
	}
	if write.RowsAffected != 1 {
		return biz.ErrOperationState
	}
	return nil
}

func (r *OperationRepo) FailPending(ctx context.Context, operationID, executionToken, reasonCode string) error {
	write := dbFromContext(ctx, r.db).Model(&model.PlatformOperation{}).
		Where("operation_id = ? AND state = ? AND execution_token = ?", operationID, operationStatePending, executionToken).
		Updates(map[string]any{"state": "failed", "reason_code": reasonCode, "execution_token": uuid.NewString()})
	if write.Error != nil {
		return write.Error
	}
	if write.RowsAffected != 1 {
		return biz.ErrOperationState
	}
	return nil
}

func (r *OperationRepo) Get(ctx context.Context, operationID string) (*biz.OperationResult, error) {
	return r.get(ctx, operationID, false)
}

func (r *OperationRepo) get(ctx context.Context, operationID string, lock bool) (*biz.OperationResult, error) {
	var record model.PlatformOperation
	query := dbFromContext(ctx, r.db).Where("operation_id = ?", operationID)
	if lock {
		query = query.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	err := query.Take(&record).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return operationFromRecord(record), nil
}

func (r *OperationRepo) Resolve(ctx context.Context, operation biz.OperationRef) (*biz.OperationResult, error) {
	var resolved *biz.OperationResult
	err := r.WithinTransaction(ctx, func(txCtx context.Context) error {
		record := model.PlatformOperation{
			OperationID:        operation.OperationID,
			Kind:               operation.Kind,
			BindingRef:         operation.BindingRef,
			PreGeneration:      operation.PreGeneration,
			TargetGeneration:   operation.TargetGeneration,
			RequestFingerprint: operation.RequestFingerprint,
			ExecutionToken:     uuid.NewString(),
			LeaseExpiresAt:     time.Now().UTC(),
			State:              operationStateNotReceived,
			SnapshotJSON:       "{}",
		}
		insert := dbFromContext(txCtx, r.db).Clauses(clause.OnConflict{DoNothing: true}).Create(&record)
		if insert.Error != nil {
			return insert.Error
		}
		if insert.RowsAffected == 1 {
			resolved = operationFromRecord(record)
			return nil
		}
		existing, err := r.get(txCtx, operation.OperationID, true)
		if err != nil {
			return err
		}
		if existing.Operation != operation {
			return biz.ErrOperationConflict
		}
		if existing.State == operationStatePending && !existing.LeaseExpiresAt.After(time.Now().UTC()) {
			write := dbFromContext(txCtx, r.db).Model(&model.PlatformOperation{}).
				Where("operation_id = ? AND state = ? AND execution_token = ?", operation.OperationID, operationStatePending, existing.ExecutionToken).
				Updates(map[string]any{"state": operationStateFailedInputRequired, "reason_code": "operation_outcome_unknown", "execution_token": uuid.NewString()})
			if write.Error != nil {
				return write.Error
			}
			return reloadOperation(txCtx, r, operation.OperationID, &resolved)
		}
		resolved = existing
		return nil
	})
	return resolved, err
}

func reloadOperation(ctx context.Context, repo *OperationRepo, operationID string, output **biz.OperationResult) error {
	result, err := repo.Get(ctx, operationID)
	if err != nil {
		return err
	}
	*output = result
	return nil
}

func operationFromRecord(record model.PlatformOperation) *biz.OperationResult {
	return &biz.OperationResult{
		Operation: biz.OperationRef{
			OperationID:        record.OperationID,
			Kind:               record.Kind,
			BindingRef:         record.BindingRef,
			PreGeneration:      record.PreGeneration,
			TargetGeneration:   record.TargetGeneration,
			RequestFingerprint: record.RequestFingerprint,
		},
		State:          record.State,
		ReasonCode:     record.ReasonCode,
		AccountKey:     record.AccountKey,
		Status:         record.CredentialStatus,
		SnapshotJSON:   record.SnapshotJSON,
		ExecutionToken: record.ExecutionToken,
		LeaseExpiresAt: record.LeaseExpiresAt,
		CreatedAt:      record.CreatedAt,
		UpdatedAt:      record.UpdatedAt,
	}
}

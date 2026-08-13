package platformbinding

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"paigram/internal/model"
)

func deadLetterOperation(t *testing.T, dbOperationID string) (*OperationRecoveryService, *OperationIntentService) {
	t.Helper()
	db := openOperationIntentTestDB(t)
	intents := NewOperationIntentService(db)
	_, err := intents.Admit(context.Background(), operationIntentInput(dbOperationID))
	require.NoError(t, err)
	require.NoError(t, intents.MarkUncertain(context.Background(), dbOperationID, "delivery_outcome_unknown"))
	require.NoError(t, intents.MarkInvariantViolation(context.Background(), dbOperationID, "resolve_rejected"))
	require.NoError(t, db.Model(&model.PlatformOperationOutbox{}).Where("operation_id = ?", dbOperationID).Updates(map[string]any{
		"status":           model.PlatformOperationOutboxStatusDeadLetter,
		"attempt_count":    credentialOperationDeadLetterAttempts,
		"last_reason_code": "retry_exhausted",
		"available_at":     time.Now().UTC().Add(time.Hour),
	}).Error)
	return NewOperationRecoveryService(db), intents
}

func TestOperationRecoveryListsSafeRecoveryState(t *testing.T) {
	recovery, _ := deadLetterOperation(t, "op_dead")

	items, total, err := recovery.ListForBinding(context.Background(), 101, ListParams{Page: 1, PageSize: 20})
	require.NoError(t, err)
	assert.Equal(t, int64(1), total)
	require.Len(t, items, 1)
	assert.Equal(t, "op_dead", items[0].OperationID)
	assert.Equal(t, model.PlatformOperationIntentStateInvariantViolation, items[0].State)
	assert.Equal(t, model.PlatformOperationOutboxStatusDeadLetter, items[0].OutboxStatus)
	assert.Equal(t, uint32(credentialOperationDeadLetterAttempts), items[0].AttemptCount)
	assert.Equal(t, "resolve_rejected", items[0].ReasonCode)
}

func TestOperationRecoveryRequeuesOnlyPayloadFreeWakeup(t *testing.T) {
	recovery, intents := deadLetterOperation(t, "op_dead")
	before, err := intents.Get(context.Background(), "op_dead")
	require.NoError(t, err)

	record, err := recovery.RequeueDeadLetter(context.Background(), 101, "op_dead", 99)
	require.NoError(t, err)
	assert.Equal(t, model.PlatformOperationOutboxStatusPending, record.OutboxStatus)
	assert.Zero(t, record.AttemptCount)
	assert.WithinDuration(t, time.Now().UTC(), record.AvailableAt, time.Second)

	after, err := intents.Get(context.Background(), "op_dead")
	require.NoError(t, err)
	assert.Equal(t, before.Kind, after.Kind)
	assert.Equal(t, before.PreGeneration, after.PreGeneration)
	assert.Equal(t, before.TargetGeneration, after.TargetGeneration)
	assert.Equal(t, before.RequestFingerprint, after.RequestFingerprint)
	assert.Equal(t, before.ActorType, after.ActorType)
	assert.Equal(t, before.ActorID, after.ActorID)

	var audit model.AuditEvent
	require.NoError(t, recovery.db.Where("action = ?", "platform_operation_requeued").Take(&audit).Error)
	assert.True(t, audit.ActorUserID.Valid)
	assert.Equal(t, int64(99), audit.ActorUserID.Int64)
	assert.NotContains(t, audit.MetadataJSON, before.RequestFingerprint)
}

func TestOperationRecoveryRejectsPendingAndWrongBinding(t *testing.T) {
	recovery, _ := deadLetterOperation(t, "op_dead")

	_, err := recovery.RequeueDeadLetter(context.Background(), 999, "op_dead", 99)
	require.ErrorIs(t, err, ErrCredentialOperationNotFound)
	require.NoError(t, recovery.db.Model(&model.PlatformOperationOutbox{}).Where("operation_id = ?", "op_dead").Update("status", model.PlatformOperationOutboxStatusPending).Error)
	_, err = recovery.RequeueDeadLetter(context.Background(), 101, "op_dead", 99)
	require.ErrorIs(t, err, ErrCredentialOperationNotRecoverable)
}

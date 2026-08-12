package platformbinding

import (
	"context"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"paigram/internal/model"
)

func openOperationIntentTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.Exec(`CREATE TABLE platform_account_bindings (id INTEGER PRIMARY KEY, owner_user_id INTEGER, platform TEXT, deleted_at DATETIME)`).Error)
	require.NoError(t, db.AutoMigrate(&model.AuditEvent{}, &model.PlatformOperationIntent{}, &model.PlatformOperationOutbox{}))
	require.NoError(t, db.Exec(`INSERT INTO platform_account_bindings (id, owner_user_id, platform) VALUES (101, 7, 'mihomo')`).Error)
	return db
}

func operationIntentInput(operationID string) CredentialOperationIntentInput {
	return CredentialOperationIntentInput{
		OperationID:        operationID,
		BindingID:          101,
		BindingRef:         "bind_test",
		Kind:               "OPERATION_KIND_BIND_CREDENTIAL",
		PreGeneration:      0,
		TargetGeneration:   1,
		RequestFingerprint: "non-secret-fingerprint",
		ActorType:          "user",
		ActorID:            "session:test",
	}
}

func TestOperationIntentAdmitPersistsOnlyNonSensitiveTupleAndWakeup(t *testing.T) {
	db := openOperationIntentTestDB(t)
	service := NewOperationIntentService(db)

	intent, err := service.Admit(context.Background(), operationIntentInput("op_first"))
	require.NoError(t, err)
	require.NotNil(t, intent)
	assert.Equal(t, model.PlatformOperationIntentStatePendingDelivery, intent.State)
	assert.Equal(t, uint64(7), intent.OwnerUserID)
	assert.Equal(t, "mihomo", intent.Platform)
	assert.Equal(t, model.PlatformOperationDeliveryModeSyncSecret, intent.DeliveryMode)

	var outbox model.PlatformOperationOutbox
	require.NoError(t, db.Where("operation_id = ?", intent.OperationID).Take(&outbox).Error)
	assert.Equal(t, model.PlatformOperationOutboxStatusPending, outbox.Status)
	var audit model.AuditEvent
	require.NoError(t, db.Where("action = ?", "platform_operation_admitted").Take(&audit).Error)
	assert.Contains(t, audit.MetadataJSON, intent.OperationID)

	columns, err := db.Migrator().ColumnTypes(&model.PlatformOperationIntent{})
	require.NoError(t, err)
	for _, column := range columns {
		assert.NotContains(t, []string{"credential_payload", "payload_json", "credential_hash"}, column.Name())
	}
}

func TestOperationIntentUncertainRetainsBindingReservation(t *testing.T) {
	service := NewOperationIntentService(openOperationIntentTestDB(t))
	_, err := service.Admit(context.Background(), operationIntentInput("op_first"))
	require.NoError(t, err)
	require.NoError(t, service.MarkUncertain(context.Background(), "op_first", "delivery_outcome_unknown"))

	_, err = service.Admit(context.Background(), operationIntentInput("op_second"))
	var pending *CredentialOperationPendingError
	require.ErrorAs(t, err, &pending)
	assert.Equal(t, "op_first", pending.OperationID)
	assert.Equal(t, model.PlatformOperationIntentStateUncertain, pending.State)
}

func TestOperationIntentFindsPendingBindReservationForOwnerAndPlatform(t *testing.T) {
	service := NewOperationIntentService(openOperationIntentTestDB(t))
	_, err := service.Admit(context.Background(), operationIntentInput("op_first"))
	require.NoError(t, err)
	require.NoError(t, service.MarkUncertain(context.Background(), "op_first", "delivery_outcome_unknown"))

	intent, err := service.FindPendingBindForOwner(context.Background(), 7, "mihomo")
	require.NoError(t, err)
	require.NotNil(t, intent)
	assert.Equal(t, "op_first", intent.OperationID)

	missing, err := service.FindPendingBindForOwner(context.Background(), 8, "mihomo")
	require.NoError(t, err)
	assert.Nil(t, missing)
}

func TestOperationIntentInputRequiredCanBeSupersededOnlyByNewAdmission(t *testing.T) {
	db := openOperationIntentTestDB(t)
	service := NewOperationIntentService(db)
	_, err := service.Admit(context.Background(), operationIntentInput("op_first"))
	require.NoError(t, err)
	require.NoError(t, service.MarkInputRequired(context.Background(), "op_first", "credential_resubmission_required"))
	firstRequired, err := service.Get(context.Background(), "op_first")
	require.NoError(t, err)
	assert.NotNil(t, firstRequired.InputExpiresAt)

	second, err := service.Admit(context.Background(), operationIntentInput("op_second"))
	require.NoError(t, err)
	assert.Equal(t, "op_second", second.OperationID)

	first, err := service.Get(context.Background(), "op_first")
	require.NoError(t, err)
	assert.Equal(t, model.PlatformOperationIntentStateSuperseded, first.State)
	var firstOutbox model.PlatformOperationOutbox
	require.NoError(t, db.Where("operation_id = ?", "op_first").Take(&firstOutbox).Error)
	assert.Equal(t, model.PlatformOperationOutboxStatusCompleted, firstOutbox.Status)
}

func TestOperationIntentInputRequiredExpiresBeforeReservationIsReleased(t *testing.T) {
	service := NewOperationIntentService(openOperationIntentTestDB(t))
	_, err := service.Admit(context.Background(), operationIntentInput("op_first"))
	require.NoError(t, err)
	require.NoError(t, service.MarkUncertain(context.Background(), "op_first", "delivery_outcome_unknown"))
	require.NoError(t, service.MarkInputRequired(context.Background(), "op_first", "credential_resubmission_required"))

	intent, err := service.Get(context.Background(), "op_first")
	require.NoError(t, err)
	require.NotNil(t, intent.InputExpiresAt)
	var outbox model.PlatformOperationOutbox
	require.NoError(t, service.db.Where("operation_id = ?", "op_first").Take(&outbox).Error)
	assert.Zero(t, outbox.AttemptCount)
	require.NoError(t, service.ExpireInputRequired(context.Background(), "op_first", intent.InputExpiresAt.Add(-time.Second)))
	intent, err = service.Get(context.Background(), "op_first")
	require.NoError(t, err)
	assert.Equal(t, model.PlatformOperationIntentStateInputRequired, intent.State)

	require.NoError(t, service.ExpireInputRequired(context.Background(), "op_first", intent.InputExpiresAt.Add(time.Second)))
	intent, err = service.Get(context.Background(), "op_first")
	require.NoError(t, err)
	assert.Equal(t, model.PlatformOperationIntentStateSuperseded, intent.State)
}

func TestOperationIntentTerminalStateCannotBeReopenedByLateDelivery(t *testing.T) {
	service := NewOperationIntentService(openOperationIntentTestDB(t))
	_, err := service.Admit(context.Background(), operationIntentInput("op_first"))
	require.NoError(t, err)
	require.NoError(t, service.MarkUncertain(context.Background(), "op_first", "delivery_outcome_unknown"))
	require.NoError(t, service.MarkInputRequired(context.Background(), "op_first", "credential_resubmission_required"))

	require.ErrorIs(t, service.MarkProjectionPending(context.Background(), "op_first", "late_success"), ErrBindingGenerationConflict)
	intent, err := service.Get(context.Background(), "op_first")
	require.NoError(t, err)
	assert.Equal(t, model.PlatformOperationIntentStateInputRequired, intent.State)
}

func TestOperationIntentTerminalTransitionCompletesOutbox(t *testing.T) {
	db := openOperationIntentTestDB(t)
	service := NewOperationIntentService(db)
	_, err := service.Admit(context.Background(), operationIntentInput("op_first"))
	require.NoError(t, err)
	require.NoError(t, service.MarkProjectionPending(context.Background(), "op_first", "projection_pending"))
	require.NoError(t, service.MarkSucceeded(context.Background(), "op_first"))

	var outbox model.PlatformOperationOutbox
	require.NoError(t, db.Where("operation_id = ?", "op_first").Take(&outbox).Error)
	assert.Equal(t, model.PlatformOperationOutboxStatusCompleted, outbox.Status)
	assert.NotZero(t, outbox.UpdatedAt)
}

func TestOperationIntentRetriesNonSensitiveCommandAtomicallyWithNewOperationID(t *testing.T) {
	db := openOperationIntentTestDB(t)
	service := NewOperationIntentService(db)
	input := operationIntentInput("op_first")
	input.Kind = "OPERATION_KIND_REFRESH_CREDENTIAL"
	input.PreGeneration = 4
	input.TargetGeneration = 5
	_, err := service.Admit(context.Background(), input)
	require.NoError(t, err)
	require.NoError(t, service.MarkUncertain(context.Background(), "op_first", "delivery_outcome_unknown"))

	retryInput := input
	retryInput.OperationID = "op_retry"
	retry, err := service.RetryNonSensitive(context.Background(), "op_first", retryInput)
	require.NoError(t, err)
	assert.Equal(t, model.PlatformOperationIntentStatePendingDelivery, retry.State)
	assert.Equal(t, uint64(7), retry.OwnerUserID)
	assert.Equal(t, "mihomo", retry.Platform)

	first, err := service.Get(context.Background(), "op_first")
	require.NoError(t, err)
	assert.Equal(t, model.PlatformOperationIntentStateSuperseded, first.State)
	var pendingCount int64
	require.NoError(t, db.Model(&model.PlatformOperationOutbox{}).Where("status = ?", model.PlatformOperationOutboxStatusPending).Count(&pendingCount).Error)
	assert.Equal(t, int64(1), pendingCount)
}

func TestOperationIntentListsOnlyDuePendingWakeups(t *testing.T) {
	db := openOperationIntentTestDB(t)
	service := NewOperationIntentService(db)
	_, err := service.Admit(context.Background(), operationIntentInput("op_due"))
	require.NoError(t, err)
	input := operationIntentInput("op_later")
	input.BindingID = 102
	require.NoError(t, db.Exec(`INSERT INTO platform_account_bindings (id, owner_user_id, platform) VALUES (102, 8, 'mihomo')`).Error)
	_, err = service.Admit(context.Background(), input)
	require.NoError(t, err)
	require.NoError(t, db.Model(&model.PlatformOperationOutbox{}).Where("operation_id = ?", "op_later").Update("available_at", time.Now().UTC().Add(time.Hour)).Error)

	require.NoError(t, db.Model(&model.PlatformOperationOutbox{}).Where("operation_id = ?", "op_due").Update("available_at", time.Now().UTC().Add(-time.Minute)).Error)
	operationIDs, err := service.ClaimDueOperationIDs(context.Background(), time.Now().UTC(), 10)
	require.NoError(t, err)
	assert.Equal(t, []string{"op_due"}, operationIDs)
	var claimed model.PlatformOperationOutbox
	require.NoError(t, db.Where("operation_id = ?", "op_due").Take(&claimed).Error)
	assert.Equal(t, uint32(1), claimed.AttemptCount)
	assert.True(t, claimed.AvailableAt.After(time.Now().UTC()))
}

func TestOperationIntentInitialDeliveryLeasePreventsWorkerFromRacingSynchronousSend(t *testing.T) {
	service := NewOperationIntentService(openOperationIntentTestDB(t))
	_, err := service.Admit(context.Background(), operationIntentInput("op_sync"))
	require.NoError(t, err)

	operationIDs, err := service.ClaimDueOperationIDs(context.Background(), time.Now().UTC(), 10)
	require.NoError(t, err)
	assert.Empty(t, operationIDs)
}

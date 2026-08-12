package usecase

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"platform-mihomo-service/internal/biz"
	"platform-mihomo-service/internal/data"
	"platform-mihomo-service/internal/data/model"
)

func TestOperationExecuteReturnsStoredResultWithoutRepeatingMutation(t *testing.T) {
	uc := newOperationUsecaseForTest(t)
	operation := testOperationRef()
	calls := 0
	mutation := func(context.Context) (*biz.OperationResult, error) {
		calls++
		return &biz.OperationResult{State: "succeeded", AccountKey: "account-1", Status: "active", SnapshotJSON: `{"complete":true}`}, nil
	}

	first, err := uc.Execute(context.Background(), operation, mutation)
	require.NoError(t, err)
	second, err := uc.Execute(context.Background(), operation, mutation)
	require.NoError(t, err)

	require.Equal(t, 1, calls)
	require.Equal(t, first, second)
	require.Equal(t, "succeeded", second.State)
}

func TestOperationExecuteRejectsOperationIDReuseWithDifferentTuple(t *testing.T) {
	uc := newOperationUsecaseForTest(t)
	operation := testOperationRef()
	_, err := uc.Execute(context.Background(), operation, successfulOperationMutation)
	require.NoError(t, err)

	conflicting := operation
	conflicting.BindingRef = "binding-2"
	_, err = uc.Execute(context.Background(), conflicting, successfulOperationMutation)
	require.ErrorIs(t, err, biz.ErrOperationConflict)
}

func TestOperationExecuteRejectsOperationIDReuseWithDifferentRequestFingerprint(t *testing.T) {
	uc := newOperationUsecaseForTest(t)
	operation := testOperationRef()
	_, err := uc.Execute(context.Background(), operation, successfulOperationMutation)
	require.NoError(t, err)

	conflicting := operation
	conflicting.RequestFingerprint = "different-request"
	_, err = uc.Execute(context.Background(), conflicting, successfulOperationMutation)
	require.ErrorIs(t, err, biz.ErrOperationConflict)
}

func TestOperationResolveCreatesNotReceivedTombstone(t *testing.T) {
	uc := newOperationUsecaseForTest(t)
	operation := testOperationRef()

	resolved, err := uc.Resolve(context.Background(), operation)
	require.NoError(t, err)
	require.Equal(t, "not_received", resolved.State)

	calls := 0
	replayed, err := uc.Execute(context.Background(), operation, func(context.Context) (*biz.OperationResult, error) {
		calls++
		return successfulOperationMutation(context.Background())
	})
	require.NoError(t, err)
	require.Equal(t, 0, calls)
	require.Equal(t, "not_received", replayed.State)
}

func TestOperationResolveWaitsForLeaseBeforeInvalidatingPendingExecution(t *testing.T) {
	db := newOperationDatabaseForTest(t)
	repo := data.NewOperationRepo(db)
	uc := NewOperationUsecase(repo)
	operation := testOperationRef()

	admitted, created, err := repo.Admit(context.Background(), operation)
	require.NoError(t, err)
	require.True(t, created)
	require.Equal(t, "pending", admitted.State)

	resolved, err := uc.Resolve(context.Background(), operation)
	require.NoError(t, err)
	require.Equal(t, "pending", resolved.State)

	require.NoError(t, db.Model(&model.PlatformOperation{}).
		Where("operation_id = ?", operation.OperationID).
		Update("lease_expires_at", time.Now().UTC().Add(-time.Second)).Error)
	resolved, err = uc.Resolve(context.Background(), operation)
	require.NoError(t, err)
	require.Equal(t, "failed_input_required", resolved.State)

	calls := 0
	replayed, err := uc.Execute(context.Background(), operation, func(context.Context) (*biz.OperationResult, error) {
		calls++
		return successfulOperationMutation(context.Background())
	})
	require.NoError(t, err)
	require.Zero(t, calls)
	require.Equal(t, "failed_input_required", replayed.State)
}

func TestOperationExecuteRetainsFailedAdmissionWhenMutationFails(t *testing.T) {
	uc := newOperationUsecaseForTest(t)
	operation := testOperationRef()

	_, err := uc.Execute(context.Background(), operation, func(context.Context) (*biz.OperationResult, error) {
		return nil, errors.New("write failed")
	})
	require.ErrorContains(t, err, "write failed")

	stored, err := uc.Get(context.Background(), operation.OperationID)
	require.NoError(t, err)
	require.NotNil(t, stored)
	require.Equal(t, "failed", stored.State)
	require.Equal(t, "internal_failure", stored.ReasonCode)
}

func newOperationUsecaseForTest(t *testing.T) *OperationUsecase {
	t.Helper()
	return NewOperationUsecase(data.NewOperationRepo(newOperationDatabaseForTest(t)))
}

func newOperationDatabaseForTest(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := fmt.Sprintf("file:%s_%d?mode=memory&cache=shared", t.Name(), time.Now().UnixNano())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.PlatformOperation{}))
	return db
}

func testOperationRef() biz.OperationRef {
	return biz.OperationRef{
		OperationID:        "op-1",
		Kind:               "bind",
		BindingRef:         "binding-1",
		PreGeneration:      0,
		TargetGeneration:   1,
		RequestFingerprint: "request-one",
	}
}

func successfulOperationMutation(context.Context) (*biz.OperationResult, error) {
	return &biz.OperationResult{State: "succeeded", AccountKey: "account-1", Status: "active", SnapshotJSON: `{}`}, nil
}

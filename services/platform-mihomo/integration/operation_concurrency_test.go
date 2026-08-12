//go:build integration

package integration

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"platform-mihomo-service/internal/biz"
	"platform-mihomo-service/internal/data"
	"platform-mihomo-service/internal/data/model"
	"platform-mihomo-service/internal/usecase"
)

func TestConcurrentAdmitAndResolveProduceOneConsistentOperation(t *testing.T) {
	stack := newIntegrationStack(t)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	repository := data.NewOperationRepo(stack.DB)
	operation := biz.OperationRef{
		OperationID: "operation-admit-resolve-race", Kind: "OPERATION_KIND_BIND_CREDENTIAL",
		BindingRef: "binding-admit-resolve-race", PreGeneration: 0, TargetGeneration: 1,
		RequestFingerprint: "fingerprint-admit-resolve-race",
	}

	blocker, err := stack.SQLDB.BeginTx(ctx, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = blocker.Rollback() })
	_, err = blocker.ExecContext(ctx, `LOCK TABLE platform_operations IN ACCESS EXCLUSIVE MODE`)
	require.NoError(t, err)

	type admitResult struct {
		operation *biz.OperationResult
		admitted  bool
		err       error
	}
	type resolveResult struct {
		operation *biz.OperationResult
		err       error
	}
	start := make(chan struct{})
	admitted := make(chan admitResult, 1)
	resolved := make(chan resolveResult, 1)
	go func() {
		<-start
		result, created, callErr := repository.Admit(ctx, operation)
		admitted <- admitResult{operation: result, admitted: created, err: callErr}
	}()
	go func() {
		<-start
		result, callErr := repository.Resolve(ctx, operation)
		resolved <- resolveResult{operation: result, err: callErr}
	}()
	close(start)
	waitForOperationLockWaiters(t, ctx, stack.SQLDB, 2)
	require.NoError(t, blocker.Commit())

	admitOutcome := <-admitted
	resolveOutcome := <-resolved
	require.NoError(t, admitOutcome.err)
	require.NoError(t, resolveOutcome.err)
	require.NotNil(t, admitOutcome.operation)
	require.NotNil(t, resolveOutcome.operation)
	require.Equal(t, operation, admitOutcome.operation.Operation)
	require.Equal(t, operation, resolveOutcome.operation.Operation)
	require.Equal(t, admitOutcome.operation.State, resolveOutcome.operation.State)
	if admitOutcome.admitted {
		require.Equal(t, "pending", admitOutcome.operation.State)
	} else {
		require.Equal(t, "not_received", admitOutcome.operation.State)
	}

	var rowCount int
	require.NoError(t, stack.SQLDB.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM platform_operations WHERE operation_id = $1
	`, operation.OperationID).Scan(&rowCount))
	require.Equal(t, 1, rowCount)
}

func TestActiveOperationSerializesResolverWithMutation(t *testing.T) {
	stack := newIntegrationStack(t)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	database := stack.DB.WithContext(ctx)
	repository := data.NewOperationRepo(database)
	operations := usecase.NewOperationUsecase(repository)
	operation := biz.OperationRef{
		OperationID: "operation-active-resolve-race", Kind: "OPERATION_KIND_BIND_CREDENTIAL",
		BindingRef: "binding-active-resolve-race", PreGeneration: 0, TargetGeneration: 1,
		RequestFingerprint: "fingerprint-active-resolve-race",
	}

	mutationStarted := make(chan struct{})
	releaseMutation := make(chan struct{})
	executionDone := make(chan *biz.OperationResult, 1)
	executionErrors := make(chan error, 1)
	go func() {
		result, err := operations.Execute(ctx, operation, func(txCtx context.Context) (*biz.OperationResult, error) {
			close(mutationStarted)
			select {
			case <-releaseMutation:
			case <-txCtx.Done():
				return nil, txCtx.Err()
			}
			return &biz.OperationResult{State: "succeeded", AccountKey: "active-account", SnapshotJSON: "{}"}, nil
		})
		executionDone <- result
		executionErrors <- err
	}()
	select {
	case <-mutationStarted:
	case <-ctx.Done():
		t.Fatal("mutation did not reach the PostgreSQL barrier")
	}
	resolved := make(chan *biz.OperationResult, 1)
	resolveErrors := make(chan error, 1)
	go func() {
		result, err := operations.Resolve(ctx, operation)
		resolved <- result
		resolveErrors <- err
	}()
	waitForOperationLockWaiters(t, ctx, stack.SQLDB, 1)
	select {
	case result := <-resolved:
		t.Fatalf("resolver bypassed the active operation lock: %#v", result)
	default:
	}
	close(releaseMutation)
	select {
	case result := <-executionDone:
		require.NoError(t, <-executionErrors)
		require.Equal(t, "succeeded", result.State)
	case <-ctx.Done():
		t.Fatal("active execution did not complete")
	}
	select {
	case result := <-resolved:
		require.NoError(t, <-resolveErrors)
		require.Equal(t, "succeeded", result.State)
	case <-ctx.Done():
		t.Fatal("resolver did not converge after active execution committed")
	}
}

func TestExpiredOrphanOperationRejectsOldExecutionToken(t *testing.T) {
	stack := newIntegrationStack(t)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	database := stack.DB.WithContext(ctx)
	repository := data.NewOperationRepo(database)
	operations := usecase.NewOperationUsecase(repository)
	invalidations := data.NewGrantInvalidationRepo(database)
	operation := biz.OperationRef{
		OperationID: "operation-expired-orphan", Kind: "OPERATION_KIND_BIND_CREDENTIAL",
		BindingRef: "binding-expired-orphan", PreGeneration: 0, TargetGeneration: 1,
		RequestFingerprint: "fingerprint-expired-orphan",
	}
	admitted, created, err := repository.Admit(ctx, operation)
	require.NoError(t, err)
	require.True(t, created)
	require.NoError(t, database.Model(&model.PlatformOperation{}).
		Where("operation_id = ?", operation.OperationID).
		Update("lease_expires_at", time.Now().UTC().Add(-time.Second)).Error)

	resolved, err := operations.Resolve(ctx, operation)
	require.NoError(t, err)
	require.Equal(t, "failed_input_required", resolved.State)
	require.NotEqual(t, admitted.ExecutionToken, resolved.ExecutionToken)

	err = repository.WithinTransaction(ctx, func(txCtx context.Context) error {
		if err := invalidations.Upsert(txCtx, operation.BindingRef, "expired-operation-sentinel", 1); err != nil {
			return err
		}
		return repository.Complete(txCtx, biz.OperationResult{
			Operation: operation, State: "succeeded", ExecutionToken: admitted.ExecutionToken, SnapshotJSON: "{}",
		})
	})
	require.ErrorIs(t, err, biz.ErrOperationState)
	var sentinelCount int64
	require.NoError(t, database.Model(&model.ConsumerGrantInvalidation{}).
		Where("binding_ref = ? AND consumer = ?", operation.BindingRef, "expired-operation-sentinel").
		Count(&sentinelCount).Error)
	require.Zero(t, sentinelCount)
}

func waitForOperationLockWaiters(t *testing.T, ctx context.Context, db *sql.DB, minimum int) {
	t.Helper()
	require.Eventually(t, func() bool {
		var count int
		err := db.QueryRowContext(ctx, `
			SELECT COUNT(*)
			FROM pg_stat_activity
			WHERE datname = current_database()
			  AND pid <> pg_backend_pid()
			  AND wait_event_type = 'Lock'
			  AND query ILIKE '%platform_operations%'
		`).Scan(&count)
		return err == nil && count >= minimum
	}, 5*time.Second, 10*time.Millisecond)
}

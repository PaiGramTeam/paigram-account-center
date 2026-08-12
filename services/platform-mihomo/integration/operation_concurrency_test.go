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

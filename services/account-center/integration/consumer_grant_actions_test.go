//go:build integration

package integration

import (
	"context"
	"database/sql"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	serviceplatformbinding "paigram/internal/service/platformbinding"
)

func TestConsumerGrantActionsAreDeferredAndRequired(t *testing.T) {
	stack := newIntegrationStack(t)
	ctx := context.Background()
	grantID, _ := insertGrantWithActions(t, ctx, stack, "cn:grant-actions", "mihomo.status.read")

	_, err := stack.SQLDB.ExecContext(ctx, `DELETE FROM consumer_grant_actions WHERE grant_id = $1`, grantID)
	require.Error(t, err, "active grant must retain at least one action")

	tx, err := stack.SQLDB.BeginTx(ctx, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = tx.Rollback() })
	_, err = tx.ExecContext(ctx, `DELETE FROM consumer_grant_actions WHERE grant_id = $1`, grantID)
	require.NoError(t, err)
	_, err = tx.ExecContext(ctx, `
		INSERT INTO consumer_grant_actions (grant_id, action)
		VALUES ($1, 'mihomo.profile.read')
	`, grantID)
	require.NoError(t, err)
	require.NoError(t, tx.Commit())

	var action string
	require.NoError(t, stack.SQLDB.QueryRowContext(ctx, `
		SELECT action FROM consumer_grant_actions WHERE grant_id = $1
	`, grantID).Scan(&action))
	require.Equal(t, "mihomo.profile.read", action)
}

func TestConsumerGrantActionConstraintSerializesConcurrentDeletes(t *testing.T) {
	stack := newIntegrationStack(t)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	grantID, _ := insertGrantWithActions(t, ctx, stack, "cn:grant-actions-race", "mihomo.status.read", "mihomo.profile.read")
	const advisoryLockKey int64 = 73012001
	require.NoError(t, installGrantActionCommitBarrier(ctx, stack.SQLDB, advisoryLockKey))
	lockConn, err := stack.SQLDB.Conn(ctx)
	require.NoError(t, err)
	t.Cleanup(func() { _ = lockConn.Close() })
	_, err = lockConn.ExecContext(ctx, `SELECT pg_advisory_lock($1)`, advisoryLockKey)
	require.NoError(t, err)
	lockHeld := true
	t.Cleanup(func() {
		if lockHeld {
			_, _ = lockConn.ExecContext(context.Background(), `SELECT pg_advisory_unlock($1)`, advisoryLockKey)
		}
	})

	firstTx, err := stack.SQLDB.BeginTx(ctx, nil)
	require.NoError(t, err)
	secondTx, err := stack.SQLDB.BeginTx(ctx, nil)
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = firstTx.Rollback()
		_ = secondTx.Rollback()
	})
	_, err = firstTx.ExecContext(ctx, `DELETE FROM consumer_grant_actions WHERE grant_id = $1 AND action = 'mihomo.status.read'`, grantID)
	require.NoError(t, err)
	_, err = secondTx.ExecContext(ctx, `DELETE FROM consumer_grant_actions WHERE grant_id = $1 AND action = 'mihomo.profile.read'`, grantID)
	require.NoError(t, err)

	commitResults := startConcurrentCommits(firstTx, secondTx)
	waitForAdvisoryWaiters(t, ctx, lockConn, 2)
	_, err = lockConn.ExecContext(ctx, `SELECT pg_advisory_unlock($1)`, advisoryLockKey)
	require.NoError(t, err)
	lockHeld = false
	results := collectCommitResults(commitResults)
	require.Equal(t, 1, countNilErrors(results), "exactly one deletion must commit")

	var actionCount int
	require.NoError(t, stack.SQLDB.QueryRowContext(ctx, `SELECT COUNT(*) FROM consumer_grant_actions WHERE grant_id = $1`, grantID).Scan(&actionCount))
	require.Equal(t, 1, actionCount)
}

func TestGrantServiceSerializesConcurrentActionChanges(t *testing.T) {
	stack := newIntegrationStack(t)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	_, bindingID := insertGrantWithActions(t, ctx, stack, "cn:grant-service-race", "mihomo.credential.read_meta")

	callbackName := fmt.Sprintf("test:grant-read-barrier:%d", time.Now().UnixNano())
	readArrivals := make(chan struct{}, 2)
	releaseReads := make(chan struct{})
	require.NoError(t, stack.DB.Callback().Query().After("gorm:query").Before("gorm:preload").Register(callbackName, func(tx *gorm.DB) {
		if tx.Statement.Schema == nil || tx.Statement.Schema.Table != "consumer_grants" {
			return
		}
		readArrivals <- struct{}{}
		<-releaseReads
	}))
	t.Cleanup(func() { _ = stack.DB.Callback().Query().Remove(callbackName) })

	service := serviceplatformbinding.NewGrantService(stack.DB)
	results := make(chan error, 2)
	for _, action := range []string{"mihomo.status.read", "mihomo.profile.read"} {
		action := action
		go func() {
			_, _, err := service.UpsertGrant(serviceplatformbinding.UpsertGrantInput{
				BindingID: bindingID,
				Consumer:  serviceplatformbinding.ConsumerPaiGramBot,
				Actions:   []string{action},
			})
			results <- err
		}()
	}

	arrivals := 0
	timer := time.NewTimer(250 * time.Millisecond)
	defer timer.Stop()
	waiting := true
	for waiting && arrivals < 2 {
		select {
		case <-readArrivals:
			arrivals++
		case <-timer.C:
			waiting = false
		}
	}
	close(releaseReads)
	require.NoError(t, <-results)
	require.NoError(t, <-results)

	var version uint64
	require.NoError(t, stack.SQLDB.QueryRowContext(ctx, `SELECT ticket_version FROM consumer_grants WHERE binding_id = $1`, bindingID).Scan(&version))
	require.Equal(t, uint64(3), version, "both committed mutations must advance the version")
}

func TestDeletingBindingCascadesConsumerGrantActions(t *testing.T) {
	stack := newIntegrationStack(t)
	ctx := context.Background()
	grantID, bindingID := insertGrantWithActions(t, ctx, stack, "cn:grant-actions-cascade", "mihomo.status.read")

	_, err := stack.SQLDB.ExecContext(ctx, `DELETE FROM platform_account_bindings WHERE id = $1`, bindingID)
	require.NoError(t, err)

	var actionCount int
	require.NoError(t, stack.SQLDB.QueryRowContext(ctx, `SELECT COUNT(*) FROM consumer_grant_actions WHERE grant_id = $1`, grantID).Scan(&actionCount))
	require.Zero(t, actionCount)
}

func insertGrantWithActions(t *testing.T, ctx context.Context, stack *integrationStack, externalAccountKey string, actions ...string) (uint64, uint64) {
	t.Helper()
	ownerID := insertTestUser(t, ctx, stack.SQLDB)
	bindingID := insertTestBinding(t, ctx, stack.SQLDB, ownerID, "mihomo", externalAccountKey)

	tx, err := stack.SQLDB.BeginTx(ctx, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = tx.Rollback() })
	var grantID uint64
	err = tx.QueryRowContext(ctx, `
		INSERT INTO consumer_grants (binding_id, consumer, status, ticket_version, granted_at)
		VALUES ($1, 'paigram-bot', 'active', 1, CURRENT_TIMESTAMP)
		RETURNING id
	`, bindingID).Scan(&grantID)
	require.NoError(t, err)
	for _, action := range actions {
		_, err = tx.ExecContext(ctx, `INSERT INTO consumer_grant_actions (grant_id, action) VALUES ($1, $2)`, grantID, action)
		require.NoError(t, err)
	}
	require.NoError(t, tx.Commit())
	return grantID, bindingID
}

func installGrantActionCommitBarrier(ctx context.Context, db *sql.DB, lockKey int64) error {
	_, err := db.ExecContext(ctx, fmt.Sprintf(`
		CREATE FUNCTION test_wait_before_grant_action_validation() RETURNS TRIGGER AS $function$
		BEGIN
			PERFORM pg_advisory_xact_lock_shared(%d);
			RETURN OLD;
		END;
		$function$ LANGUAGE plpgsql;

		CREATE CONSTRAINT TRIGGER aaa_test_wait_before_grant_action_validation
			AFTER DELETE ON consumer_grant_actions
			DEFERRABLE INITIALLY DEFERRED
			FOR EACH ROW EXECUTE FUNCTION test_wait_before_grant_action_validation();
	`, lockKey))
	return err
}

func waitForAdvisoryWaiters(t *testing.T, ctx context.Context, conn *sql.Conn, expected int) {
	t.Helper()
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		var waiters int
		err := conn.QueryRowContext(ctx, `
			SELECT COUNT(*)
			FROM pg_locks
			WHERE locktype = 'advisory'
			  AND database = (SELECT oid FROM pg_database WHERE datname = current_database())
			  AND NOT granted
		`).Scan(&waiters)
		require.NoError(t, err)
		if waiters >= expected {
			return
		}
		select {
		case <-ctx.Done():
			t.Fatalf("wait for advisory lock contenders: %v", ctx.Err())
		case <-ticker.C:
		}
	}
}

func startConcurrentCommits(transactions ...*sql.Tx) <-chan error {
	results := make(chan error, len(transactions))
	var commits sync.WaitGroup
	commits.Add(len(transactions))
	for _, tx := range transactions {
		tx := tx
		go func() {
			defer commits.Done()
			results <- tx.Commit()
		}()
	}
	go func() {
		commits.Wait()
		close(results)
	}()
	return results
}

func collectCommitResults(results <-chan error) []error {
	errors := make([]error, 0)
	for err := range results {
		errors = append(errors, err)
	}
	return errors
}

func countNilErrors(errors []error) int {
	count := 0
	for _, err := range errors {
		if err == nil {
			count++
		}
	}
	return count
}

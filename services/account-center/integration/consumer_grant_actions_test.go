//go:build integration

package integration

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
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
	require.Equal(t, []string{"23514"}, failedSQLStates(t, results))

	var actionCount int
	require.NoError(t, stack.SQLDB.QueryRowContext(ctx, `SELECT COUNT(*) FROM consumer_grant_actions WHERE grant_id = $1`, grantID).Scan(&actionCount))
	require.Equal(t, 1, actionCount)
}

func TestGrantServiceSerializesConcurrentActionChanges(t *testing.T) {
	stack := newIntegrationStack(t)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	_, bindingID := insertGrantWithActions(t, ctx, stack, "cn:grant-service-race", "mihomo.status.read")

	callbackName := fmt.Sprintf("test:grant-read-barrier:%d", time.Now().UnixNano())
	readArrivals := make(chan struct{}, 8)
	releaseReads := make(chan struct{})
	require.NoError(t, stack.DB.Callback().Query().After("gorm:query").Before("gorm:preload").Register(callbackName, func(tx *gorm.DB) {
		if tx.Statement.Schema == nil || tx.Statement.Schema.Table != "consumer_grants" {
			return
		}
		select {
		case readArrivals <- struct{}{}:
		default:
		}
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

	waitForSignals(t, ctx, readArrivals, 2, "concurrent grant reads")
	close(releaseReads)
	require.NoError(t, <-results)
	require.NoError(t, <-results)

	var version uint64
	require.NoError(t, stack.SQLDB.QueryRowContext(ctx, `SELECT ticket_version FROM consumer_grants WHERE binding_id = $1`, bindingID).Scan(&version))
	require.Equal(t, uint64(3), version, "both committed mutations must advance the version")
}

func TestGrantServiceSerializesConcurrentInitialUpserts(t *testing.T) {
	stack := newIntegrationStack(t)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	ownerID := insertTestUser(t, ctx, stack.SQLDB)
	bindingID := insertTestBinding(t, ctx, stack.SQLDB, ownerID, "mihomo", "cn:grant-initial-race")

	readArrivals, releaseReads := installGrantReadBarrier(t, stack, "initial-upsert")
	type upsertResult struct {
		created bool
		err     error
	}
	results := make(chan upsertResult, 2)
	service := serviceplatformbinding.NewGrantService(stack.DB)
	for _, action := range []string{"mihomo.status.read", "mihomo.profile.read"} {
		action := action
		go func() {
			_, created, err := service.UpsertGrant(serviceplatformbinding.UpsertGrantInput{
				BindingID: bindingID,
				Consumer:  serviceplatformbinding.ConsumerPaiGramBot,
				Actions:   []string{action},
			})
			results <- upsertResult{created: created, err: err}
		}()
	}
	waitForSignals(t, ctx, readArrivals, 2, "initial grant reads")
	close(releaseReads)

	createdCount := 0
	for range 2 {
		result := <-results
		require.NoError(t, result.err)
		if result.created {
			createdCount++
		}
	}
	require.Equal(t, 1, createdCount)
	var version uint64
	require.NoError(t, stack.SQLDB.QueryRowContext(ctx, `SELECT ticket_version FROM consumer_grants WHERE binding_id = $1`, bindingID).Scan(&version))
	require.Equal(t, uint64(2), version)
}

func TestGrantServiceSerializesMissingRevokeWithInitialUpsert(t *testing.T) {
	stack := newIntegrationStack(t)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	ownerID := insertTestUser(t, ctx, stack.SQLDB)
	bindingID := insertTestBinding(t, ctx, stack.SQLDB, ownerID, "mihomo", "cn:grant-revoke-upsert-race")

	readArrivals, releaseReads := installGrantReadBarrier(t, stack, "revoke-upsert")
	results := make(chan error, 2)
	service := serviceplatformbinding.NewGrantService(stack.DB)
	go func() {
		_, _, err := service.UpsertGrant(serviceplatformbinding.UpsertGrantInput{
			BindingID: bindingID,
			Consumer:  serviceplatformbinding.ConsumerPaiGramBot,
			Actions:   []string{"mihomo.status.read"},
		})
		results <- err
	}()
	go func() {
		_, err := service.RevokeGrant(serviceplatformbinding.RevokeGrantInput{
			BindingID: bindingID,
			Consumer:  serviceplatformbinding.ConsumerPaiGramBot,
		})
		results <- err
	}()
	waitForSignals(t, ctx, readArrivals, 2, "missing revoke and initial upsert reads")
	close(releaseReads)
	require.NoError(t, <-results)
	require.NoError(t, <-results)

	var version uint64
	require.NoError(t, stack.SQLDB.QueryRowContext(ctx, `SELECT ticket_version FROM consumer_grants WHERE binding_id = $1`, bindingID).Scan(&version))
	require.Equal(t, uint64(2), version, "both serialized operations must be reflected")
}

func TestMissingRevokeReturnsSupersedingGrantAfterInvalidation(t *testing.T) {
	stack := newIntegrationStack(t)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	ownerID := insertTestUser(t, ctx, stack.SQLDB)
	bindingID := insertTestBinding(t, ctx, stack.SQLDB, ownerID, "mihomo", "cn:grant-revoke-superseded")
	invalidator := &blockingGrantInvalidator{
		called:  make(chan struct{}),
		release: make(chan struct{}),
	}
	service := serviceplatformbinding.NewGrantService(stack.DB, invalidator)
	type revokeResult struct {
		grantVersion uint64
		grantStatus  string
		err          error
	}
	result := make(chan revokeResult, 1)
	go func() {
		grant, err := service.RevokeGrant(serviceplatformbinding.RevokeGrantInput{
			BindingID: bindingID,
			Consumer:  serviceplatformbinding.ConsumerPaiGramBot,
		})
		if err != nil {
			result <- revokeResult{err: err}
			return
		}
		result <- revokeResult{
			grantVersion: grant.TicketVersion,
			grantStatus:  string(grant.Status),
		}
	}()
	select {
	case <-invalidator.called:
	case <-ctx.Done():
		t.Fatalf("wait for grant invalidation: %v", ctx.Err())
	}

	active, created, err := serviceplatformbinding.NewGrantService(stack.DB).UpsertGrant(serviceplatformbinding.UpsertGrantInput{
		BindingID: bindingID,
		Consumer:  serviceplatformbinding.ConsumerPaiGramBot,
		Actions:   []string{"mihomo.status.read"},
	})
	require.NoError(t, err)
	require.False(t, created)
	require.Equal(t, uint64(2), active.TicketVersion)
	close(invalidator.release)

	select {
	case revoked := <-result:
		require.NoError(t, revoked.err)
		require.Equal(t, uint64(2), revoked.grantVersion)
		require.Equal(t, "active", revoked.grantStatus)
	case <-ctx.Done():
		t.Fatalf("wait for superseded revoke result: %v", ctx.Err())
	}
}

func TestGrantServiceDoesNotDeadlockWithDirectActionDelete(t *testing.T) {
	stack := newIntegrationStack(t)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	grantID, bindingID := insertGrantWithActions(t, ctx, stack, "cn:grant-lock-order", "mihomo.authkey.issue", "mihomo.status.read")
	attempts := &recordingGrantTransactionObserver{}

	directTx, err := stack.SQLDB.BeginTx(ctx, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = directTx.Rollback() })
	_, err = directTx.ExecContext(ctx, `DELETE FROM consumer_grant_actions WHERE grant_id = $1 AND action = 'mihomo.authkey.issue'`, grantID)
	require.NoError(t, err)

	serviceResult := make(chan error, 1)
	go func() {
		_, _, err := serviceplatformbinding.NewGrantService(stack.DB, attempts).UpsertGrant(serviceplatformbinding.UpsertGrantInput{
			BindingID: bindingID,
			Consumer:  serviceplatformbinding.ConsumerPaiGramBot,
			Actions:   []string{"mihomo.profile.read"},
		})
		serviceResult <- err
	}()
	waitForDatabaseLockWaiter(t, ctx, stack.SQLDB)
	require.NoError(t, directTx.Commit())
	select {
	case err = <-serviceResult:
		require.NoError(t, err)
	case <-ctx.Done():
		t.Fatalf("grant service remained blocked after direct action mutation committed: %v", ctx.Err())
	}

	var version uint64
	require.NoError(t, stack.SQLDB.QueryRowContext(ctx, `SELECT ticket_version FROM consumer_grants WHERE id = $1`, grantID).Scan(&version))
	require.Equal(t, uint64(2), version)
	require.NotContains(t, attempts.SQLStates(), "40P01")
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

func failedSQLStates(t *testing.T, transactionErrors []error) []string {
	t.Helper()
	states := make([]string, 0, len(transactionErrors))
	for _, err := range transactionErrors {
		if err == nil {
			continue
		}
		var postgresError *pgconn.PgError
		require.True(t, errors.As(err, &postgresError), "expected PostgreSQL error, got %T: %v", err, err)
		states = append(states, postgresError.Code)
	}
	return states
}

func installGrantReadBarrier(t *testing.T, stack *integrationStack, suffix string) (<-chan struct{}, chan struct{}) {
	t.Helper()
	callbackName := fmt.Sprintf("test:grant-read-%s:%d", suffix, time.Now().UnixNano())
	readArrivals := make(chan struct{}, 8)
	releaseReads := make(chan struct{})
	require.NoError(t, stack.DB.Callback().Query().After("gorm:query").Before("gorm:preload").Register(callbackName, func(tx *gorm.DB) {
		if tx.Statement.Schema == nil || tx.Statement.Schema.Table != "consumer_grants" {
			return
		}
		select {
		case readArrivals <- struct{}{}:
		default:
		}
		<-releaseReads
	}))
	t.Cleanup(func() { _ = stack.DB.Callback().Query().Remove(callbackName) })
	return readArrivals, releaseReads
}

func waitForSignals(t *testing.T, ctx context.Context, signals <-chan struct{}, expected int, description string) {
	t.Helper()
	for range expected {
		select {
		case <-signals:
		case <-ctx.Done():
			t.Fatalf("wait for %s: %v", description, ctx.Err())
		}
	}
}

func waitForDatabaseLockWaiter(t *testing.T, ctx context.Context, db *sql.DB) {
	t.Helper()
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		var waiters int
		err := db.QueryRowContext(ctx, `
			SELECT COUNT(*)
			FROM pg_stat_activity
			WHERE datname = current_database()
			  AND wait_event_type = 'Lock'
		`).Scan(&waiters)
		require.NoError(t, err)
		if waiters > 0 {
			return
		}
		select {
		case <-ctx.Done():
			t.Fatalf("wait for database lock contention: %v", ctx.Err())
		case <-ticker.C:
		}
	}
}

type blockingGrantInvalidator struct {
	called  chan struct{}
	release chan struct{}
	once    sync.Once
}

type recordingGrantTransactionObserver struct {
	mutex     sync.Mutex
	sqlStates []string
}

func (o *recordingGrantTransactionObserver) ObserveGrantTransactionAttempt(err error) {
	if err == nil {
		return
	}
	var postgresError *pgconn.PgError
	if !errors.As(err, &postgresError) {
		return
	}
	o.mutex.Lock()
	defer o.mutex.Unlock()
	o.sqlStates = append(o.sqlStates, postgresError.Code)
}

func (o *recordingGrantTransactionObserver) SQLStates() []string {
	o.mutex.Lock()
	defer o.mutex.Unlock()
	return append([]string(nil), o.sqlStates...)
}

func (b *blockingGrantInvalidator) InvalidateConsumerGrant(ctx context.Context, _ serviceplatformbinding.GrantInvalidationInput) error {
	b.once.Do(func() { close(b.called) })
	select {
	case <-b.release:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

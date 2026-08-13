//go:build integration

package integration

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	serviceplatformbinding "paigram/internal/service/platformbinding"
)

func TestDeadLetterRequeueIsSerializedAndAudited(t *testing.T) {
	stack := newIntegrationStack(t)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	ownerID := insertTestUser(t, ctx, stack.SQLDB)
	bindingID := insertTestBinding(t, ctx, stack.SQLDB, ownerID, "mihomo", "recovery-"+uuid.NewString())
	operationID := "op_" + uuid.NewString()
	_, err := stack.SQLDB.ExecContext(ctx, `
		INSERT INTO platform_operation_intents (
			operation_id, binding_id, binding_ref, owner_user_id, platform, kind,
			pre_generation, target_generation, request_fingerprint, delivery_mode,
			state, reason_code, actor_type, actor_id
		) VALUES ($1, $2, $3, $4, 'mihomo', 'OPERATION_KIND_BIND_CREDENTIAL',
			0, 1, $5, 'sync_secret', 'invariant_violation', 'resolve_rejected', 'user', 'session:test')
	`, operationID, bindingID, "bind_"+uuid.NewString(), ownerID, uuid.NewString())
	require.NoError(t, err)
	_, err = stack.SQLDB.ExecContext(ctx, `
		INSERT INTO platform_operation_outbox (operation_id, status, available_at, attempt_count, last_reason_code)
		VALUES ($1, 'dead_letter', CURRENT_TIMESTAMP, 100, 'retry_exhausted')
	`, operationID)
	require.NoError(t, err)

	recovery := serviceplatformbinding.NewOperationRecoveryService(stack.DB)
	start := make(chan struct{})
	errorsCh := make(chan error, 2)
	for range 2 {
		go func() {
			<-start
			_, requeueErr := recovery.RequeueDeadLetter(ctx, bindingID, operationID, ownerID)
			errorsCh <- requeueErr
		}()
	}
	close(start)
	results := []error{<-errorsCh, <-errorsCh}
	var successes, conflicts int
	for _, result := range results {
		switch {
		case result == nil:
			successes++
		case errors.Is(result, serviceplatformbinding.ErrCredentialOperationNotRecoverable):
			conflicts++
		default:
			require.NoError(t, result)
		}
	}
	require.Equal(t, 1, successes)
	require.Equal(t, 1, conflicts)

	var status string
	var attempts int
	require.NoError(t, stack.SQLDB.QueryRowContext(ctx, `SELECT status, attempt_count FROM platform_operation_outbox WHERE operation_id = $1`, operationID).Scan(&status, &attempts))
	require.Equal(t, "pending", status)
	require.Zero(t, attempts)
	var audits int
	require.NoError(t, stack.SQLDB.QueryRowContext(ctx, `SELECT COUNT(*) FROM audit_events WHERE action = 'platform_operation_requeued' AND target_id = $1`, operationID).Scan(&audits))
	require.Equal(t, 1, audits)
}

//go:build integration

package integration

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	serviceplatformbinding "paigram/internal/service/platformbinding"
)

func TestPlatformOperationIntentSchemaHasNoCredentialPayloadColumns(t *testing.T) {
	stack := newIntegrationStack(t)
	requireTableExists(t, stack.SQLDB, stack.Schema, "platform_operation_intents")
	requireTableExists(t, stack.SQLDB, stack.Schema, "platform_operation_outbox")
	requireColumnAbsent(t, stack.SQLDB, stack.Schema, "platform_operation_intents", "credential_payload")
	requireColumnAbsent(t, stack.SQLDB, stack.Schema, "platform_operation_intents", "payload_json")
	requireColumnAbsent(t, stack.SQLDB, stack.Schema, "platform_operation_intents", "credential_hash")
	requireColumnExists(t, stack.SQLDB, stack.Schema, "platform_operation_intents", "delivery_mode")
	requireColumnExists(t, stack.SQLDB, stack.Schema, "platform_operation_intents", "profile_ref")
	requireColumnExists(t, stack.SQLDB, stack.Schema, "platform_operation_intents", "profile_revision")
	requireColumnAbsent(t, stack.SQLDB, stack.Schema, "platform_operation_outbox", "payload_json")
	requireIndexExists(t, stack.SQLDB, stack.Schema, "platform_operation_intents", "uk_platform_operation_intents_active_binding")
	requireIndexExists(t, stack.SQLDB, stack.Schema, "platform_operation_intents", "uk_platform_operation_intents_active_owner_platform_bind")
}

func TestPlatformPrimaryProfileIntentRequiresDurableProfileTuple(t *testing.T) {
	stack := newIntegrationStack(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	ownerID := insertTestUser(t, ctx, stack.SQLDB)
	bindingID := insertTestBinding(t, ctx, stack.SQLDB, ownerID, "mihomo", "account-"+uuid.NewString())

	insertIntent := func(operationID, profileRef string, profileRevision uint64) error {
		_, err := stack.SQLDB.ExecContext(ctx, `
			INSERT INTO platform_operation_intents (
				operation_id, binding_id, binding_ref, owner_user_id, platform, kind,
				pre_generation, target_generation, request_fingerprint, profile_ref, profile_revision,
				delivery_mode, state, actor_type, actor_id
			) VALUES ($1, $2, 'bind_test', $3, 'mihomo', 'OPERATION_KIND_SET_PRIMARY_PROFILE',
				4, 4, $4, $5, $6, 'outbox', 'succeeded', 'user', 'session:test')
		`, operationID, bindingID, ownerID, uuid.NewString(), profileRef, profileRevision)
		return err
	}

	require.NoError(t, insertIntent("op_primary_valid", "profile-stable", 7))
	err := insertIntent("op_primary_missing_ref", "", 7)
	requirePostgresViolation(t, err, "23514", "chk_platform_operation_intents_profile")
	err = insertIntent("op_primary_missing_revision", "profile-stable", 0)
	requirePostgresViolation(t, err, "23514", "chk_platform_operation_intents_profile")
}

func TestPlatformOperationIntentKeepsOneActiveReservationPerBinding(t *testing.T) {
	stack := newIntegrationStack(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	ownerID := insertTestUser(t, ctx, stack.SQLDB)
	bindingID := insertTestBinding(t, ctx, stack.SQLDB, ownerID, "mihomo", "account-"+uuid.NewString())

	insertIntent := func(operationID, state string) error {
		_, err := stack.SQLDB.ExecContext(ctx, `
			INSERT INTO platform_operation_intents (
				operation_id, binding_id, binding_ref, owner_user_id, platform, kind, pre_generation, target_generation,
				request_fingerprint, delivery_mode, state, actor_type, actor_id
			) VALUES ($1, $2, 'bind_test', $3, 'mihomo', 'OPERATION_KIND_REFRESH_CREDENTIAL', 4, 5, $4, 'outbox', $5, 'user', 'session:test')
		`, operationID, bindingID, ownerID, uuid.NewString(), state)
		return err
	}

	require.NoError(t, insertIntent("op_first", "uncertain"))
	err := insertIntent("op_conflict", "pending_delivery")
	requirePostgresViolation(t, err, "23505", "uk_platform_operation_intents_active_binding")

	_, err = stack.SQLDB.ExecContext(ctx, `UPDATE platform_operation_intents SET state = 'superseded' WHERE operation_id = 'op_first'`)
	require.NoError(t, err)
	require.NoError(t, insertIntent("op_retry", "pending_delivery"))
}

func TestPlatformOperationIntentKeepsOneActiveBindReservationPerOwnerAndPlatform(t *testing.T) {
	stack := newIntegrationStack(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	ownerID := insertTestUser(t, ctx, stack.SQLDB)
	firstBindingID := insertTestBinding(t, ctx, stack.SQLDB, ownerID, "mihomo", "account-"+uuid.NewString())
	secondBindingID := insertTestBinding(t, ctx, stack.SQLDB, ownerID, "mihomo", "account-"+uuid.NewString())

	insertBindIntent := func(operationID string, bindingID uint64) error {
		_, err := stack.SQLDB.ExecContext(ctx, `
			INSERT INTO platform_operation_intents (
				operation_id, binding_id, binding_ref, owner_user_id, platform, kind,
				pre_generation, target_generation, request_fingerprint, delivery_mode, state, actor_type, actor_id
			) VALUES ($1, $2, $3, $4, 'mihomo', 'OPERATION_KIND_BIND_CREDENTIAL',
				0, 1, $5, 'sync_secret', 'pending_delivery', 'user', 'session:test')
		`, operationID, bindingID, "bind_"+uuid.NewString(), ownerID, uuid.NewString())
		return err
	}

	require.NoError(t, insertBindIntent("op_owner_first", firstBindingID))
	err := insertBindIntent("op_owner_conflict", secondBindingID)
	requirePostgresViolation(t, err, "23505", "uk_platform_operation_intents_active_owner_platform_bind")
}

func TestCreateBindingAndIntentAdmissionIsAtomic(t *testing.T) {
	stack := newIntegrationStack(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	ownerID := insertTestUser(t, ctx, stack.SQLDB)
	service := serviceplatformbinding.NewOperationIntentService(stack.DB)
	input := serviceplatformbinding.CreateBindingInput{
		OwnerUserID: ownerID, Platform: "mihomo", PlatformServiceKey: "platform-mihomo-service", DisplayName: "Atomic binding",
	}

	binding, intent, err := service.CreateBindingAndAdmit(ctx, input, "user", "session:test", "op_atomic")
	require.NoError(t, err)
	require.NotNil(t, binding)
	require.NotNil(t, intent)
	require.Equal(t, binding.ID, intent.BindingID)

	var beforeRetry int64
	require.NoError(t, stack.DB.Table("platform_account_bindings").Count(&beforeRetry).Error)
	_, _, err = service.CreateBindingAndAdmit(ctx, input, "user", "session:test", "op_atomic")
	var pending *serviceplatformbinding.CredentialOperationPendingError
	require.ErrorAs(t, err, &pending)
	require.Equal(t, intent.OperationID, pending.OperationID)
	require.Equal(t, binding.ID, pending.BindingID)
	var afterRetry int64
	require.NoError(t, stack.DB.Table("platform_account_bindings").Count(&afterRetry).Error)
	require.Equal(t, beforeRetry, afterRetry)
}

func TestPlatformOperationOutboxClaimsAreExclusive(t *testing.T) {
	stack := newIntegrationStack(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	ownerID := insertTestUser(t, ctx, stack.SQLDB)
	bindingIDs := []uint64{
		insertTestBinding(t, ctx, stack.SQLDB, ownerID, "mihomo", "claim-"+uuid.NewString()),
		insertTestBinding(t, ctx, stack.SQLDB, ownerID, "mihomo", "claim-"+uuid.NewString()),
	}
	for index, bindingID := range bindingIDs {
		operationID := fmt.Sprintf("op_claim_%d", index)
		_, err := stack.SQLDB.ExecContext(ctx, `
			INSERT INTO platform_operation_intents (
				operation_id, binding_id, binding_ref, owner_user_id, platform, kind,
				pre_generation, target_generation, request_fingerprint, delivery_mode, state, actor_type, actor_id
			) VALUES ($1, $2, $3, $4, 'mihomo', 'OPERATION_KIND_REFRESH_CREDENTIAL',
				0, 1, $5, 'outbox', 'pending_delivery', 'user', 'session:test')
		`, operationID, bindingID, "bind_"+uuid.NewString(), ownerID, uuid.NewString())
		require.NoError(t, err)
		_, err = stack.SQLDB.ExecContext(ctx, `
			INSERT INTO platform_operation_outbox (operation_id, status, available_at)
			VALUES ($1, 'pending', CURRENT_TIMESTAMP - INTERVAL '1 minute')
		`, operationID)
		require.NoError(t, err)
	}

	service := serviceplatformbinding.NewOperationIntentService(stack.DB)
	start := make(chan struct{})
	results := make(chan []string, 2)
	errors := make(chan error, 2)
	for range 2 {
		go func() {
			<-start
			claimed, err := service.ClaimDueOperationIDs(ctx, time.Now().UTC(), 1)
			results <- claimed
			errors <- err
		}()
	}
	close(start)
	claimed := append(<-results, (<-results)...)
	require.NoError(t, <-errors)
	require.NoError(t, <-errors)
	require.Len(t, claimed, 2)
	require.NotEqual(t, claimed[0], claimed[1])
}

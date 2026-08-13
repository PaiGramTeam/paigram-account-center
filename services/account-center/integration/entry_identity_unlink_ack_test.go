//go:build integration

package integration

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"paigram/internal/model"
	"paigram/internal/service/botlink"
	"paigram/internal/service/credentials"
	"paigram/internal/service/entryidentity"
	"paigram/internal/service/platformbinding"
)

func TestConcurrentGrantAcknowledgementsFinalizeUnlinkOperationOnPostgres(t *testing.T) {
	stack := newIntegrationStack(t)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	owner := model.User{PrimaryLoginType: model.LoginTypeEmail, Status: model.UserStatusActive}
	require.NoError(t, stack.DB.WithContext(ctx).Create(&owner).Error)
	_, err := credentials.NewService(stack.DB).Create(credentials.CreateInput{
		ClientID: "multi-ack-entry-service", BotID: "multi-ack-entry-bot", EntryIssuer: "urn:paigram:entry:multi-ack",
		DisplayName: "Multi Ack Entry", OwnerUserID: owner.ID,
		Audiences: []string{"account-center"}, Scopes: []string{"bot.access.link_identity"},
	})
	require.NoError(t, err)
	_, err = botlink.NewService(stack.DB, zap.NewNop()).UpsertLink(ctx, botlink.UpsertLinkInput{
		BotID: "multi-ack-entry-bot", UserID: owner.ID, ExternalUserID: "multi-ack-subject",
	})
	require.NoError(t, err)

	grants := make([]model.ConsumerGrant, 2)
	for index := range grants {
		binding := model.PlatformAccountBinding{
			BindingRef: "binding-multi-ack-" + string(rune('a'+index)), OwnerUserID: owner.ID, Platform: "mihomo",
			PlatformServiceKey: "platform-mihomo-service", DisplayName: "Multi Ack",
			ExternalAccountKey: sql.NullString{String: "cn:multi-ack-" + string(rune('a'+index)), Valid: true},
			Status:             model.PlatformAccountBindingStatusActive, Generation: 1,
		}
		require.NoError(t, stack.DB.WithContext(ctx).Create(&binding).Error)
		grants[index] = model.ConsumerGrant{
			BindingID: binding.ID, Consumer: "multi-ack-entry-service", Status: model.ConsumerGrantStatusActive,
			TicketVersion: 1, GrantedAt: time.Now().UTC(),
			Actions: []model.ConsumerGrantAction{{Action: "mihomo.status.read"}},
		}
		require.NoError(t, stack.DB.WithContext(ctx).Create(&grants[index]).Error)
	}

	const operationID = "00000000-0000-4000-8000-000000000061"
	admitted, err := entryidentity.NewService(stack.DB, zap.NewNop(), entryidentity.Config{
		GrantInvalidator: &entryFenceRecorder{err: errors.New("hold both acknowledgements pending")},
	}).Unlink(ctx, owner.ID, "multi-ack-entry-bot", operationID, "", "")
	require.NoError(t, err)
	require.True(t, admitted.PropagationPending)

	blocker := stack.DB.WithContext(ctx).Begin()
	require.NoError(t, blocker.Error)
	defer blocker.Rollback()
	var locked model.EntryIdentityUnlinkOperation
	require.NoError(t, blocker.Clauses(clause.Locking{Strength: "UPDATE"}).Where("operation_id = ?", operationID).Take(&locked).Error)

	results := make(chan error, len(grants))
	for index := range grants {
		database, openErr := gorm.Open(postgres.Open(stack.DatabaseCfg.DSN), &gorm.Config{})
		require.NoError(t, openErr)
		sqlDB, dbErr := database.DB()
		require.NoError(t, dbErr)
		t.Cleanup(func() { require.NoError(t, sqlDB.Close()) })
		grantID := grants[index].ID
		go func() {
			results <- platformbinding.NewGrantService(database.WithContext(ctx)).ReconcileGrantInvalidation(ctx, grantID)
		}()
	}
	waitForPostgresLockWaiters(t, ctx, stack.SQLDB, "entry_identity_unlink_operations", 2)
	require.NoError(t, blocker.Commit().Error)
	for range grants {
		require.NoError(t, <-results)
	}

	var operation model.EntryIdentityUnlinkOperation
	require.NoError(t, stack.DB.WithContext(ctx).Where("operation_id = ?", operationID).Take(&operation).Error)
	assert.Equal(t, model.EntryIdentityUnlinkComplete, operation.State)
	assert.True(t, operation.CompletedAt.Valid)
	var unconfirmed int64
	require.NoError(t, stack.DB.WithContext(ctx).Model(&model.EntryIdentityUnlinkTarget{}).
		Where("operation_id = ? AND confirmed_at IS NULL", operationID).Count(&unconfirmed).Error)
	assert.Zero(t, unconfirmed)
}

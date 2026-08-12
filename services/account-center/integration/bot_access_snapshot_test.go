//go:build integration

package integration

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"paigram/internal/model"
	"paigram/internal/service/botaccess"
)

func TestTicketGrantReadKeepsVersionAndActionsInOneSnapshot(t *testing.T) {
	stack := newIntegrationStack(t)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	grantID, bindingID := insertGrantWithActions(t, ctx, stack, "cn:ticket-snapshot", "mihomo.status.read")
	_, err := stack.SQLDB.ExecContext(ctx, `UPDATE platform_account_bindings SET status = 'active' WHERE id = $1`, bindingID)
	require.NoError(t, err)

	var ownerUserID uint64
	require.NoError(t, stack.SQLDB.QueryRowContext(ctx, `SELECT owner_user_id FROM platform_account_bindings WHERE id = $1`, bindingID).Scan(&ownerUserID))
	bot := model.Bot{ID: "snapshot-bot", DisplayName: "Snapshot Bot", Type: "OTHER", Status: "ACTIVE", OwnerUserID: ownerUserID}
	require.NoError(t, stack.DB.Create(&bot).Error)
	identity := model.BotIdentity{UserID: ownerUserID, BotID: bot.ID, ExternalUserID: "snapshot-user", LinkedAt: time.Now().UTC()}
	require.NoError(t, stack.DB.Create(&identity).Error)

	callbackName := "test:ticket-grant-snapshot"
	grantRead := make(chan struct{})
	continueRead := make(chan struct{})
	var pauseOnce sync.Once
	require.NoError(t, stack.DB.Callback().Query().After("gorm:query").Before("gorm:preload").Register(callbackName, func(tx *gorm.DB) {
		if tx.Statement.Schema == nil || tx.Statement.Schema.Table != "consumer_grants" {
			return
		}
		pauseOnce.Do(func() {
			close(grantRead)
			<-continueRead
		})
	}))
	t.Cleanup(func() { _ = stack.DB.Callback().Query().Remove(callbackName) })

	group, err := botaccess.NewServiceGroup(stack.DB, newTestConfig(t, stack.RedisPrefix).Auth)
	require.NoError(t, err)
	type readResult struct {
		grant *model.ConsumerGrant
		err   error
	}
	result := make(chan readResult, 1)
	go func() {
		_, _, grant, err := group.BindingAccessService.GetGrantedBindingForConsumer(
			bot.ID,
			"paigram-bot",
			identity.ExternalUserID,
			bindingID,
			0,
		)
		result <- readResult{grant: grant, err: err}
	}()

	select {
	case <-grantRead:
	case early := <-result:
		require.NoError(t, early.err)
		t.Fatal("ticket grant read completed before the grant query barrier")
	case <-ctx.Done():
		t.Fatalf("wait for grant query: %v", ctx.Err())
	}
	updateTx, err := stack.SQLDB.BeginTx(ctx, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = updateTx.Rollback() })
	_, err = updateTx.ExecContext(ctx, `UPDATE consumer_grants SET ticket_version = 2 WHERE id = $1`, grantID)
	require.NoError(t, err)
	_, err = updateTx.ExecContext(ctx, `DELETE FROM consumer_grant_actions WHERE grant_id = $1`, grantID)
	require.NoError(t, err)
	_, err = updateTx.ExecContext(ctx, `INSERT INTO consumer_grant_actions (grant_id, action) VALUES ($1, 'mihomo.profile.read')`, grantID)
	require.NoError(t, err)
	require.NoError(t, updateTx.Commit())
	close(continueRead)

	var first readResult
	select {
	case first = <-result:
	case <-ctx.Done():
		t.Fatalf("wait for ticket grant result: %v", ctx.Err())
	}
	require.NoError(t, first.err)
	require.Equal(t, uint64(1), first.grant.TicketVersion)
	require.Equal(t, []string{"mihomo.status.read"}, botaccess.GrantActions(*first.grant))

	_, _, latest, err := group.BindingAccessService.GetGrantedBindingForConsumer(bot.ID, "paigram-bot", identity.ExternalUserID, bindingID, 0)
	require.NoError(t, err)
	require.Equal(t, uint64(2), latest.TicketVersion)
	require.Equal(t, []string{"mihomo.profile.read"}, botaccess.GrantActions(*latest))
}

//go:build integration

package integration

import (
	"context"
	"database/sql"
	"errors"
	"net/url"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"paigram/internal/model"
	"paigram/internal/service/botaccess"
	"paigram/internal/service/botlink"
	"paigram/internal/service/credentials"
	"paigram/internal/service/entryidentity"
	"paigram/internal/service/platformbinding"
)

type entryFenceRecorder struct {
	inputs []platformbinding.GrantInvalidationInput
	err    error
}

func (recorder *entryFenceRecorder) InvalidateConsumerGrant(_ context.Context, input platformbinding.GrantInvalidationInput) error {
	recorder.inputs = append(recorder.inputs, input)
	return recorder.err
}

func TestEntryIdentityChallengeApprovalAndUnlinkFenceOnPostgres(t *testing.T) {
	stack := newIntegrationStack(t)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	owner := model.User{PrimaryLoginType: model.LoginTypeEmail, Status: model.UserStatusActive}
	require.NoError(t, stack.DB.WithContext(ctx).Create(&owner).Error)
	credential, err := credentials.NewService(stack.DB).Create(credentials.CreateInput{
		ClientID: "telegram-service", BotID: "paigrambot", EntryIssuer: "urn:paigram:entry:telegram",
		DisplayName: "Telegram", OwnerUserID: owner.ID,
		Audiences: []string{"account-center"}, Scopes: []string{"bot.access.link_identity"},
	})
	require.NoError(t, err)
	require.Equal(t, "urn:paigram:entry:telegram", credential.View.EntryIssuer)

	linkingUser := model.User{PrimaryLoginType: model.LoginTypeEmail, Status: model.UserStatusActive}
	require.NoError(t, stack.DB.WithContext(ctx).Create(&linkingUser).Error)
	oidcLink, err := botlink.NewService(stack.DB, zap.NewNop()).UpsertLink(ctx, botlink.UpsertLinkInput{
		BotID: "paigrambot", UserID: linkingUser.ID, ExternalUserID: "telegram-user-42",
	})
	require.NoError(t, err)
	assert.Equal(t, "urn:paigram:entry:telegram", oidcLink.Issuer)
	recorder := &entryFenceRecorder{}
	service := entryidentity.NewService(stack.DB, nil, entryidentity.Config{
		FrontendBaseURL: "https://account.example.test", GrantInvalidator: recorder,
	})
	started, err := service.Start(ctx, entryidentity.StartInput{
		Consumer: "telegram-service", BotID: "paigrambot", ExternalSubject: "telegram-user-42",
	})
	require.NoError(t, err)
	parsed, err := url.Parse(started.ApprovalURL)
	require.NoError(t, err)
	fragment, err := url.ParseQuery(parsed.Fragment)
	require.NoError(t, err)
	challenge := fragment.Get("challenge")
	require.NotEmpty(t, challenge)

	identity, err := service.Approve(ctx, linkingUser.ID, challenge, "127.0.0.1", "integration")
	require.NoError(t, err)
	assert.Equal(t, oidcLink.ID, identity.ID)
	assert.Equal(t, "urn:paigram:entry:telegram", identity.Issuer)
	_, err = service.Approve(ctx, linkingUser.ID, challenge, "", "")
	assert.ErrorIs(t, err, entryidentity.ErrChallengeConsumed)

	binding := model.PlatformAccountBinding{
		BindingRef: "binding-entry-link", OwnerUserID: linkingUser.ID, Platform: "mihomo",
		PlatformServiceKey: "platform-mihomo-service", DisplayName: "Entry Link",
		ExternalAccountKey: sql.NullString{String: "cn:entry-link", Valid: true},
		Status:             model.PlatformAccountBindingStatusActive, Generation: 1,
	}
	require.NoError(t, stack.DB.WithContext(ctx).Create(&binding).Error)
	grant := model.ConsumerGrant{
		BindingID: binding.ID, Consumer: "telegram-service", Status: model.ConsumerGrantStatusActive,
		TicketVersion: 1, GrantedAt: time.Now().UTC(),
		Actions: []model.ConsumerGrantAction{{Action: "mihomo.status.read"}},
	}
	require.NoError(t, stack.DB.WithContext(ctx).Create(&grant).Error)

	recorder.err = assert.AnError
	unlinked, err := service.Unlink(ctx, linkingUser.ID, "paigrambot", "00000000-0000-4000-8000-000000000003", "127.0.0.1", "integration")
	require.NoError(t, err)
	assert.True(t, unlinked.PropagationPending)
	require.Len(t, recorder.inputs, 1)
	assert.Equal(t, unlinked.MinimumEntryEpoch, recorder.inputs[0].MinimumEntryEpoch)
	recorder.err = nil
	unlinked, err = service.Unlink(ctx, linkingUser.ID, "paigrambot", "00000000-0000-4000-8000-000000000003", "127.0.0.1", "integration")
	require.NoError(t, err)
	assert.False(t, unlinked.PropagationPending)
	assert.Equal(t, "UNLINKED", unlinked.State)
	botAccess, err := botaccess.NewServiceGroup(stack.DB, newTestConfig(t, stack.RedisPrefix).Auth)
	require.NoError(t, err)
	_, err = botAccess.BindingAccessService.ResolveBotUser("paigrambot", "telegram-user-42")
	assert.ErrorIs(t, err, botaccess.ErrBotIdentityNotFound)
}

func TestConcurrentEntryIdentityApprovalsConsumeConflictOnPostgres(t *testing.T) {
	stack := newIntegrationStack(t)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	owner := model.User{PrimaryLoginType: model.LoginTypeEmail, Status: model.UserStatusActive}
	require.NoError(t, stack.DB.WithContext(ctx).Create(&owner).Error)
	_, err := credentials.NewService(stack.DB).Create(credentials.CreateInput{
		ClientID: "concurrent-entry-service", BotID: "concurrent-entry-bot", EntryIssuer: "urn:paigram:entry:concurrent",
		DisplayName: "Concurrent Entry", OwnerUserID: owner.ID,
		Audiences: []string{"account-center"}, Scopes: []string{"bot.access.link_identity"},
	})
	require.NoError(t, err)
	users := []model.User{
		{PrimaryLoginType: model.LoginTypeEmail, Status: model.UserStatusActive},
		{PrimaryLoginType: model.LoginTypeEmail, Status: model.UserStatusActive},
	}
	for index := range users {
		require.NoError(t, stack.DB.WithContext(ctx).Create(&users[index]).Error)
	}
	service := entryidentity.NewService(stack.DB, nil, entryidentity.Config{FrontendBaseURL: "https://account.example.test"})
	tokens := make([]string, len(users))
	for index := range users {
		started, startErr := service.Start(ctx, entryidentity.StartInput{
			Consumer: "concurrent-entry-service", BotID: "concurrent-entry-bot", ExternalSubject: "shared-subject",
		})
		require.NoError(t, startErr)
		parsed, parseErr := url.Parse(started.ApprovalURL)
		require.NoError(t, parseErr)
		fragment, parseErr := url.ParseQuery(parsed.Fragment)
		require.NoError(t, parseErr)
		tokens[index] = fragment.Get("challenge")
	}

	errorsByUser := make([]error, len(users))
	var workers sync.WaitGroup
	for index := range users {
		workers.Add(1)
		go func(index int) {
			defer workers.Done()
			_, errorsByUser[index] = service.Approve(ctx, users[index].ID, tokens[index], "", "")
		}(index)
	}
	workers.Wait()
	approved := 0
	conflicted := 0
	for _, approvalErr := range errorsByUser {
		switch {
		case approvalErr == nil:
			approved++
		case errors.Is(approvalErr, entryidentity.ErrIdentityConflict):
			conflicted++
		default:
			require.NoError(t, approvalErr)
		}
	}
	assert.Equal(t, 1, approved)
	assert.Equal(t, 1, conflicted)
	var terminal int64
	require.NoError(t, stack.DB.WithContext(ctx).Model(&model.EntryIdentityLinkChallenge{}).
		Where("consumer = ? AND status IN ?", "concurrent-entry-service", []model.EntryIdentityLinkChallengeStatus{
			model.EntryIdentityLinkChallengeApproved, model.EntryIdentityLinkChallengeConflict,
		}).Count(&terminal).Error)
	assert.Equal(t, int64(2), terminal)
}

func TestConcurrentBotLinkInsertResolvesCommittedWinnerOnPostgres(t *testing.T) {
	stack := newIntegrationStack(t)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	owner := model.User{PrimaryLoginType: model.LoginTypeEmail, Status: model.UserStatusActive}
	require.NoError(t, stack.DB.WithContext(ctx).Create(&owner).Error)
	_, err := credentials.NewService(stack.DB).Create(credentials.CreateInput{
		ClientID: "barrier-entry-service", BotID: "barrier-entry-bot", EntryIssuer: "urn:paigram:entry:barrier",
		DisplayName: "Barrier Entry", OwnerUserID: owner.ID,
		Audiences: []string{"account-center"}, Scopes: []string{"bot.access.link_identity"},
	})
	require.NoError(t, err)
	users := []model.User{
		{PrimaryLoginType: model.LoginTypeEmail, Status: model.UserStatusActive},
		{PrimaryLoginType: model.LoginTypeEmail, Status: model.UserStatusActive},
	}
	for index := range users {
		require.NoError(t, stack.DB.WithContext(ctx).Create(&users[index]).Error)
	}

	arrived := make(chan struct{}, 2)
	release := make(chan struct{})
	services := make([]*botlink.Service, 2)
	for index := range services {
		database, openErr := gorm.Open(postgres.Open(stack.DatabaseCfg.DSN), &gorm.Config{})
		require.NoError(t, openErr)
		sqlDB, dbErr := database.DB()
		require.NoError(t, dbErr)
		t.Cleanup(func() { require.NoError(t, sqlDB.Close()) })
		callbackName := "test:bot-identity-insert-barrier"
		require.NoError(t, database.Callback().Create().Before("gorm:create").Register(callbackName, func(tx *gorm.DB) {
			if tx.Statement.Table != "bot_identities" {
				return
			}
			arrived <- struct{}{}
			select {
			case <-release:
			case <-tx.Statement.Context.Done():
			}
		}))
		services[index] = botlink.NewService(database, zap.NewNop())
	}

	results := make(chan error, 2)
	for index := range services {
		go func(index int) {
			_, linkErr := services[index].UpsertLink(ctx, botlink.UpsertLinkInput{
				BotID: "barrier-entry-bot", UserID: users[index].ID, ExternalUserID: "shared-barrier-subject",
			})
			results <- linkErr
		}(index)
	}
	for range 2 {
		select {
		case <-arrived:
		case <-ctx.Done():
			t.Fatal("both bot identity inserts did not reach the barrier")
		}
	}
	close(release)
	errorsByUser := []error{<-results, <-results}
	created := 0
	conflicted := 0
	for _, linkErr := range errorsByUser {
		switch {
		case linkErr == nil:
			created++
		case errors.Is(linkErr, botlink.ErrTelegramAlreadyLinkedToOther):
			conflicted++
		default:
			require.NoError(t, linkErr)
		}
	}
	assert.Equal(t, 1, created)
	assert.Equal(t, 1, conflicted)
	var active int64
	require.NoError(t, stack.DB.WithContext(ctx).Model(&model.BotIdentity{}).
		Where("issuer = ? AND external_user_id = ?", "urn:paigram:entry:barrier", "shared-barrier-subject").Count(&active).Error)
	assert.Equal(t, int64(1), active)
}

func TestConcurrentUnlinkRetriesReplayOneOperationOnPostgres(t *testing.T) {
	stack := newIntegrationStack(t)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	owner := model.User{PrimaryLoginType: model.LoginTypeEmail, Status: model.UserStatusActive}
	require.NoError(t, stack.DB.WithContext(ctx).Create(&owner).Error)
	_, err := credentials.NewService(stack.DB).Create(credentials.CreateInput{
		ClientID: "unlink-retry-service", BotID: "unlink-retry-bot", EntryIssuer: "urn:paigram:entry:unlink-retry",
		DisplayName: "Unlink Retry", OwnerUserID: owner.ID,
		Audiences: []string{"account-center"}, Scopes: []string{"bot.access.link_identity"},
	})
	require.NoError(t, err)
	_, err = botlink.NewService(stack.DB, zap.NewNop()).UpsertLink(ctx, botlink.UpsertLinkInput{
		BotID: "unlink-retry-bot", UserID: owner.ID, ExternalUserID: "unlink-retry-subject",
	})
	require.NoError(t, err)
	service := entryidentity.NewService(stack.DB, zap.NewNop(), entryidentity.Config{})
	const operationID = "00000000-0000-4000-8000-000000000041"
	results := make(chan *entryidentity.UnlinkResult, 2)
	errors := make(chan error, 2)
	for range 2 {
		go func() {
			result, unlinkErr := service.Unlink(ctx, owner.ID, "unlink-retry-bot", operationID, "", "")
			results <- result
			errors <- unlinkErr
		}()
	}
	for range 2 {
		require.NoError(t, <-errors)
		result := <-results
		require.NotNil(t, result)
		assert.Equal(t, operationID, result.OperationID)
		assert.Equal(t, "UNLINKED", result.State)
	}
	var operationCount int64
	require.NoError(t, stack.DB.WithContext(ctx).Model(&model.EntryIdentityUnlinkOperation{}).
		Where("operation_id = ?", operationID).Count(&operationCount).Error)
	assert.Equal(t, int64(1), operationCount)
	var auditCount int64
	require.NoError(t, stack.DB.WithContext(ctx).Model(&model.AuditLog{}).
		Where("user_id = ? AND action = ?", owner.ID, "telegram_link_revoked").Count(&auditCount).Error)
	assert.Equal(t, int64(1), auditCount)
}

func TestPendingUnlinkSerializesConcurrentRelinkOnPostgres(t *testing.T) {
	stack := newIntegrationStack(t)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	owner := model.User{PrimaryLoginType: model.LoginTypeEmail, Status: model.UserStatusActive}
	require.NoError(t, stack.DB.WithContext(ctx).Create(&owner).Error)
	_, err := credentials.NewService(stack.DB).Create(credentials.CreateInput{
		ClientID: "unlink-relink-service", BotID: "unlink-relink-bot", EntryIssuer: "urn:paigram:entry:unlink-relink",
		DisplayName: "Unlink Relink", OwnerUserID: owner.ID,
		Audiences: []string{"account-center"}, Scopes: []string{"bot.access.link_identity"},
	})
	require.NoError(t, err)
	_, err = botlink.NewService(stack.DB, zap.NewNop()).UpsertLink(ctx, botlink.UpsertLinkInput{
		BotID: "unlink-relink-bot", UserID: owner.ID, ExternalUserID: "old-subject",
	})
	require.NoError(t, err)
	binding := model.PlatformAccountBinding{
		BindingRef: "binding-unlink-relink", OwnerUserID: owner.ID, Platform: "mihomo",
		PlatformServiceKey: "platform-mihomo-service", DisplayName: "Unlink Relink",
		ExternalAccountKey: sql.NullString{String: "cn:unlink-relink", Valid: true},
		Status:             model.PlatformAccountBindingStatusActive, Generation: 1,
	}
	require.NoError(t, stack.DB.WithContext(ctx).Create(&binding).Error)
	grant := model.ConsumerGrant{
		BindingID: binding.ID, Consumer: "unlink-relink-service", Status: model.ConsumerGrantStatusActive,
		TicketVersion: 1, GrantedAt: time.Now().UTC(),
		Actions: []model.ConsumerGrantAction{{Action: "mihomo.status.read"}},
	}
	require.NoError(t, stack.DB.WithContext(ctx).Create(&grant).Error)

	openDatabase := func() (*gorm.DB, *sql.DB) {
		database, openErr := gorm.Open(postgres.Open(stack.DatabaseCfg.DSN), &gorm.Config{})
		require.NoError(t, openErr)
		sqlDB, dbErr := database.DB()
		require.NoError(t, dbErr)
		return database.WithContext(ctx), sqlDB
	}
	unlinkDB, unlinkSQL := openDatabase()
	linkDB, linkSQL := openDatabase()
	t.Cleanup(func() {
		require.NoError(t, unlinkSQL.Close())
		require.NoError(t, linkSQL.Close())
	})
	unlinkAdmissionReached := make(chan struct{}, 1)
	releaseUnlink := make(chan struct{})
	require.NoError(t, unlinkDB.Callback().Create().Before("gorm:create").Register("test:unlink-admission-barrier", func(tx *gorm.DB) {
		if tx.Statement.Table != "entry_identity_unlink_operations" {
			return
		}
		unlinkAdmissionReached <- struct{}{}
		select {
		case <-releaseUnlink:
		case <-tx.Statement.Context.Done():
		}
	}))

	type unlinkOutcome struct {
		result *entryidentity.UnlinkResult
		err    error
	}
	unlinked := make(chan unlinkOutcome, 1)
	go func() {
		result, unlinkErr := entryidentity.NewService(unlinkDB, zap.NewNop(), entryidentity.Config{
			GrantInvalidator: &entryFenceRecorder{err: assert.AnError},
		}).Unlink(ctx, owner.ID, "unlink-relink-bot", "00000000-0000-4000-8000-000000000051", "", "")
		unlinked <- unlinkOutcome{result: result, err: unlinkErr}
	}()
	select {
	case <-unlinkAdmissionReached:
	case <-ctx.Done():
		t.Fatal("unlink did not reach its admission barrier")
	}

	relinked := make(chan error, 1)
	go func() {
		_, linkErr := botlink.NewService(linkDB, zap.NewNop()).UpsertLink(ctx, botlink.UpsertLinkInput{
			BotID: "unlink-relink-bot", UserID: owner.ID, ExternalUserID: "new-subject",
		})
		relinked <- linkErr
	}()
	waitForPostgresLockWaiters(t, ctx, stack.SQLDB, "users", 1)
	close(releaseUnlink)

	outcome := <-unlinked
	require.NoError(t, outcome.err)
	require.NotNil(t, outcome.result)
	assert.True(t, outcome.result.PropagationPending)
	assert.ErrorIs(t, <-relinked, botlink.ErrUnlinkPending)
	var active int64
	require.NoError(t, stack.DB.WithContext(ctx).Model(&model.BotIdentity{}).
		Where("user_id = ? AND bot_id = ?", owner.ID, "unlink-relink-bot").Count(&active).Error)
	assert.Zero(t, active)
}

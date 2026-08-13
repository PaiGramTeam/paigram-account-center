package entryidentity

import (
	"bytes"
	"context"
	"errors"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"gorm.io/gorm"

	"paigram/internal/model"
	"paigram/internal/service/botlink"
	"paigram/internal/service/platformbinding"
)

func TestStartCreatesOpaqueFragmentLinkWithoutPersistingRawChallenge(t *testing.T) {
	db := newEntryIdentityTestDB(t)
	seedEntryIdentityPrincipal(t, db, "telegram-service", "paigrambot", "urn:paigram:entry:telegram")
	now := time.Date(2026, 8, 13, 12, 0, 0, 0, time.UTC)
	service := NewService(db, zap.NewNop(), Config{
		FrontendBaseURL: "https://account.example.com",
		ChallengeTTL:    5 * time.Minute,
		Now:             func() time.Time { return now },
		Random:          bytes.NewReader(bytes.Repeat([]byte{0x42}, challengeEntropyBytes)),
	})

	started, err := service.Start(context.Background(), StartInput{
		Consumer: "telegram-service", BotID: "paigrambot", ExternalSubject: "123456789",
	})
	require.NoError(t, err)
	assert.Equal(t, "urn:paigram:entry:telegram", started.Issuer)
	assert.Equal(t, "12*****89", started.MaskedSubject)
	assert.Equal(t, now.Add(5*time.Minute), started.ExpiresAt)
	approvalURL, err := url.Parse(started.ApprovalURL)
	require.NoError(t, err)
	assert.Equal(t, "/entry-identity-link", approvalURL.Path)
	assert.Empty(t, approvalURL.RawQuery)
	assert.True(t, strings.HasPrefix(approvalURL.Fragment, "challenge="))
	token := strings.TrimPrefix(approvalURL.Fragment, "challenge=")
	require.NotEmpty(t, token)

	var stored model.EntryIdentityLinkChallenge
	require.NoError(t, db.First(&stored).Error)
	assert.NotEqual(t, token, stored.ChallengeHash)
	assert.Len(t, stored.ChallengeHash, 64)
	var rawTokenCount int64
	require.NoError(t, db.Raw("SELECT COUNT(*) FROM entry_identity_link_challenges WHERE challenge_hash = ?", token).Scan(&rawTokenCount).Error)
	assert.Zero(t, rawTokenCount)
}

func TestApproveConsumesChallengeOnceAndDoesNotTransferIdentity(t *testing.T) {
	db := newEntryIdentityTestDB(t)
	seedEntryIdentityPrincipal(t, db, "telegram-service", "paigrambot", "urn:paigram:entry:telegram")
	seedEntryIdentityUser(t, db, 10)
	seedEntryIdentityUser(t, db, 20)
	service := NewService(db, zap.NewNop(), Config{FrontendBaseURL: "https://account.example.com"})

	first, err := service.Start(context.Background(), StartInput{Consumer: "telegram-service", BotID: "paigrambot", ExternalSubject: "subject-1"})
	require.NoError(t, err)
	token := challengeFromApprovalURL(t, first.ApprovalURL)
	identity, err := service.Approve(context.Background(), 10, token, "127.0.0.1", "test")
	require.NoError(t, err)
	assert.Equal(t, uint64(10), identity.UserID)
	assert.Equal(t, "urn:paigram:entry:telegram", identity.Issuer)

	_, err = service.Approve(context.Background(), 10, token, "127.0.0.1", "test")
	assert.ErrorIs(t, err, ErrChallengeConsumed)

	second, err := service.Start(context.Background(), StartInput{Consumer: "telegram-service", BotID: "paigrambot", ExternalSubject: "subject-1"})
	require.NoError(t, err)
	_, err = service.Approve(context.Background(), 20, challengeFromApprovalURL(t, second.ApprovalURL), "127.0.0.1", "test")
	assert.ErrorIs(t, err, ErrIdentityConflict)
	var stored model.BotIdentity
	require.NoError(t, db.Where("issuer = ? AND external_user_id = ?", "urn:paigram:entry:telegram", "subject-1").First(&stored).Error)
	assert.Equal(t, uint64(10), stored.UserID)
}

func TestExpiredAndCancelledChallengesCannotBeApproved(t *testing.T) {
	db := newEntryIdentityTestDB(t)
	seedEntryIdentityPrincipal(t, db, "telegram-service", "paigrambot", "urn:paigram:entry:telegram")
	seedEntryIdentityUser(t, db, 10)
	now := time.Date(2026, 8, 13, 12, 0, 0, 0, time.UTC)
	service := NewService(db, zap.NewNop(), Config{
		FrontendBaseURL: "https://account.example.com",
		Now:             func() time.Time { return now },
	})

	expired, err := service.Start(context.Background(), StartInput{Consumer: "telegram-service", BotID: "paigrambot", ExternalSubject: "expired"})
	require.NoError(t, err)
	now = now.Add(defaultChallengeTTL + time.Second)
	_, err = service.Approve(context.Background(), 10, challengeFromApprovalURL(t, expired.ApprovalURL), "", "")
	assert.ErrorIs(t, err, ErrChallengeExpired)

	cancelled, err := service.Start(context.Background(), StartInput{Consumer: "telegram-service", BotID: "paigrambot", ExternalSubject: "cancelled"})
	require.NoError(t, err)
	token := challengeFromApprovalURL(t, cancelled.ApprovalURL)
	require.NoError(t, service.Cancel(context.Background(), 10, token))
	_, err = service.Approve(context.Background(), 10, token, "", "")
	assert.ErrorIs(t, err, ErrChallengeConsumed)
}

func TestStartRejectsCredentialBotMismatch(t *testing.T) {
	db := newEntryIdentityTestDB(t)
	seedEntryIdentityPrincipal(t, db, "telegram-service", "paigrambot", "urn:paigram:entry:telegram")
	service := NewService(db, zap.NewNop(), Config{FrontendBaseURL: "https://account.example.com"})

	_, err := service.Start(context.Background(), StartInput{
		Consumer: "telegram-service", BotID: "different-bot", ExternalSubject: "subject",
	})
	assert.ErrorIs(t, err, ErrPrincipalMismatch)
}

func TestStartRejectsInactiveBotNamespace(t *testing.T) {
	db := newEntryIdentityTestDB(t)
	seedEntryIdentityPrincipal(t, db, "telegram-service", "paigrambot", "urn:paigram:entry:telegram")
	require.NoError(t, db.Exec(`UPDATE bots SET status = 'INACTIVE' WHERE id = 'paigrambot'`).Error)
	service := NewService(db, zap.NewNop(), Config{FrontendBaseURL: "https://account.example.com"})

	_, err := service.Start(context.Background(), StartInput{
		Consumer: "telegram-service", BotID: "paigrambot", ExternalSubject: "subject",
	})
	assert.ErrorIs(t, err, ErrNamespaceUnavailable)
}

func TestApproveRejectsChallengeAfterCredentialIsDisabled(t *testing.T) {
	db := newEntryIdentityTestDB(t)
	seedEntryIdentityPrincipal(t, db, "telegram-service", "paigrambot", "urn:paigram:entry:telegram")
	seedEntryIdentityUser(t, db, 10)
	service := NewService(db, zap.NewNop(), Config{FrontendBaseURL: "https://account.example.com"})
	started, err := service.Start(context.Background(), StartInput{
		Consumer: "telegram-service", BotID: "paigrambot", ExternalSubject: "subject",
	})
	require.NoError(t, err)
	require.NoError(t, db.Exec(`UPDATE service_credentials SET status = ? WHERE client_id = ?`,
		model.ServiceCredentialStatusDisabled, "telegram-service").Error)

	_, err = service.Approve(context.Background(), 10, challengeFromApprovalURL(t, started.ApprovalURL), "", "")
	assert.ErrorIs(t, err, ErrPrincipalMismatch)
	var identities int64
	require.NoError(t, db.Model(&model.BotIdentity{}).Count(&identities).Error)
	assert.Zero(t, identities)
}

func TestMaskSubjectDoesNotExposeShortIdentifiers(t *testing.T) {
	tests := map[string]string{
		"":      "",
		"a":     "*",
		"ab":    "a*",
		"abc":   "a*c",
		"abcd":  "a**d",
		"12345": "12*45",
	}
	for subject, expected := range tests {
		assert.Equal(t, expected, maskSubject(subject), subject)
	}
}

type recordingGrantInvalidator struct {
	inputs []platformbinding.GrantInvalidationInput
	err    error
}

func (invalidator *recordingGrantInvalidator) InvalidateConsumerGrant(_ context.Context, input platformbinding.GrantInvalidationInput) error {
	invalidator.inputs = append(invalidator.inputs, input)
	return invalidator.err
}

func TestUnlinkAdvancesEntryEpochAndPropagatesConsumerFence(t *testing.T) {
	db := newEntryIdentityTestDB(t)
	seedEntryIdentityPrincipal(t, db, "telegram-service", "paigrambot", "urn:paigram:entry:telegram")
	seedEntryIdentityPrincipal(t, db, "discord-service", "discordbot", "urn:paigram:entry:discord")
	seedEntryIdentityUser(t, db, 10)
	require.NoError(t, db.Exec(`INSERT INTO bot_identities (entry_identity_ref, entry_epoch, user_id, bot_id, issuer, external_user_id) VALUES
		('entry-telegram', 1, 10, 'paigrambot', 'urn:paigram:entry:telegram', 'external-1'),
		('entry-discord', 1, 10, 'discordbot', 'urn:paigram:entry:discord', 'external-2')`).Error)
	require.NoError(t, db.Exec(`INSERT INTO platform_account_bindings (id, binding_ref, generation, owner_user_id, platform, platform_service_key, status) VALUES
		(20, 'binding-20', 3, 10, 'mihomo', 'platform-mihomo-service', 'active')`).Error)
	require.NoError(t, db.Exec(`INSERT INTO consumer_grants (id, binding_id, consumer, status, ticket_version) VALUES
		(30, 20, 'telegram-service', 'active', 4)`).Error)
	require.NoError(t, db.Exec(`INSERT INTO consumer_grant_actions (grant_id, action) VALUES (30, 'mihomo.status.read')`).Error)
	invalidator := &recordingGrantInvalidator{}
	service := NewService(db, zap.NewNop(), Config{
		FrontendBaseURL: "https://account.example.com", GrantInvalidator: invalidator,
	})

	result, err := service.Unlink(context.Background(), 10, "paigrambot", "00000000-0000-4000-8000-000000000001", "127.0.0.1", "test")

	require.NoError(t, err)
	assert.Equal(t, uint64(2), result.MinimumEntryEpoch)
	assert.False(t, result.PropagationPending)
	require.Len(t, invalidator.inputs, 1)
	assert.Equal(t, uint64(2), invalidator.inputs[0].MinimumEntryEpoch)
	assert.Equal(t, uint64(4), invalidator.inputs[0].MinimumGrantVersion)
	var remaining model.BotIdentity
	require.NoError(t, db.Where("bot_id = ?", "discordbot").First(&remaining).Error)
	assert.Equal(t, uint64(2), remaining.EntryEpoch)
	var targetCount int64
	require.NoError(t, db.Model(&model.BotIdentity{}).Where("bot_id = ?", "paigrambot").Count(&targetCount).Error)
	assert.Zero(t, targetCount)
	var grant model.ConsumerGrant
	require.NoError(t, db.First(&grant, 30).Error)
	assert.Zero(t, grant.PendingEntryEpoch)
	assert.True(t, grant.LastInvalidatedAt.Valid)
}

func TestUnlinkKeepsDurablePendingFenceWhenPlatformIsUnavailable(t *testing.T) {
	db := newEntryIdentityTestDB(t)
	seedEntryIdentityPrincipal(t, db, "telegram-service", "paigrambot", "urn:paigram:entry:telegram")
	seedEntryIdentityUser(t, db, 10)
	require.NoError(t, db.Exec(`INSERT INTO bot_identities (entry_identity_ref, entry_epoch, user_id, bot_id, issuer, external_user_id)
		VALUES ('entry-telegram', 7, 10, 'paigrambot', 'urn:paigram:entry:telegram', 'external-1')`).Error)
	require.NoError(t, db.Exec(`INSERT INTO platform_account_bindings (id, binding_ref, generation, owner_user_id, platform, platform_service_key, status)
		VALUES (20, 'binding-20', 3, 10, 'mihomo', 'platform-mihomo-service', 'active')`).Error)
	require.NoError(t, db.Exec(`INSERT INTO consumer_grants (id, binding_id, consumer, status, ticket_version)
		VALUES (30, 20, 'telegram-service', 'active', 4)`).Error)
	require.NoError(t, db.Exec(`INSERT INTO consumer_grant_actions (grant_id, action) VALUES (30, 'mihomo.status.read')`).Error)
	invalidator := &recordingGrantInvalidator{err: errors.New("platform unavailable")}
	service := NewService(db, zap.NewNop(), Config{GrantInvalidator: invalidator})

	result, err := service.Unlink(context.Background(), 10, "paigrambot", "00000000-0000-4000-8000-000000000002", "", "")

	require.NoError(t, err)
	assert.True(t, result.PropagationPending)
	assert.Equal(t, "PROPAGATION_PENDING", result.State)
	require.NotEmpty(t, result.OperationID)
	status, err := service.UnlinkStatus(context.Background(), 10, "paigrambot", result.OperationID)
	require.NoError(t, err)
	assert.True(t, status.PropagationPending)
	var grant model.ConsumerGrant
	require.NoError(t, db.First(&grant, 30).Error)
	assert.Equal(t, uint64(8), grant.PendingEntryEpoch)
	assert.False(t, grant.LastInvalidatedAt.Valid)

	invalidator.err = nil
	completed, err := service.Unlink(context.Background(), 10, "paigrambot", "00000000-0000-4000-8000-000000000002", "", "")
	require.NoError(t, err)
	assert.False(t, completed.PropagationPending)
	assert.Equal(t, "UNLINKED", completed.State)
	assert.Equal(t, uint64(8), completed.MinimumEntryEpoch)
	status, err = service.UnlinkStatus(context.Background(), 10, "paigrambot", result.OperationID)
	require.NoError(t, err)
	assert.False(t, status.PropagationPending)
	assert.Equal(t, "UNLINKED", status.State)
	relinked, err := service.linker.UpsertLink(context.Background(), botlink.UpsertLinkInput{
		BotID: "paigrambot", UserID: 10, ExternalUserID: "external-1",
	})
	require.NoError(t, err)
	assert.NotEqual(t, result.OperationID, relinked.EntryIdentityRef)
	status, err = service.UnlinkStatus(context.Background(), 10, "paigrambot", result.OperationID)
	require.NoError(t, err)
	assert.Equal(t, "UNLINKED", status.State)
	replayed, err := service.Unlink(context.Background(), 10, "paigrambot", result.OperationID, "", "")
	require.NoError(t, err)
	assert.Equal(t, "UNLINKED", replayed.State)
	var activeRelink model.BotIdentity
	require.NoError(t, db.Where("entry_identity_ref = ?", relinked.EntryIdentityRef).Take(&activeRelink).Error)

	invalidator.err = errors.New("second fence unavailable")
	second, err := service.Unlink(context.Background(), 10, "paigrambot", "00000000-0000-4000-8000-000000000003", "", "")
	require.NoError(t, err)
	assert.True(t, second.PropagationPending)
	requestsBeforeOldReplay := len(invalidator.inputs)
	firstStatus, err := service.UnlinkStatus(context.Background(), 10, "paigrambot", result.OperationID)
	require.NoError(t, err)
	assert.Equal(t, "UNLINKED", firstStatus.State)
	firstReplay, err := service.Unlink(context.Background(), 10, "paigrambot", result.OperationID, "", "")
	require.NoError(t, err)
	assert.Equal(t, "UNLINKED", firstReplay.State)
	assert.Len(t, invalidator.inputs, requestsBeforeOldReplay)
	secondStatus, err := service.UnlinkStatus(context.Background(), 10, "paigrambot", second.OperationID)
	require.NoError(t, err)
	assert.True(t, secondStatus.PropagationPending)

	require.NoError(t, db.First(&grant, 30).Error)
	assert.Equal(t, second.MinimumEntryEpoch, grant.PendingEntryEpoch)
	assert.False(t, grant.LastInvalidatedAt.Valid)
}

func challengeFromApprovalURL(t *testing.T, value string) string {
	t.Helper()
	parsed, err := url.Parse(value)
	require.NoError(t, err)
	fragment, err := url.ParseQuery(parsed.Fragment)
	require.NoError(t, err)
	return fragment.Get("challenge")
}

func newEntryIdentityTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.Exec(`CREATE TABLE users (id INTEGER PRIMARY KEY, user_ref TEXT, owner_epoch INTEGER NOT NULL DEFAULT 1, deleted_at DATETIME)`).Error)
	require.NoError(t, db.Exec(`CREATE TABLE bots (id TEXT PRIMARY KEY, entry_issuer TEXT NOT NULL, display_name TEXT NOT NULL, status TEXT NOT NULL, owner_user_id INTEGER NOT NULL, deleted_at DATETIME)`).Error)
	require.NoError(t, db.Exec(`CREATE TABLE service_credentials (client_id TEXT PRIMARY KEY, bot_id TEXT NOT NULL, status TEXT NOT NULL, deleted_at DATETIME)`).Error)
	require.NoError(t, db.Exec(`CREATE TABLE entry_identity_link_challenges (
		challenge_hash TEXT PRIMARY KEY, consumer TEXT NOT NULL, bot_id TEXT NOT NULL, issuer TEXT NOT NULL,
		external_subject TEXT NOT NULL, external_username TEXT, status TEXT NOT NULL, expires_at DATETIME NOT NULL,
		approved_user_id INTEGER, consumed_at DATETIME, created_at DATETIME NOT NULL, updated_at DATETIME NOT NULL
	)`).Error)
	require.NoError(t, db.Exec(`CREATE TABLE entry_identity_unlink_operations (
		operation_id TEXT PRIMARY KEY, user_id INTEGER NOT NULL, bot_id TEXT NOT NULL, entry_identity_ref TEXT NOT NULL,
		minimum_entry_epoch INTEGER NOT NULL, state TEXT NOT NULL, completed_at DATETIME,
		created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP, updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
	)`).Error)
	require.NoError(t, db.Exec(`CREATE TABLE bot_identities (
		id INTEGER PRIMARY KEY AUTOINCREMENT, entry_identity_ref TEXT NOT NULL, entry_epoch INTEGER NOT NULL DEFAULT 1,
		user_id INTEGER NOT NULL, bot_id TEXT NOT NULL, issuer TEXT NOT NULL, external_user_id TEXT NOT NULL,
		external_username TEXT, linked_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
		created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP, updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP, deleted_at DATETIME
	)`).Error)
	require.NoError(t, db.Exec(`CREATE UNIQUE INDEX uk_test_entry_ref ON bot_identities(entry_identity_ref)`).Error)
	require.NoError(t, db.Exec(`CREATE UNIQUE INDEX uk_test_entry_subject ON bot_identities(issuer, external_user_id) WHERE deleted_at IS NULL`).Error)
	require.NoError(t, db.Exec(`CREATE UNIQUE INDEX uk_test_user_issuer ON bot_identities(user_id, issuer) WHERE deleted_at IS NULL`).Error)
	require.NoError(t, db.Exec(`CREATE TABLE audit_logs (
		id INTEGER PRIMARY KEY AUTOINCREMENT, user_id INTEGER NOT NULL, action TEXT NOT NULL, resource TEXT,
		resource_id INTEGER, old_value TEXT, new_value TEXT, ip TEXT, user_agent TEXT, details TEXT, created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
	)`).Error)
	require.NoError(t, db.Exec(`CREATE TABLE platform_account_bindings (
		id INTEGER PRIMARY KEY, binding_ref TEXT NOT NULL, generation INTEGER NOT NULL, owner_user_id INTEGER NOT NULL,
		platform TEXT NOT NULL, platform_service_key TEXT NOT NULL, status TEXT NOT NULL, deleted_at DATETIME
	)`).Error)
	require.NoError(t, db.Exec(`CREATE TABLE consumer_grants (
		id INTEGER PRIMARY KEY, binding_id INTEGER NOT NULL, consumer TEXT NOT NULL, status TEXT NOT NULL,
		ticket_version INTEGER NOT NULL DEFAULT 1, pending_entry_epoch INTEGER NOT NULL DEFAULT 0,
		granted_by INTEGER, granted_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP, revoked_at DATETIME,
		last_invalidated_at DATETIME, created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
	)`).Error)
	require.NoError(t, db.Exec(`CREATE TABLE entry_identity_unlink_targets (
		operation_id TEXT NOT NULL, grant_id INTEGER NOT NULL, confirmed_at DATETIME, PRIMARY KEY (operation_id, grant_id)
	)`).Error)
	require.NoError(t, db.Exec(`CREATE TABLE consumer_grant_actions (
		grant_id INTEGER NOT NULL, action TEXT NOT NULL, created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
		PRIMARY KEY (grant_id, action)
	)`).Error)
	return db
}

func seedEntryIdentityPrincipal(t *testing.T, db *gorm.DB, consumer, botID, issuer string) {
	t.Helper()
	require.NoError(t, db.Exec(`INSERT INTO bots (id, entry_issuer, display_name, status, owner_user_id) VALUES (?, ?, ?, 'ACTIVE', 1)`, botID, issuer, "Test Bot").Error)
	require.NoError(t, db.Exec(`INSERT INTO service_credentials (client_id, bot_id, status) VALUES (?, ?, 'active')`, consumer, botID).Error)
}

func seedEntryIdentityUser(t *testing.T, db *gorm.DB, id uint64) {
	t.Helper()
	require.NoError(t, db.Exec(`INSERT INTO users (id, user_ref) VALUES (?, ?)`, id, "user-test").Error)
}

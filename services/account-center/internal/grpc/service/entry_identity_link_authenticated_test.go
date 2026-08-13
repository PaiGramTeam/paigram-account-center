package service_test

import (
	"context"
	"net/url"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/metadata"
	"gorm.io/gorm"

	pb "github.com/PaiGramTeam/paigram-account-center/contracts/gen/go/account/v1"
	"paigram/internal/model"
	"paigram/internal/service/credentials"
)

func TestStartEntryIdentityLinkAuthenticatesCredentialAndStoresOnlyChallengeHash(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.Exec(`CREATE TABLE users (
		id INTEGER PRIMARY KEY AUTOINCREMENT, user_ref TEXT NOT NULL, owner_epoch INTEGER NOT NULL DEFAULT 1,
		primary_login_type TEXT NOT NULL, status TEXT NOT NULL, primary_role_id INTEGER, last_login_at DATETIME,
		created_at DATETIME, updated_at DATETIME, deleted_at DATETIME
	)`).Error)
	require.NoError(t, db.Exec(`CREATE TABLE bots (
		id TEXT PRIMARY KEY, entry_issuer TEXT NOT NULL, display_name TEXT NOT NULL, description TEXT, type TEXT NOT NULL,
		status TEXT NOT NULL, owner_user_id INTEGER NOT NULL, created_at DATETIME, updated_at DATETIME, deleted_at DATETIME
	)`).Error)
	require.NoError(t, db.Exec(`CREATE TABLE service_credentials (
		client_id TEXT PRIMARY KEY, consumer_epoch INTEGER NOT NULL DEFAULT 1, bot_id TEXT NOT NULL, display_name TEXT NOT NULL,
		secret_hash TEXT NOT NULL, audiences TEXT NOT NULL, scopes TEXT NOT NULL, status TEXT NOT NULL,
		owner_user_id INTEGER NOT NULL, description TEXT, last_used_at DATETIME, created_at DATETIME, updated_at DATETIME, deleted_at DATETIME
	)`).Error)
	require.NoError(t, db.Exec(`CREATE TABLE entry_identity_link_challenges (
		challenge_hash TEXT PRIMARY KEY, consumer TEXT NOT NULL, bot_id TEXT NOT NULL, issuer TEXT NOT NULL,
		external_subject TEXT NOT NULL, external_username TEXT, status TEXT NOT NULL, expires_at DATETIME NOT NULL,
		approved_user_id INTEGER, consumed_at DATETIME, created_at DATETIME NOT NULL, updated_at DATETIME NOT NULL
	)`).Error)
	owner := model.User{PrimaryLoginType: model.LoginTypeEmail, Status: model.UserStatusActive}
	require.NoError(t, db.Create(&owner).Error)
	require.NoError(t, db.Create(&model.Bot{
		ID: "telegram-service", EntryIssuer: "urn:paigram:entry:telegram", DisplayName: "PaiGram",
		Type: "SERVICE", Status: "ACTIVE", OwnerUserID: owner.ID,
	}).Error)
	signingKey := botAccessTestSigningKey(t)
	accessToken := seedServiceCredentialAndIssueToken(t, db, signingKey, "telegram-service", []string{"bot.access.link_identity"})
	conn := newBotAccessBufconnClient(t, db, signingKey)
	defer conn.Close()
	ctx := metadata.NewOutgoingContext(context.Background(), metadata.Pairs("authorization", "Bearer "+accessToken))

	response, err := pb.NewBotAccessServiceClient(conn).StartEntryIdentityLink(ctx, &pb.StartEntryIdentityLinkRequest{
		ExternalSubject: "external-user-42", ExternalUsername: "traveler",
	})

	require.NoError(t, err)
	assert.Equal(t, "urn:paigram:entry:telegram", response.GetIssuer())
	parsed, err := url.Parse(response.GetApprovalUrl())
	require.NoError(t, err)
	fragment, err := url.ParseQuery(parsed.Fragment)
	require.NoError(t, err)
	rawChallenge := fragment.Get("challenge")
	require.NotEmpty(t, rawChallenge)
	var stored model.EntryIdentityLinkChallenge
	require.NoError(t, db.First(&stored).Error)
	assert.NotEqual(t, rawChallenge, stored.ChallengeHash)
	assert.Len(t, stored.ChallengeHash, 64)

	credential, err := credentials.NewService(db).GetByClientID("telegram-service")
	require.NoError(t, err)
	assert.Equal(t, "telegram-service", credential.BotID)
}

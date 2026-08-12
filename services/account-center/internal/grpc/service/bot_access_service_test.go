package service_test

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"database/sql"
	"encoding/json"
	"net"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
	"gorm.io/datatypes"
	"gorm.io/gorm"

	pb "github.com/PaiGramTeam/paigram-account-center/contracts/gen/go/account/v1"
	contractticket "github.com/PaiGramTeam/paigram-account-center/contracts/runtime/go/serviceticket"
	"paigram/internal/grpc/interceptor"
	grpcservice "paigram/internal/grpc/service"
	"paigram/internal/model"
	"paigram/internal/service/botaccess"
	"paigram/internal/service/credentials"
	internalticket "paigram/internal/serviceticket"
	"paigram/internal/testutil"
)

// botAccessTestSigningKey returns a random 32-byte HS256 test key, matching
// the minimum accepted by the credential token service.
func botAccessTestSigningKey(t *testing.T) []byte {
	t.Helper()
	key := make([]byte, 32)
	_, err := rand.Read(key)
	require.NoError(t, err)
	return key
}

// seedServiceCredentialAndIssueToken inserts an active model.ServiceCredential
// row with the given client_id + scopes, then mints an HS256 access token via
// credentials.TokenService bound to the same signingKey. The returned token
// is what the bufconn-mounted interceptor will validate against the seeded
// row at request time.
func seedServiceCredentialAndIssueToken(t *testing.T, db *gorm.DB, signingKey []byte, clientID string, scopes []string) string {
	t.Helper()

	// Bcrypt-hash a fixed plaintext; the token issuance path verifies
	// the secret before minting, so the hash must correspond to the
	// plaintext we'll pass into IssueClientCredentials.
	plaintextSecret := clientID + "-test-secret"
	hash, err := credentials.HashClientSecret(plaintextSecret)
	require.NoError(t, err)

	audiencesJSON, err := json.Marshal([]string{"account-center"})
	require.NoError(t, err)
	scopesJSON, err := json.Marshal(scopes)
	require.NoError(t, err)

	row := &model.ServiceCredential{
		ClientID:    clientID,
		BotID:       clientID,
		DisplayName: clientID,
		SecretHash:  hash,
		Audiences:   datatypes.JSON(audiencesJSON),
		Scopes:      datatypes.JSON(scopesJSON),
		Status:      model.ServiceCredentialStatusActive,
		OwnerUserID: 1,
	}
	require.NoError(t, db.Save(row).Error)

	tokenSvc, err := credentials.NewTokenService(credentials.NewService(db), credentials.TokenServiceConfig{
		Issuer:                "account-center",
		AccessTokenTTLSeconds: 3600,
		SigningKey:            signingKey,
	})
	require.NoError(t, err)
	issued, err := tokenSvc.IssueClientCredentials(credentials.IssueClientCredentialsInput{
		ClientID:        clientID,
		ClientSecret:    plaintextSecret,
		Audience:        "account-center",
		RequestedScopes: scopes,
	})
	require.NoError(t, err)
	return issued.AccessToken
}

func TestBotAccessServiceAuthenticatedFlow(t *testing.T) {
	db := testutil.OpenPostgreSQLTestDB(t, "bot_access_grpc",
		&model.User{},
		&model.UserEmail{},
		&model.Bot{},
		&model.ServiceCredential{},
		&model.BotIdentity{},
		&model.PlatformService{},
		&model.PlatformAccountBinding{},
		&model.PlatformAccountProfile{},
		&model.ConsumerGrant{},
		&model.AuditEvent{},
	)

	signingKey := botAccessTestSigningKey(t)
	bot, identityUser, ref := seedBotAccessGRPCTestData(t, db)
	var ticketPublicKey ed25519.PublicKey
	conn := newBotAccessBufconnClient(t, db, signingKey, &ticketPublicKey)
	defer conn.Close()

	accessToken := seedServiceCredentialAndIssueToken(t, db, signingKey, bot.ID, []string{"bot.access.read", "bot.access.issue_ticket"})

	ctx := metadata.NewOutgoingContext(context.Background(), metadata.Pairs("authorization", "Bearer "+accessToken))
	accessClient := pb.NewBotAccessServiceClient(conn)

	resolved, err := accessClient.ResolveBotUser(ctx, &pb.ResolveBotUserRequest{ExternalUserId: "tg-123"})
	require.NoError(t, err)
	assert.Equal(t, identityUser.ID, resolved.UserId)
	assert.Equal(t, bot.ID, resolved.BotId)
	assert.Equal(t, "tg-123", resolved.ExternalUserId)
	assert.Equal(t, "alice", resolved.ExternalUsername)

	accounts, err := accessClient.ListAccessibleBindings(ctx, &pb.ListAccessibleBindingsRequest{
		ExternalUserId: "tg-123",
		Platform:       "hoyoverse",
	})
	require.NoError(t, err)
	require.Len(t, accounts.Bindings, 1)
	assert.Equal(t, ref.ID, accounts.Bindings[0].Id)
	assert.Equal(t, "platform-hoyoverse-service", accounts.Bindings[0].PlatformServiceKey)

	ticketResp, err := accessClient.IssueServiceTicket(ctx, &pb.IssueServiceTicketRequest{
		ExternalUserId:  "tg-123",
		BindingId:       ref.ID,
		RequestedScopes: []string{"daily.sign"},
	})
	require.NoError(t, err)
	assert.NotEmpty(t, ticketResp.Ticket)
	assert.Equal(t, "hoyoverse.runtime", ticketResp.Audience)
	assert.Equal(t, ref.ID, ticketResp.Binding.Id)

	parsedClaims := &botaccess.ServiceTicketClaims{}
	parsedToken, err := jwt.ParseWithClaims(ticketResp.Ticket, parsedClaims, func(token *jwt.Token) (any, error) {
		assert.Equal(t, contractticket.AlgorithmEd25519, token.Method.Alg())
		assert.Equal(t, internalticket.TypeDelegation, token.Header["typ"])
		assert.Equal(t, "test-key", token.Header["kid"])
		return ticketPublicKey, nil
	}, jwt.WithValidMethods([]string{contractticket.AlgorithmEd25519}))
	require.NoError(t, err)
	require.True(t, parsedToken.Valid)
	assert.Equal(t, "consumer", parsedClaims.ActorType)
	assert.Equal(t, bot.ID, parsedClaims.ActorID)
	assert.Equal(t, bot.ID, parsedClaims.Consumer)
	assert.Equal(t, bot.ID, parsedClaims.BotID)
	assert.Equal(t, identityUser.ID, parsedClaims.UserID)
	assert.Equal(t, ref.ID, parsedClaims.BindingID)
	assert.Equal(t, []string{"daily.sign"}, parsedClaims.AllowedActions)
	assert.Equal(t, "consumer:"+bot.ID, parsedClaims.Subject)
	assert.ElementsMatch(t, []string{"hoyoverse.runtime"}, []string(parsedClaims.Audience))
	assert.WithinDuration(t, ticketResp.ExpiresAt.AsTime(), parsedClaims.ExpiresAt.Time, time.Second)

	var event model.AuditEvent
	require.NoError(t, db.Where("category = ? AND action = ?", "bot_access", "ticket_issue").Order("id DESC").First(&event).Error)
	assert.Equal(t, "consumer", event.ActorType)
	assert.Equal(t, "success", event.Result)
	assert.Equal(t, "binding", event.TargetType)
}

func TestBotAccessServiceRejectsRequestedScopesOutsideGrantedSet(t *testing.T) {
	db := testutil.OpenPostgreSQLTestDB(t, "bot_access_grpc_scope_reject",
		&model.User{},
		&model.UserEmail{},
		&model.Bot{},
		&model.ServiceCredential{},
		&model.BotIdentity{},
		&model.PlatformService{},
		&model.PlatformAccountBinding{},
		&model.PlatformAccountProfile{},
		&model.ConsumerGrant{},
		&model.AuditEvent{},
	)

	signingKey := botAccessTestSigningKey(t)
	bot, _, ref := seedBotAccessGRPCTestData(t, db)
	grantJSON, err := json.Marshal([]string{"daily.sign"})
	require.NoError(t, err)
	require.NoError(t, db.Model(&model.ConsumerGrant{}).Where("binding_id = ? AND consumer = ?", ref.ID, bot.ID).Update("scopes_json", string(grantJSON)).Error)

	conn := newBotAccessBufconnClient(t, db, signingKey)
	defer conn.Close()
	accessToken := seedServiceCredentialAndIssueToken(t, db, signingKey, bot.ID, []string{"bot.access.read", "bot.access.issue_ticket"})
	ctx := metadata.NewOutgoingContext(context.Background(), metadata.Pairs("authorization", "Bearer "+accessToken))
	accessClient := pb.NewBotAccessServiceClient(conn)

	_, err = accessClient.IssueServiceTicket(ctx, &pb.IssueServiceTicketRequest{
		ExternalUserId:  "tg-123",
		BindingId:       ref.ID,
		RequestedScopes: []string{"daily.sign", "notes.write"},
	})
	require.Error(t, err)
	assert.Equal(t, codes.PermissionDenied, status.Code(err))

	var event model.AuditEvent
	require.NoError(t, db.Where("category = ? AND action = ?", "bot_access", "ticket_reject").Order("id DESC").First(&event).Error)
	assert.Equal(t, "failure", event.Result)
	assert.NotEmpty(t, event.ReasonCode)
}

func TestBotAccessServiceRejectsRevokedConsumerGrantOnTicketIssue(t *testing.T) {
	db := testutil.OpenPostgreSQLTestDB(t, "bot_access_grpc_revoked",
		&model.User{},
		&model.UserEmail{},
		&model.Bot{},
		&model.ServiceCredential{},
		&model.BotIdentity{},
		&model.PlatformService{},
		&model.PlatformAccountBinding{},
		&model.ConsumerGrant{},
	)

	signingKey := botAccessTestSigningKey(t)
	bot, _, ref := seedBotAccessGRPCTestData(t, db)
	var grant model.ConsumerGrant
	require.NoError(t, db.Where("binding_id = ? AND consumer = ?", ref.ID, bot.ID).First(&grant).Error)
	grant.Status = model.ConsumerGrantStatusRevoked
	grant.RevokedAt = sql.NullTime{Time: time.Now().UTC(), Valid: true}
	require.NoError(t, db.Save(&grant).Error)

	conn := newBotAccessBufconnClient(t, db, signingKey)
	defer conn.Close()
	accessToken := seedServiceCredentialAndIssueToken(t, db, signingKey, bot.ID, []string{"bot.access.read", "bot.access.issue_ticket"})
	ctx := metadata.NewOutgoingContext(context.Background(), metadata.Pairs("authorization", "Bearer "+accessToken))
	accessClient := pb.NewBotAccessServiceClient(conn)

	_, err := accessClient.IssueServiceTicket(ctx, &pb.IssueServiceTicketRequest{
		ExternalUserId:  "tg-123",
		BindingId:       ref.ID,
		RequestedScopes: []string{"daily.sign"},
	})
	require.Error(t, err)
	assert.Equal(t, codes.PermissionDenied, status.Code(err))
}

func TestBotAccessServiceRejectsMissingAuthorization(t *testing.T) {
	db := testutil.OpenPostgreSQLTestDB(t, "bot_access_grpc_noauth",
		&model.User{},
		&model.UserEmail{},
		&model.Bot{},
		&model.ServiceCredential{},
		&model.BotIdentity{},
		&model.PlatformService{},
		&model.PlatformAccountBinding{},
		&model.ConsumerGrant{},
	)

	signingKey := botAccessTestSigningKey(t)
	_, _, _ = seedBotAccessGRPCTestData(t, db)
	conn := newBotAccessBufconnClient(t, db, signingKey)
	defer conn.Close()

	accessClient := pb.NewBotAccessServiceClient(conn)
	_, err := accessClient.ResolveBotUser(context.Background(), &pb.ResolveBotUserRequest{ExternalUserId: "tg-123"})
	require.Error(t, err)
	assert.Equal(t, codes.Unauthenticated, status.Code(err))
}

// TestBotAccessServiceRejectsTokenMissingIssueTicketScope confirms that a
// credential without bot.access.issue_ticket is rejected before binding lookup.
func TestBotAccessServiceRejectsTokenMissingIssueTicketScope(t *testing.T) {
	db := testutil.OpenPostgreSQLTestDB(t, "bot_access_grpc_missing_machine_scope",
		&model.User{},
		&model.UserEmail{},
		&model.Bot{},
		&model.ServiceCredential{},
		&model.BotIdentity{},
		&model.PlatformService{},
		&model.PlatformAccountBinding{},
		&model.ConsumerGrant{},
	)

	signingKey := botAccessTestSigningKey(t)
	bot, _, ref := seedBotAccessGRPCTestData(t, db)
	conn := newBotAccessBufconnClient(t, db, signingKey)
	defer conn.Close()
	accessToken := seedServiceCredentialAndIssueToken(t, db, signingKey, bot.ID, []string{"bot.access.read"})
	ctx := metadata.NewOutgoingContext(context.Background(), metadata.Pairs("authorization", "Bearer "+accessToken))
	accessClient := pb.NewBotAccessServiceClient(conn)

	_, err := accessClient.IssueServiceTicket(ctx, &pb.IssueServiceTicketRequest{
		ExternalUserId:  "tg-123",
		BindingId:       ref.ID,
		RequestedScopes: []string{"daily.sign"},
	})

	require.Error(t, err)
	assert.Equal(t, codes.PermissionDenied, status.Code(err))
}

func TestBotAccessServiceRejectsCallerWithoutGrant(t *testing.T) {
	db := testutil.OpenPostgreSQLTestDB(t, "bot_access_grpc_no_grant",
		&model.User{},
		&model.UserEmail{},
		&model.Bot{},
		&model.ServiceCredential{},
		&model.BotIdentity{},
		&model.PlatformService{},
		&model.PlatformAccountBinding{},
		&model.ConsumerGrant{},
	)

	signingKey := botAccessTestSigningKey(t)
	_, identityUser, ref := seedBotAccessGRPCTestData(t, db)
	conn := newBotAccessBufconnClient(t, db, signingKey)
	defer conn.Close()
	// The authenticated client ID identifies both the bot identity and grant
	// consumer in this test. Seed the second identity but omit its grant so
	// GetGrantedBindingForConsumer hits
	// ErrConsumerGrantNotFound instead of ErrBotIdentityNotFound (which would
	// map to NotFound).
	require.NoError(t, db.Create(&model.Bot{
		ID:          "pamgram",
		DisplayName: "Pamgram",
		Description: "second test bot without a grant",
		Type:        "TELEGRAM",
		Status:      "ACTIVE",
		OwnerUserID: identityUser.ID,
	}).Error)
	require.NoError(t, db.Create(&model.BotIdentity{
		UserID:           identityUser.ID,
		BotID:            "pamgram",
		ExternalUserID:   "tg-123",
		ExternalUsername: sql.NullString{String: "alice", Valid: true},
		LinkedAt:         time.Now().UTC(),
	}).Error)
	accessToken := seedServiceCredentialAndIssueToken(t, db, signingKey, "pamgram", []string{"bot.access.issue_ticket"})
	ctx := metadata.NewOutgoingContext(context.Background(), metadata.Pairs("authorization", "Bearer "+accessToken))
	accessClient := pb.NewBotAccessServiceClient(conn)

	_, err := accessClient.IssueServiceTicket(ctx, &pb.IssueServiceTicketRequest{
		ExternalUserId: "tg-123",
		BindingId:      ref.ID,
	})

	require.Error(t, err)
	assert.Equal(t, codes.PermissionDenied, status.Code(err))
}

func seedBotAccessGRPCTestData(t *testing.T, db *gorm.DB) (model.Bot, model.User, model.PlatformAccountBinding) {
	t.Helper()
	require.NoError(t, db.Where("platform_key = ?", "hoyoverse").FirstOrCreate(&model.PlatformService{
		PlatformKey:          "hoyoverse",
		DisplayName:          "Hoyoverse",
		ServiceKey:           "platform-hoyoverse-service",
		ServiceAudience:      "hoyoverse.runtime",
		DiscoveryType:        "static",
		Endpoint:             "127.0.0.1:1",
		Enabled:              true,
		SupportedActionsJSON: `[]`,
		CredentialSchemaJSON: `{"type":"object"}`,
	}).Error)

	owner := model.User{PrimaryLoginType: model.LoginTypeEmail, Status: model.UserStatusActive}
	require.NoError(t, db.Create(&owner).Error)
	require.NoError(t, db.Create(&model.UserEmail{UserID: owner.ID, Email: "owner@example.com", IsPrimary: true}).Error)

	// Bot rows contain identity metadata, not credentials or scopes.
	bot := model.Bot{
		ID:          "paigram-bot",
		DisplayName: "PaiGramBot",
		Description: "test bot",
		Type:        "TELEGRAM",
		Status:      "ACTIVE",
		OwnerUserID: owner.ID,
	}
	require.NoError(t, db.Create(&bot).Error)

	identityUser := model.User{PrimaryLoginType: model.LoginTypeEmail, Status: model.UserStatusActive}
	require.NoError(t, db.Create(&identityUser).Error)
	require.NoError(t, db.Create(&model.UserEmail{UserID: identityUser.ID, Email: "alice@example.com", IsPrimary: true}).Error)
	require.NoError(t, db.Create(&model.BotIdentity{
		UserID:           identityUser.ID,
		BotID:            bot.ID,
		ExternalUserID:   "tg-123",
		ExternalUsername: sql.NullString{String: "alice", Valid: true},
		LinkedAt:         time.Now().UTC(),
	}).Error)

	ref := model.PlatformAccountBinding{
		OwnerUserID:        identityUser.ID,
		Platform:           "hoyoverse",
		ExternalAccountKey: sql.NullString{String: "hoyo-account-001", Valid: true},
		PlatformServiceKey: "platform-hoyoverse-service",
		DisplayName:        "Alice Hoyo",
		Status:             model.PlatformAccountBindingStatusActive,
	}
	require.NoError(t, db.Create(&ref).Error)
	// Seed the grant under the calling credential's client ID.
	require.NoError(t, db.Create(&model.ConsumerGrant{
		BindingID:  ref.ID,
		Consumer:   bot.ID,
		Status:     model.ConsumerGrantStatusActive,
		ScopesJSON: `["daily.sign","daily.note.read"]`,
		GrantedAt:  time.Now().UTC(),
	}).Error)

	return bot, identityUser, ref
}

func seedBotAccessPlatformService(t *testing.T, db *gorm.DB) {
	t.Helper()
	require.NoError(t, db.Create(&model.PlatformService{
		PlatformKey:          "mihomo",
		DisplayName:          "Mihomo",
		ServiceKey:           "platform-mihomo-service",
		ServiceAudience:      "platform-mihomo-service",
		DiscoveryType:        "static",
		Endpoint:             "127.0.0.1:1",
		Enabled:              true,
		SupportedActionsJSON: `[]`,
		CredentialSchemaJSON: `{"type":"object"}`,
	}).Error)
}

func requireBotAccessMetadata(t *testing.T, raw string) map[string]any {
	t.Helper()
	var metadata map[string]any
	require.NoError(t, json.Unmarshal([]byte(raw), &metadata))
	return metadata
}

func newBotAccessBufconnClient(t *testing.T, db *gorm.DB, signingKey []byte, ticketPublicKeyOutput ...*ed25519.PublicKey) *grpc.ClientConn {
	t.Helper()

	tokenSvc, err := credentials.NewTokenService(credentials.NewService(db), credentials.TokenServiceConfig{
		Issuer:                "account-center",
		AccessTokenTTLSeconds: 3600,
		SigningKey:            signingKey,
	})
	require.NoError(t, err)

	listener := bufconn.Listen(1024 * 1024)
	grpcServer := grpc.NewServer(grpc.ChainUnaryInterceptor(interceptor.NewAuthInterceptor(tokenSvc).Unary()))

	authConfig, ticketPublicKey := testutil.NewAuthConfig(t)
	if len(ticketPublicKeyOutput) > 0 && ticketPublicKeyOutput[0] != nil {
		*ticketPublicKeyOutput[0] = ticketPublicKey
	}
	group, err := botaccess.NewServiceGroup(db, authConfig)
	require.NoError(t, err)
	pb.RegisterBotAccessServiceServer(grpcServer, grpcservice.NewBotAccessService(&group.BindingAccessService, &group.TicketService, db))

	serveErrCh := make(chan error, 1)
	go func() {
		serveErrCh <- grpcServer.Serve(listener)
	}()
	t.Cleanup(func() {
		grpcServer.Stop()
		_ = listener.Close()
		<-serveErrCh
	})

	conn, err := grpc.DialContext(context.Background(), "passthrough:///bufnet",
		grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) {
			return listener.Dial()
		}),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	require.NoError(t, err)

	return conn
}

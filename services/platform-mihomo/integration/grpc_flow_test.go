//go:build integration

package integration

import (
	"context"
	"crypto/ed25519"
	"database/sql"
	"net"
	"strconv"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"

	v1 "platform-mihomo-service/api/mihomo/v1"
	"platform-mihomo-service/internal/data"
	platformmihomo "platform-mihomo-service/internal/platform/mihomo"
	"platform-mihomo-service/internal/service"
	"platform-mihomo-service/internal/usecase"
)

const (
	integrationTicketIssuer   = "paigram-account-center"
	integrationTicketAudience = "platform-mihomo-service"
	integrationTicketKeyID    = "integration-service-ticket-key"
	bufConnAddress            = "bufnet"
	testBindingID             = uint64(101)
)

var (
	integrationEncryptionKey    = []byte("0123456789abcdef0123456789abcdef")
	integrationTicketPrivateKey = ed25519.NewKeyFromSeed([]byte("abcdef0123456789abcdef0123456789"))
)

func TestBindThenGetAuthKeyFlow(t *testing.T) {
	stack := newIntegrationStack(t)
	t.Cleanup(stack.cleanup)
	client := newMihomoClientForTest(t, stack)

	bindResp, err := client.BindCredential(testTicket(t), validBindRequest())
	require.NoError(t, err)
	require.NotEmpty(t, bindResp.PlatformAccountId)
	require.NotEmpty(t, bindResp.Profiles)
	playerID := bindResp.Profiles[0].PlayerId

	authResp, err := client.GetAuthKey(testTicketForAccount(t, bindResp.PlatformAccountId, "mihomo.authkey.issue"), bindResp.PlatformAccountId, playerID)
	require.NoError(t, err)
	require.NotEmpty(t, authResp.Authkey)

	artifact := requireRuntimeArtifact(t, stack.SQLDB, bindResp.PlatformAccountId, playerID)
	require.Equal(t, authResp.Authkey, artifact.ArtifactValue)
	requireRuntimeArtifactCount(t, stack.SQLDB, bindResp.PlatformAccountId, 1)
	if stack.Redis != nil {
		requireRedisArtifactCached(t, stack, testBindingID, bindResp.PlatformAccountId, playerID, authResp.Authkey)
	}

	secondAuthResp, err := client.GetAuthKey(testTicketForAccount(t, bindResp.PlatformAccountId, "mihomo.authkey.issue"), bindResp.PlatformAccountId, playerID)
	require.NoError(t, err)
	require.Equal(t, authResp.Authkey, secondAuthResp.Authkey)
	requireRuntimeArtifactCount(t, stack.SQLDB, bindResp.PlatformAccountId, 1)
}

func TestUpdateThenDeleteCredentialFlow(t *testing.T) {
	stack := newIntegrationStack(t)
	t.Cleanup(stack.cleanup)
	client := newMihomoClientForTest(t, stack)

	bindResp, err := client.BindCredential(testTicket(t), validBindRequest())
	require.NoError(t, err)
	playerID := bindResp.Profiles[0].PlayerId

	authResp, err := client.GetAuthKey(testTicketForAccount(t, bindResp.PlatformAccountId, "mihomo.authkey.issue"), bindResp.PlatformAccountId, playerID)
	require.NoError(t, err)
	require.NotEmpty(t, authResp.Authkey)
	requireRuntimeArtifact(t, stack.SQLDB, bindResp.PlatformAccountId, playerID)
	if stack.Redis != nil {
		requireRedisArtifactCached(t, stack, testBindingID, bindResp.PlatformAccountId, playerID, authResp.Authkey)
	}

	summaryResp, err := client.GetCredentialSummary(testTicketForAccount(t, bindResp.PlatformAccountId, "mihomo.credential.read_meta"), bindResp.PlatformAccountId)
	require.NoError(t, err)
	require.NotNil(t, summaryResp.Summary)
	require.Equal(t, bindResp.PlatformAccountId, summaryResp.Summary.PlatformAccountId)
	require.Len(t, summaryResp.Summary.Devices, 1)
	require.Len(t, summaryResp.Summary.Profiles, 1)

	updateResp, err := client.UpdateCredential(testTicketForAccount(t, bindResp.PlatformAccountId, "mihomo.credential.update"), &v1.UpdateCredentialRequest{
		PlatformAccountId: bindResp.PlatformAccountId,
		CookieBundleJson:  `{"account_id":"10001","cookie_token":"updated"}`,
		Device: &v1.DeviceInfo{
			DeviceId:   "87654321-4321-4321-4321-cba987654321",
			DeviceFp:   "zyxwvutsrqponm",
			DeviceName: "updated-device",
		},
		RegionHint: "cn_gf01",
	})
	require.NoError(t, err)
	require.NotNil(t, updateResp.Summary)
	require.Len(t, updateResp.Summary.Devices, 2)
	require.Equal(t, "87654321-4321-4321-4321-cba987654321", updateResp.Summary.Devices[1].DeviceId)
	requireRuntimeArtifactCount(t, stack.SQLDB, bindResp.PlatformAccountId, 0)

	deleteResp, err := client.DeleteCredential(testTicketForAccount(t, bindResp.PlatformAccountId, "mihomo.credential.delete"), bindResp.PlatformAccountId)
	require.NoError(t, err)
	require.True(t, deleteResp.Success)

	_, err = client.GetCredentialSummary(testTicketForAccount(t, bindResp.PlatformAccountId, "mihomo.credential.read_meta"), bindResp.PlatformAccountId)
	require.Error(t, err)
	require.Equal(t, codes.NotFound, status.Code(err))
	requireRuntimeArtifactCount(t, stack.SQLDB, bindResp.PlatformAccountId, 0)
	if stack.Redis != nil {
		requireRedisArtifactMissing(t, stack, testBindingID, bindResp.PlatformAccountId, playerID)
	}
}

type testMihomoClient struct {
	client v1.MihomoAccountServiceClient
	t      *testing.T
}

func newMihomoClientForTest(t *testing.T, stack *integrationStack) *testMihomoClient {
	t.Helper()

	requireMigrationsApplied(t, stack.SQLDB)

	db, err := gorm.Open(mysql.Open(stack.DatabaseCfg.Source), &gorm.Config{})
	require.NoError(t, err)

	credentialRepo := data.NewCredentialRepo(db)
	deviceRepo := data.NewDeviceRepo(db)
	profileRepo := data.NewProfileRepo(db)
	artifactRepo := data.NewArtifactRepo(db, stack.Redis, stack.RedisPrefix)
	managementRepo := data.NewManagementRepo(db, stack.Redis, stack.RedisPrefix)
	hoyoClient := platformmihomo.StubClient{}
	bindUC := usecase.NewBindUsecase(credentialRepo, deviceRepo, profileRepo, hoyoClient, integrationEncryptionKey, artifactRepo)
	profileUC := usecase.NewProfileUsecase(profileRepo)

	svc := service.NewMihomoAccountService(
		data.NewStaticKeyTicketVerifier(integrationTicketIssuer, integrationTicketKeyID, integrationTicketPrivateKey.Public().(ed25519.PublicKey)),
		bindUC,
		usecase.NewStatusUsecase(credentialRepo, hoyoClient, integrationEncryptionKey),
		profileUC,
		usecase.NewAuthkeyUsecase(credentialRepo, artifactRepo, hoyoClient, integrationEncryptionKey),
		usecase.NewManagementUsecase(credentialRepo, deviceRepo, profileRepo, artifactRepo, managementRepo, bindUC, profileUC),
	)

	listener := bufconn.Listen(1024 * 1024)
	server := grpc.NewServer()
	v1.RegisterMihomoAccountServiceServer(server, svc)

	go func() {
		_ = server.Serve(listener)
	}()

	t.Cleanup(func() {
		server.Stop()
		_ = listener.Close()
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	t.Cleanup(cancel)

	conn, err := grpc.DialContext(ctx, bufConnAddress,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) {
			return listener.Dial()
		}),
	)
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })

	return &testMihomoClient{client: v1.NewMihomoAccountServiceClient(conn), t: t}
}

func (c *testMihomoClient) BindCredential(serviceTicket string, request *v1.BindCredentialRequest) (*v1.BindCredentialResponse, error) {
	c.t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	request.ServiceTicket = serviceTicket
	return c.client.BindCredential(ctx, request)
}

func (c *testMihomoClient) GetAuthKey(serviceTicket string, platformAccountID string, playerID string) (*v1.GetAuthKeyResponse, error) {
	c.t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	return c.client.GetAuthKey(ctx, &v1.GetAuthKeyRequest{
		ServiceTicket:     serviceTicket,
		PlatformAccountId: platformAccountID,
		PlayerId:          playerID,
	})
}

func (c *testMihomoClient) GetCredentialSummary(serviceTicket string, platformAccountID string) (*v1.GetCredentialSummaryResponse, error) {
	c.t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	return c.client.GetCredentialSummary(ctx, &v1.GetCredentialSummaryRequest{
		ServiceTicket:     serviceTicket,
		PlatformAccountId: platformAccountID,
	})
}

func (c *testMihomoClient) UpdateCredential(serviceTicket string, request *v1.UpdateCredentialRequest) (*v1.UpdateCredentialResponse, error) {
	c.t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	request.ServiceTicket = serviceTicket
	return c.client.UpdateCredential(ctx, request)
}

func (c *testMihomoClient) DeleteCredential(serviceTicket string, platformAccountID string) (*v1.DeleteCredentialResponse, error) {
	c.t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	return c.client.DeleteCredential(ctx, &v1.DeleteCredentialRequest{
		ServiceTicket:     serviceTicket,
		PlatformAccountId: platformAccountID,
	})
}

type runtimeArtifactRecord struct {
	BindingID     uint64
	ArtifactValue string
	ScopeKey      string
}

func requireRuntimeArtifact(t *testing.T, db *sql.DB, platformAccountID string, playerID string) runtimeArtifactRecord {
	t.Helper()

	const query = `
		SELECT binding_id, artifact_value, scope_key
		FROM runtime_artifacts
		WHERE platform_account_id = ? AND artifact_type = ? AND scope_key = ?
		LIMIT 1
	`

	var record runtimeArtifactRecord
	err := db.QueryRow(query, platformAccountID, "authkey", playerID).Scan(&record.BindingID, &record.ArtifactValue, &record.ScopeKey)
	require.NoError(t, err)
	return record
}

func requireRuntimeArtifactCount(t *testing.T, db *sql.DB, platformAccountID string, want int) {
	t.Helper()

	var count int
	err := db.QueryRow(`SELECT COUNT(*) FROM runtime_artifacts WHERE platform_account_id = ?`, platformAccountID).Scan(&count)
	require.NoError(t, err)
	require.Equal(t, want, count)
}

func requireRedisArtifactCached(t *testing.T, stack *integrationStack, bindingID uint64, platformAccountID string, playerID string, expectedAuthKey string) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	payload, err := stack.Redis.Get(ctx, redisArtifactBindingKey(stack, bindingID, playerID)).Result()
	require.NoError(t, err)
	require.Contains(t, payload, expectedAuthKey)
	count, err := stack.Redis.Exists(ctx, redisArtifactKey(stack, platformAccountID, playerID)).Result()
	require.NoError(t, err)
	require.Zero(t, count)
}

func requireRedisArtifactMissing(t *testing.T, stack *integrationStack, bindingID uint64, platformAccountID string, playerID string) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	count, err := stack.Redis.Exists(ctx,
		redisArtifactBindingKey(stack, bindingID, playerID),
		redisArtifactKey(stack, platformAccountID, playerID),
	).Result()
	require.NoError(t, err)
	require.Zero(t, count)
}

func redisArtifactKey(stack *integrationStack, platformAccountID string, playerID string) string {
	return stack.RedisPrefix + "artifact:" + platformAccountID + ":authkey:" + playerID
}

func redisArtifactBindingKey(stack *integrationStack, bindingID uint64, playerID string) string {
	return stack.RedisPrefix + "artifact:binding:" + strconv.FormatUint(bindingID, 10) + ":authkey:" + playerID
}

func testTicket(t *testing.T) string { return testTicketForAccount(t, "", "mihomo.credential.bind") }

func testTicketForAccount(t *testing.T, platformAccountID string, scopes ...string) string {
	t.Helper()

	claims := jwt.MapClaims{
		"iss":                     integrationTicketIssuer,
		"aud":                     []string{integrationTicketAudience},
		"actor_type":              "bot",
		"actor_id":                "bot-paigram",
		"owner_user_id":           float64(1),
		"binding_id":              float64(testBindingID),
		"bot_id":                  "bot-paigram",
		"platform":                "mihomo",
		"user_id":                 float64(1),
		"platform_service_key":    integrationTicketAudience,
		"platform_account_ref_id": float64(testBindingID),
		"exp":                     time.Now().Add(time.Minute).Unix(),
	}
	if platformAccountID != "" {
		claims["platform_account_id"] = platformAccountID
	}
	if len(scopes) > 0 {
		claims["scopes"] = scopes
	}
	token := jwt.NewWithClaims(jwt.SigningMethodEdDSA, claims)
	token.Header["kid"] = integrationTicketKeyID
	token.Header["typ"] = "service_ticket"

	signed, err := token.SignedString(integrationTicketPrivateKey)
	require.NoError(t, err)

	return signed
}

func validBindRequest() *v1.BindCredentialRequest {
	return &v1.BindCredentialRequest{
		CookieBundleJson: `{"account_id":"10001","cookie_token":"abc"}`,
		Device: &v1.DeviceInfo{
			DeviceId:   "12345678-1234-1234-1234-123456789abc",
			DeviceFp:   "abcdefghijklmn",
			DeviceName: "integration-device",
		},
	}
}

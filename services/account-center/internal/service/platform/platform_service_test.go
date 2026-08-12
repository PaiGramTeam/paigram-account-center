package platform

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"net"
	"strings"
	"testing"

	platformv1 "github.com/PaiGramTeam/paigram-account-center/contracts/gen/go/platform/v1"
	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/test/bufconn"

	"paigram/internal/config"
	"paigram/internal/model"
	"paigram/internal/service/platformbinding"
	"paigram/internal/testutil"
)

func testPlatformAuthConfig(t *testing.T) config.AuthConfig {
	config, _ := testPlatformAuth(t)
	return config
}

func testPlatformAuth(t *testing.T) (config.AuthConfig, ed25519.PublicKey) {
	t.Helper()
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	privateDER, err := x509.MarshalPKCS8PrivateKey(privateKey)
	require.NoError(t, err)

	return config.AuthConfig{
		ServiceTicketTTLSeconds:    300,
		ServiceTicketIssuer:        "account-center",
		ServiceTicketKeyID:         "test-key",
		ServiceTicketPrivateKeyPEM: string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: privateDER})),
	}, publicKey
}

func TestPlatformServiceGetEnabledPlatform(t *testing.T) {
	db := testutil.OpenPostgreSQLTestDB(t, "platform_registry", &model.PlatformService{})
	require.NoError(t, db.Create(&model.PlatformService{
		PlatformKey:          "mihomo",
		DisplayName:          "Mihomo",
		ServiceKey:           "platform-mihomo-service",
		ServiceAudience:      "platform-mihomo-service",
		DiscoveryType:        "static",
		Endpoint:             "127.0.0.1:9000",
		Enabled:              true,
		SupportedActionsJSON: `["bind_credential","delete_credential"]`,
		CredentialSchemaJSON: `{}`,
	}).Error)

	svc := NewServiceGroup(db)
	platform, err := svc.PlatformService.GetEnabledPlatform("mihomo")
	require.NoError(t, err)
	require.Equal(t, "platform-mihomo-service", platform.ServiceKey)
}

func TestPlatformServiceListEnabledPlatforms(t *testing.T) {
	db := testutil.OpenPostgreSQLTestDB(t, "platform_registry_list", &model.PlatformService{})
	require.NoError(t, db.Create(&model.PlatformService{
		PlatformKey:          "zenless",
		DisplayName:          "Zenless Zone Zero",
		ServiceKey:           "platform-zenless-service",
		ServiceAudience:      "platform-zenless-service",
		DiscoveryType:        "static",
		Endpoint:             "127.0.0.1:9001",
		Enabled:              true,
		SupportedActionsJSON: `["bind_credential"]`,
		CredentialSchemaJSON: `{}`,
	}).Error)
	require.NoError(t, db.Create(&model.PlatformService{
		PlatformKey:          "mihomo",
		DisplayName:          "Mihomo",
		ServiceKey:           "platform-mihomo-service",
		ServiceAudience:      "platform-mihomo-service",
		DiscoveryType:        "static",
		Endpoint:             "127.0.0.1:9000",
		Enabled:              true,
		SupportedActionsJSON: `["bind_credential","delete_credential"]`,
		CredentialSchemaJSON: `{}`,
	}).Error)
	disabled := &model.PlatformService{
		PlatformKey:          "disabled",
		DisplayName:          "Disabled",
		ServiceKey:           "platform-disabled-service",
		ServiceAudience:      "platform-disabled-service",
		DiscoveryType:        "static",
		Endpoint:             "127.0.0.1:9002",
		Enabled:              false,
		SupportedActionsJSON: `[]`,
		CredentialSchemaJSON: `{}`,
	}
	require.NoError(t, db.Create(disabled).Error)
	require.NoError(t, db.Model(disabled).Update("enabled", false).Error)

	svc := NewServiceGroup(db)
	platforms, err := svc.PlatformService.ListEnabledPlatforms()
	require.NoError(t, err)
	require.Len(t, platforms, 2)
	require.Equal(t, []string{"mihomo", "zenless"}, []string{platforms[0].PlatformKey, platforms[1].PlatformKey})
}

func TestPlatformServiceBuildsBindingActorTicketClaims(t *testing.T) {
	claims := buildBindingScopedTicketClaims("user", "session-123", 7, 11, "mihomo", "platform-mihomo-service", "hoyo_ref_11_10001", []string{"mihomo.credential.read_meta"})
	require.Equal(t, "user", claims.ActorType)
	require.Equal(t, "session-123", claims.ActorID)
	require.Equal(t, uint64(7), claims.OwnerUserID)
	require.Equal(t, uint64(11), claims.BindingID)
	require.Equal(t, "mihomo", claims.Platform)
	require.Equal(t, "platform-mihomo-service", claims.PlatformServiceKey)
	require.Equal(t, "hoyo_ref_11_10001", claims.PlatformAccountID)
	require.Equal(t, []string{"mihomo.credential.read_meta"}, claims.Scopes)
}

func TestPlatformServiceConfigureAuthRejectsMissingPrivateKey(t *testing.T) {
	svc := PlatformService{}
	require.ErrorIs(t, svc.ConfigureAuth(config.AuthConfig{
		ServiceTicketTTLSeconds: 300,
		ServiceTicketIssuer:     "account-center",
		ServiceTicketKeyID:      "test-key",
	}), ErrInvalidTicketConfig)
	require.Nil(t, svc.ticketSigner)
}

func TestPlatformServiceInvalidateConsumerGrantCallsPlatformService(t *testing.T) {
	db := testutil.OpenPostgreSQLTestDB(t, "platform_registry_grant_invalidation", &model.PlatformService{})
	require.NoError(t, db.Create(&model.PlatformService{
		PlatformKey:          "mihomo",
		DisplayName:          "Mihomo",
		ServiceKey:           "platform-mihomo-service",
		ServiceAudience:      "platform-mihomo-service",
		DiscoveryType:        "static",
		Endpoint:             "bufnet",
		Enabled:              true,
		SupportedActionsJSON: `[]`,
		CredentialSchemaJSON: `{}`,
	}).Error)

	stub := &grantInvalidationPlatformServiceStub{response: &platformv1.InvalidateConsumerGrantResponse{Success: true}}
	listener := bufconn.Listen(1024 * 1024)
	server := grpc.NewServer()
	platformv1.RegisterPlatformServiceServer(server, stub)
	go server.Serve(listener)
	t.Cleanup(func() {
		server.Stop()
		_ = listener.Close()
	})

	svc := NewServiceGroup(db).PlatformService
	authConfig, publicKey := testPlatformAuth(t)
	require.NoError(t, svc.ConfigureAuth(authConfig))
	svc.dial = func(ctx context.Context, endpoint string) (*grpc.ClientConn, error) {
		require.Equal(t, "bufnet", endpoint)
		return grpc.DialContext(ctx, "passthrough:///bufnet",
			grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) {
				return listener.Dial()
			}),
			grpc.WithTransportCredentials(insecure.NewCredentials()),
		)
	}

	err := svc.InvalidateConsumerGrant(context.Background(), platformbinding.GrantInvalidationInput{
		BindingID:           42,
		OwnerUserID:         7,
		Platform:            "mihomo",
		PlatformServiceKey:  "platform-mihomo-service",
		Consumer:            platformbinding.ConsumerPaiGramBot,
		MinimumGrantVersion: 8,
		ActorType:           "admin",
		ActorID:             "admin:7",
	})
	require.NoError(t, err)
	require.NotNil(t, stub.lastRequest)
	require.Equal(t, uint64(42), stub.lastRequest.GetBindingId())
	require.Equal(t, platformbinding.ConsumerPaiGramBot, stub.lastRequest.GetConsumer())
	require.Equal(t, uint64(8), stub.lastRequest.GetMinimumGrantVersion())

	parsed := &ServiceTicketClaims{}
	token, err := jwt.ParseWithClaims(stub.lastServiceTicket, parsed, func(token *jwt.Token) (any, error) {
		return publicKey, nil
	})
	require.NoError(t, err)
	require.True(t, token.Valid)
	require.Equal(t, "admin", parsed.ActorType)
	require.Equal(t, "admin:7", parsed.ActorID)
	require.Equal(t, uint64(7), parsed.OwnerUserID)
	require.Equal(t, uint64(42), parsed.BindingID)
	require.Equal(t, "mihomo", parsed.Platform)
	require.Equal(t, "platform-mihomo-service", parsed.PlatformServiceKey)
	require.Equal(t, []string{"mihomo.consumer_grant.invalidate"}, parsed.Scopes)
	require.Equal(t, []string{"platform-mihomo-service"}, []string(parsed.Audience))
}

func TestPlatformServiceInvalidateConsumerGrantReturnsErrorWhenPlatformRejects(t *testing.T) {
	db := testutil.OpenPostgreSQLTestDB(t, "platform_registry_grant_invalidation_rejected", &model.PlatformService{})
	require.NoError(t, db.Create(&model.PlatformService{
		PlatformKey:          "mihomo",
		DisplayName:          "Mihomo",
		ServiceKey:           "platform-mihomo-service",
		ServiceAudience:      "platform-mihomo-service",
		DiscoveryType:        "static",
		Endpoint:             "bufnet",
		Enabled:              true,
		SupportedActionsJSON: `[]`,
		CredentialSchemaJSON: `{}`,
	}).Error)

	stub := &grantInvalidationPlatformServiceStub{response: &platformv1.InvalidateConsumerGrantResponse{Success: false}}
	listener := bufconn.Listen(1024 * 1024)
	server := grpc.NewServer()
	platformv1.RegisterPlatformServiceServer(server, stub)
	go server.Serve(listener)
	t.Cleanup(func() {
		server.Stop()
		_ = listener.Close()
	})

	svc := NewServiceGroup(db).PlatformService
	authConfig, _ := testPlatformAuth(t)
	require.NoError(t, svc.ConfigureAuth(authConfig))
	svc.dial = func(ctx context.Context, endpoint string) (*grpc.ClientConn, error) {
		require.Equal(t, "bufnet", endpoint)
		return grpc.DialContext(ctx, "passthrough:///bufnet",
			grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) {
				return listener.Dial()
			}),
			grpc.WithTransportCredentials(insecure.NewCredentials()),
		)
	}

	err := svc.InvalidateConsumerGrant(context.Background(), platformbinding.GrantInvalidationInput{
		BindingID:           42,
		OwnerUserID:         7,
		Platform:            "mihomo",
		PlatformServiceKey:  "platform-mihomo-service",
		Consumer:            platformbinding.ConsumerPaiGramBot,
		MinimumGrantVersion: 8,
		ActorType:           "admin",
		ActorID:             "admin:7",
	})
	require.ErrorIs(t, err, ErrConsumerGrantInvalidationRejected)
}

func TestPlatformServiceInvalidateConsumerGrantNormalizesConsumerActorToUser(t *testing.T) {
	db := testutil.OpenPostgreSQLTestDB(t, "platform_registry_grant_invalidation_actor", &model.PlatformService{})
	require.NoError(t, db.Create(&model.PlatformService{
		PlatformKey:          "mihomo",
		DisplayName:          "Mihomo",
		ServiceKey:           "platform-mihomo-service",
		ServiceAudience:      "platform-mihomo-service",
		DiscoveryType:        "static",
		Endpoint:             "bufnet",
		Enabled:              true,
		SupportedActionsJSON: `[]`,
		CredentialSchemaJSON: `{}`,
	}).Error)

	stub := &grantInvalidationPlatformServiceStub{response: &platformv1.InvalidateConsumerGrantResponse{Success: true}}
	listener := bufconn.Listen(1024 * 1024)
	server := grpc.NewServer()
	platformv1.RegisterPlatformServiceServer(server, stub)
	go server.Serve(listener)
	t.Cleanup(func() {
		server.Stop()
		_ = listener.Close()
	})

	svc := NewServiceGroup(db).PlatformService
	authConfig, publicKey := testPlatformAuth(t)
	require.NoError(t, svc.ConfigureAuth(authConfig))
	svc.dial = func(ctx context.Context, endpoint string) (*grpc.ClientConn, error) {
		require.Equal(t, "bufnet", endpoint)
		return grpc.DialContext(ctx, "passthrough:///bufnet",
			grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) {
				return listener.Dial()
			}),
			grpc.WithTransportCredentials(insecure.NewCredentials()),
		)
	}

	err := svc.InvalidateConsumerGrant(context.Background(), platformbinding.GrantInvalidationInput{
		BindingID:           42,
		OwnerUserID:         7,
		Platform:            "mihomo",
		PlatformServiceKey:  "platform-mihomo-service",
		Consumer:            platformbinding.ConsumerPaiGramBot,
		MinimumGrantVersion: 8,
		ActorType:           "consumer",
		ActorID:             platformbinding.ConsumerPaiGramBot,
	})
	require.NoError(t, err)

	parsed := &ServiceTicketClaims{}
	token, err := jwt.ParseWithClaims(stub.lastServiceTicket, parsed, func(token *jwt.Token) (any, error) {
		return publicKey, nil
	})
	require.NoError(t, err)
	require.True(t, token.Valid)
	assert.Equal(t, "user", parsed.ActorType)
	assert.Equal(t, "system:grant-revoke", parsed.ActorID)
}

func TestPlatformServiceListPlatformViews(t *testing.T) {
	db := testutil.OpenPostgreSQLTestDB(t, "platform_registry_views", &model.PlatformService{})
	require.NoError(t, db.Create(&model.PlatformService{
		PlatformKey:          "mihomo",
		DisplayName:          "Mihomo",
		ServiceKey:           "platform-mihomo-service",
		ServiceAudience:      "platform-mihomo-service",
		DiscoveryType:        "static",
		Endpoint:             "127.0.0.1:9000",
		Enabled:              true,
		SupportedActionsJSON: `["bind_credential","delete_credential"]`,
		CredentialSchemaJSON: `{"type":"object"}`,
	}).Error)

	svc := NewServiceGroup(db)
	views, err := svc.PlatformService.ListEnabledPlatformViews()
	require.NoError(t, err)
	require.Len(t, views, 1)
	require.Equal(t, PlatformListView{
		Platform:         "mihomo",
		DisplayName:      "Mihomo",
		SupportedActions: []string{"bind_credential", "delete_credential"},
	}, views[0])
}

type grantInvalidationPlatformServiceStub struct {
	platformv1.UnimplementedPlatformServiceServer
	response          *platformv1.InvalidateConsumerGrantResponse
	lastRequest       *platformv1.InvalidateConsumerGrantRequest
	lastServiceTicket string
}

func (s *grantInvalidationPlatformServiceStub) InvalidateConsumerGrant(ctx context.Context, req *platformv1.InvalidateConsumerGrantRequest) (*platformv1.InvalidateConsumerGrantResponse, error) {
	md, _ := metadata.FromIncomingContext(ctx)
	values := md.Get("authorization")
	if len(values) == 1 {
		s.lastServiceTicket = strings.TrimPrefix(values[0], "Bearer ")
	}
	s.lastRequest = req
	return s.response, nil
}

func TestPlatformServiceGetPlatformSchemaView(t *testing.T) {
	db := testutil.OpenPostgreSQLTestDB(t, "platform_registry_schema_view", &model.PlatformService{})
	require.NoError(t, db.Create(&model.PlatformService{
		PlatformKey:          "mihomo",
		DisplayName:          "Mihomo",
		ServiceKey:           "platform-mihomo-service",
		ServiceAudience:      "platform-mihomo-service",
		DiscoveryType:        "static",
		Endpoint:             "127.0.0.1:9000",
		Enabled:              true,
		SupportedActionsJSON: `["bind_credential"]`,
		CredentialSchemaJSON: `{"type":"object","required":["cookie_bundle"]}`,
	}).Error)

	svc := NewServiceGroup(db)
	view, err := svc.PlatformService.GetPlatformSchemaView("mihomo")
	require.NoError(t, err)
	require.Equal(t, &PlatformSchemaView{
		Platform:         "mihomo",
		DisplayName:      "Mihomo",
		SupportedActions: []string{"bind_credential"},
		CredentialSchema: map[string]any{"type": "object", "required": []any{"cookie_bundle"}},
	}, view)
}

package botaccess

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"database/sql"
	"encoding/pem"
	"strings"
	"testing"
	"time"

	contractticket "github.com/PaiGramTeam/paigram-account-center/contracts/runtime/go/serviceticket"
	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"paigram/internal/config"
	"paigram/internal/model"
	internalticket "paigram/internal/serviceticket"
)

func TestNewTicketServiceRejectsInvalidPrivateKey(t *testing.T) {
	service, err := NewTicketService(config.AuthConfig{
		ServiceTicketTTLSeconds:    300,
		ServiceTicketIssuer:        "issuer",
		ServiceTicketKeyID:         "test-key",
		ServiceTicketPrivateKeyPEM: "invalid",
	})
	require.ErrorIs(t, err, ErrInvalidTicketConfig)
	assert.Nil(t, service)
}

func TestNewTicketServiceRejectsZeroTTL(t *testing.T) {
	authConfig, _ := newTestTicketAuthConfig(t, 0)
	service, err := NewTicketService(authConfig)
	require.ErrorIs(t, err, ErrInvalidTicketConfig)
	assert.Nil(t, service)
}

func TestTicketServiceIssueIncludesAudienceAndActions(t *testing.T) {
	authConfig, publicKey := newTestTicketAuthConfig(t, 300)
	service, err := NewTicketService(authConfig)
	require.NoError(t, err)

	binding := &model.PlatformAccountBinding{
		ID:                 42,
		OwnerUserID:        100,
		Platform:           "telegram",
		ExternalAccountKey: sql.NullString{String: "acct-42", Valid: true},
		PlatformServiceKey: "tg-main",
		DisplayName:        "Primary",
		Status:             model.PlatformAccountBindingStatusActive,
	}

	tokenString, expiresAt, err := service.Issue("bot-ticket", "paigram-bot", binding, []string{"profile:read", "messages:send"}, "platform-service", 0, 1)
	require.NoError(t, err)
	assert.WithinDuration(t, time.Now().UTC().Add(5*time.Minute), expiresAt, 3*time.Second)

	parsed := &ServiceTicketClaims{}
	token, err := jwt.ParseWithClaims(tokenString, parsed, func(token *jwt.Token) (any, error) {
		assert.Equal(t, contractticket.AlgorithmEd25519, token.Method.Alg())
		assert.Equal(t, "test-key", token.Header["kid"])
		assert.Equal(t, internalticket.TypeDelegation, token.Header["typ"])
		return publicKey, nil
	}, jwt.WithValidMethods([]string{contractticket.AlgorithmEd25519}), jwt.WithAudience("platform-service"), jwt.WithIssuer("issuer"), jwt.WithExpirationRequired(), jwt.WithIssuedAt())
	require.NoError(t, err)
	require.True(t, token.Valid)
	assert.Equal(t, "consumer", parsed.ActorType)
	assert.Equal(t, "paigram-bot", parsed.ActorID)
	assert.Equal(t, "paigram-bot", parsed.Consumer)
	assert.Equal(t, "paigram-bot", parsed.ClientID)
	assert.Equal(t, binding.OwnerUserID, parsed.OwnerUserID)
	assert.Equal(t, "bot-ticket", parsed.BotID)
	assert.Equal(t, binding.OwnerUserID, parsed.UserID)
	assert.Equal(t, binding.Platform, parsed.Platform)
	assert.Equal(t, binding.PlatformServiceKey, parsed.PlatformServiceKey)
	assert.Equal(t, binding.ID, parsed.BindingID)
	assert.Equal(t, "acct-42", parsed.PlatformAccountID)
	assert.ElementsMatch(t, []string{"profile:read", "messages:send"}, parsed.AllowedActions)
	assert.Equal(t, "consumer:paigram-bot", parsed.Subject)
	assert.WithinDuration(t, expiresAt, parsed.ExpiresAt.Time, time.Second)
	assert.NotNil(t, parsed.NotBefore)
	assert.NotEmpty(t, parsed.ID)
}

func TestTicketServiceIssueIncludesProfileAndGrantVersion(t *testing.T) {
	authConfig, publicKey := newTestTicketAuthConfig(t, 60)
	service, err := NewTicketService(authConfig)
	require.NoError(t, err)

	binding := activeTestBinding()
	tokenString, _, err := service.Issue("bot-paigram", "paigram-bot", binding, []string{"mihomo.profile.read"}, "platform-mihomo-service", 99, 3)
	require.NoError(t, err)

	parsed := &ServiceTicketClaims{}
	token, err := jwt.ParseWithClaims(tokenString, parsed, func(_ *jwt.Token) (any, error) {
		return publicKey, nil
	}, jwt.WithValidMethods([]string{contractticket.AlgorithmEd25519}), jwt.WithAudience("platform-mihomo-service"), jwt.WithIssuer("issuer"))
	require.NoError(t, err)
	require.True(t, token.Valid)
	assert.Equal(t, uint64(99), parsed.ProfileID)
	assert.Equal(t, uint64(3), parsed.GrantVersion)
}

func TestTicketServiceIssueRejectsZeroGrantVersion(t *testing.T) {
	authConfig, _ := newTestTicketAuthConfig(t, 60)
	service, err := NewTicketService(authConfig)
	require.NoError(t, err)

	tokenString, expiresAt, err := service.Issue("bot-paigram", "paigram-bot", activeTestBinding(), []string{"mihomo.profile.read"}, "platform-mihomo-service", 99, 0)
	require.ErrorIs(t, err, ErrInvalidTicketConfig)
	assert.Empty(t, tokenString)
	assert.True(t, expiresAt.IsZero())
}

func TestTicketServiceRejectsTamperedSignature(t *testing.T) {
	authConfig, publicKey := newTestTicketAuthConfig(t, 60)
	service, err := NewTicketService(authConfig)
	require.NoError(t, err)

	tokenString, _, err := service.Issue("bot-paigram", "paigram-bot", activeTestBinding(), []string{"mihomo.profile.read"}, "platform-mihomo-service", 1, 1)
	require.NoError(t, err)

	parts := strings.Split(tokenString, ".")
	require.Len(t, parts, 3)
	if parts[2][0] == 'A' {
		parts[2] = "B" + parts[2][1:]
	} else {
		parts[2] = "A" + parts[2][1:]
	}

	_, err = jwt.ParseWithClaims(strings.Join(parts, "."), &ServiceTicketClaims{}, func(_ *jwt.Token) (any, error) {
		return publicKey, nil
	}, jwt.WithValidMethods([]string{contractticket.AlgorithmEd25519}))
	require.Error(t, err)
}

func TestTicketServiceRejectsExpiredTicket(t *testing.T) {
	authConfig, publicKey := newTestTicketAuthConfig(t, 1)
	service, err := NewTicketService(authConfig)
	require.NoError(t, err)

	tokenString, _, err := service.Issue("bot-paigram", "paigram-bot", activeTestBinding(), []string{"mihomo.profile.read"}, "platform-mihomo-service", 1, 1)
	require.NoError(t, err)
	time.Sleep(1100 * time.Millisecond)

	_, err = jwt.ParseWithClaims(tokenString, &ServiceTicketClaims{}, func(_ *jwt.Token) (any, error) {
		return publicKey, nil
	}, jwt.WithValidMethods([]string{contractticket.AlgorithmEd25519}))
	require.ErrorIs(t, err, jwt.ErrTokenExpired)
}

func newTestTicketAuthConfig(t *testing.T, ttlSeconds int) (config.AuthConfig, ed25519.PublicKey) {
	t.Helper()
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	privateDER, err := x509.MarshalPKCS8PrivateKey(privateKey)
	require.NoError(t, err)

	return config.AuthConfig{
		ServiceTicketTTLSeconds:    ttlSeconds,
		ServiceTicketIssuer:        "issuer",
		ServiceTicketKeyID:         "test-key",
		ServiceTicketPrivateKeyPEM: string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: privateDER})),
	}, publicKey
}

func activeTestBinding() *model.PlatformAccountBinding {
	return &model.PlatformAccountBinding{
		ID:                 42,
		OwnerUserID:        7,
		Platform:           "mihomo",
		PlatformServiceKey: "platform-mihomo-service",
		Status:             model.PlatformAccountBindingStatusActive,
	}
}

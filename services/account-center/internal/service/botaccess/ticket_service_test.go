package botaccess

import (
	"crypto/rand"
	"database/sql"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"paigram/internal/config"
	"paigram/internal/model"
)

// newTestSigningKey returns a fresh 32-byte random key, the minimum
// HS256 size enforced by NewTicketService (Path D §1.4).
func newTestSigningKey(t *testing.T) []byte {
	t.Helper()
	key := make([]byte, 32)
	_, err := rand.Read(key)
	require.NoError(t, err)
	return key
}

func TestNewTicketServiceRejectsShortSigningKey(t *testing.T) {
	service, err := NewTicketService(config.AuthConfig{
		ServiceTicketTTLSeconds: 300,
		ServiceTicketIssuer:     "issuer",
	}, []byte("too-short"))
	require.ErrorIs(t, err, ErrSigningKeyUnavailable)
	assert.Nil(t, service)
}

func TestNewTicketServiceRejectsNilSigningKey(t *testing.T) {
	service, err := NewTicketService(config.AuthConfig{
		ServiceTicketTTLSeconds: 300,
		ServiceTicketIssuer:     "issuer",
	}, nil)
	require.ErrorIs(t, err, ErrSigningKeyUnavailable)
	assert.Nil(t, service)
}

func TestNewTicketServiceRejectsZeroTTL(t *testing.T) {
	service, err := NewTicketService(config.AuthConfig{
		ServiceTicketTTLSeconds: 0,
		ServiceTicketIssuer:     "issuer",
	}, newTestSigningKey(t))
	require.ErrorIs(t, err, ErrInvalidTicketConfig)
	assert.Nil(t, service)
}

func TestTicketServiceIssueIncludesAudienceAndScopes(t *testing.T) {
	key := newTestSigningKey(t)
	service, err := NewTicketService(config.AuthConfig{
		ServiceTicketTTLSeconds: 300,
		ServiceTicketIssuer:     "issuer",
	}, key)
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
		assert.Equal(t, jwt.SigningMethodHS256.Alg(), token.Method.Alg())
		assert.Equal(t, "service_ticket", token.Header["typ"])
		// HS256 path D: no kid header — single shared key.
		_, hasKID := token.Header["kid"]
		assert.False(t, hasKID, "Path D HS256 tickets must not carry a kid header")
		return key, nil
	})
	require.NoError(t, err)
	require.True(t, token.Valid)
	assert.Equal(t, "service_ticket", parsed.Type)
	assert.Equal(t, "consumer", parsed.ActorType)
	assert.Equal(t, "paigram-bot", parsed.ActorID)
	assert.Equal(t, "paigram-bot", parsed.Consumer)
	assert.Equal(t, "bot-ticket", parsed.ClientID)
	assert.Equal(t, binding.OwnerUserID, parsed.OwnerUserID)
	assert.Equal(t, "bot-ticket", parsed.BotID)
	assert.Equal(t, binding.OwnerUserID, parsed.UserID)
	assert.Equal(t, binding.Platform, parsed.Platform)
	assert.Equal(t, binding.PlatformServiceKey, parsed.PlatformServiceKey)
	assert.Equal(t, binding.ID, parsed.BindingID)
	assert.Equal(t, "acct-42", parsed.PlatformAccountID)
	assert.ElementsMatch(t, []string{"profile:read", "messages:send"}, parsed.Scopes)
	assert.Equal(t, "issuer", parsed.Issuer)
	assert.Equal(t, "user:100", parsed.Subject)
	assert.Equal(t, []string{"platform-service"}, []string(parsed.Audience))
	assert.WithinDuration(t, expiresAt, parsed.ExpiresAt.Time, time.Second)
	assert.NotEmpty(t, parsed.ID)
}

func TestTicketServiceIssueIncludesProfileAndGrantVersion(t *testing.T) {
	key := newTestSigningKey(t)
	service, err := NewTicketService(config.AuthConfig{
		ServiceTicketIssuer:     "issuer",
		ServiceTicketTTLSeconds: 60,
	}, key)
	require.NoError(t, err)

	binding := &model.PlatformAccountBinding{
		ID:                 42,
		OwnerUserID:        7,
		Platform:           "mihomo",
		PlatformServiceKey: "platform-mihomo-service",
		Status:             model.PlatformAccountBindingStatusActive,
	}

	tokenString, _, err := service.Issue("bot-paigram", "paigram-bot", binding, []string{"mihomo.profile.read"}, "platform-mihomo-service", 99, 3)
	require.NoError(t, err)

	parsed := &ServiceTicketClaims{}
	token, err := jwt.ParseWithClaims(tokenString, parsed, func(token *jwt.Token) (any, error) {
		return key, nil
	}, jwt.WithAudience("platform-mihomo-service"), jwt.WithIssuer("issuer"))
	require.NoError(t, err)
	require.True(t, token.Valid)
	assert.Equal(t, uint64(99), parsed.ProfileID)
	assert.Equal(t, uint64(3), parsed.GrantVersion)
}

func TestTicketServiceIssueRejectsZeroGrantVersion(t *testing.T) {
	key := newTestSigningKey(t)
	service, err := NewTicketService(config.AuthConfig{
		ServiceTicketIssuer:     "issuer",
		ServiceTicketTTLSeconds: 60,
	}, key)
	require.NoError(t, err)

	binding := &model.PlatformAccountBinding{
		ID:                 42,
		OwnerUserID:        7,
		Platform:           "mihomo",
		PlatformServiceKey: "platform-mihomo-service",
		Status:             model.PlatformAccountBindingStatusActive,
	}

	tokenString, expiresAt, err := service.Issue("bot-paigram", "paigram-bot", binding, []string{"mihomo.profile.read"}, "platform-mihomo-service", 99, 0)
	require.ErrorIs(t, err, ErrInvalidTicketConfig)
	assert.Empty(t, tokenString)
	assert.True(t, expiresAt.IsZero())
}

func TestTicketServiceRejectsTamperedSignature(t *testing.T) {
	key := newTestSigningKey(t)
	service, err := NewTicketService(config.AuthConfig{
		ServiceTicketIssuer:     "issuer",
		ServiceTicketTTLSeconds: 60,
	}, key)
	require.NoError(t, err)

	binding := &model.PlatformAccountBinding{
		ID:                 1,
		OwnerUserID:        7,
		Platform:           "mihomo",
		PlatformServiceKey: "platform-mihomo-service",
		Status:             model.PlatformAccountBindingStatusActive,
	}

	tokenString, _, err := service.Issue("bot-paigram", "paigram-bot", binding, []string{"mihomo.profile.read"}, "platform-mihomo-service", 1, 1)
	require.NoError(t, err)

	// Flip a character in the signature segment to corrupt the HMAC.
	parts := strings.Split(tokenString, ".")
	require.Len(t, parts, 3)
	if parts[2][0] == 'A' {
		parts[2] = "B" + parts[2][1:]
	} else {
		parts[2] = "A" + parts[2][1:]
	}
	tampered := strings.Join(parts, ".")

	parsed := &ServiceTicketClaims{}
	_, err = jwt.ParseWithClaims(tampered, parsed, func(token *jwt.Token) (any, error) { return key, nil })
	require.Error(t, err)
}

func TestTicketServiceRejectsExpiredTicket(t *testing.T) {
	key := newTestSigningKey(t)
	// 1-second TTL so we can wait past it.
	service, err := NewTicketService(config.AuthConfig{
		ServiceTicketIssuer:     "issuer",
		ServiceTicketTTLSeconds: 1,
	}, key)
	require.NoError(t, err)

	binding := &model.PlatformAccountBinding{
		ID:                 1,
		OwnerUserID:        7,
		Platform:           "mihomo",
		PlatformServiceKey: "platform-mihomo-service",
		Status:             model.PlatformAccountBindingStatusActive,
	}

	tokenString, _, err := service.Issue("bot-paigram", "paigram-bot", binding, []string{"mihomo.profile.read"}, "platform-mihomo-service", 1, 1)
	require.NoError(t, err)

	time.Sleep(1100 * time.Millisecond)

	parsed := &ServiceTicketClaims{}
	_, err = jwt.ParseWithClaims(tokenString, parsed, func(token *jwt.Token) (any, error) { return key, nil })
	require.Error(t, err)
	require.ErrorIs(t, err, jwt.ErrTokenExpired)
}

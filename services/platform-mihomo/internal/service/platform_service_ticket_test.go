package service

import (
	"crypto/ed25519"
	"testing"
	"time"

	contractticket "github.com/PaiGramTeam/paigram-account-center/contracts/runtime/go/serviceticket"
	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/require"

	"platform-mihomo-service/internal/data"
)

const (
	serviceTestAudience = "platform-mihomo-service"
	serviceTestIssuer   = "paigram-account-center"
	serviceTestKeyID    = "service-test-key"
)

var (
	serviceTestSigningKey       = []byte("0123456789abcdef0123456789abcdef")
	serviceTestTicketPrivateKey = ed25519.NewKeyFromSeed([]byte("0123456789abcdef0123456789abcdef"))
)

func signedServiceTicket(t *testing.T) string {
	return signedServiceTicketForAccount(t, "", "mihomo.credential.bind")
}

func signedServiceTicketForAccount(t *testing.T, platformAccountID string, scopes ...string) string {
	t.Helper()

	claims := jwt.MapClaims{
		"iss":                  serviceTestIssuer,
		"aud":                  []string{serviceTestAudience},
		"actor_type":           "user",
		"actor_id":             "bot-paigram",
		"owner_user_id":        float64(1),
		"binding_id":           float64(101),
		"bot_id":               "bot-paigram",
		"platform":             "mihomo",
		"platform_service_key": serviceTestAudience,
		"exp":                  time.Now().Add(time.Minute).Unix(),
	}
	if platformAccountID != "" {
		claims["platform_account_id"] = platformAccountID
	}
	if len(scopes) > 0 {
		claims["scopes"] = scopes
	}
	return signedServiceTestJWT(t, claims)
}

func signedServiceTicketForProfile(t *testing.T, platformAccountID string, profileID uint64, scopes ...string) string {
	t.Helper()

	claims := jwt.MapClaims{
		"iss":                  serviceTestIssuer,
		"aud":                  []string{serviceTestAudience},
		"actor_type":           "user",
		"actor_id":             "bot-paigram",
		"owner_user_id":        float64(1),
		"binding_id":           float64(101),
		"bot_id":               "bot-paigram",
		"platform":             "mihomo",
		"platform_service_key": serviceTestAudience,
		"platform_account_id":  platformAccountID,
		"profile_id":           float64(profileID),
		"exp":                  time.Now().Add(time.Minute).Unix(),
	}
	if len(scopes) > 0 {
		claims["scopes"] = scopes
	}

	return signedServiceTestJWT(t, claims)
}

func signedServiceTestJWT(t *testing.T, claims jwt.MapClaims) string {
	t.Helper()

	now := time.Now()
	if _, ok := claims["iat"]; !ok {
		claims["iat"] = now.Unix()
	}
	if _, ok := claims["nbf"]; !ok {
		claims["nbf"] = now.Add(-time.Second).Unix()
	}
	if _, ok := claims["jti"]; !ok {
		claims["jti"] = "service-test-ticket"
	}
	ticketType := contractticket.TypeControl
	actorType, _ := claims["actor_type"].(string)
	switch actorType {
	case "consumer":
		ticketType = contractticket.TypeDelegation
		consumer, _ := claims["consumer"].(string)
		claims["sub"] = "consumer:" + consumer
	case "system":
		claims["sub"] = "system:account-center"
	default:
		claims["sub"] = "user:1"
	}

	token := jwt.NewWithClaims(contractticket.SigningMethodEd25519, claims)
	token.Header["kid"] = serviceTestKeyID
	token.Header["typ"] = ticketType
	signed, err := token.SignedString(serviceTestTicketPrivateKey)
	require.NoError(t, err)
	return signed
}

func serviceTestTicketVerifier() *data.TicketVerifier {
	return data.NewStaticKeyTicketVerifier(serviceTestIssuer, serviceTestKeyID, serviceTestTicketPrivateKey.Public().(ed25519.PublicKey))
}

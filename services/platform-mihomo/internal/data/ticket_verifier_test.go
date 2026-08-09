package data

import (
	"context"
	"crypto/ed25519"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/require"
)

const (
	testTicketIssuer   = "paigram-account-center"
	testTicketAudience = "platform-mihomo-service"
	testTicketKeyID    = "service-ticket-test-key"
)

var testTicketPrivateKey = ed25519.NewKeyFromSeed([]byte("0123456789abcdef0123456789abcdef"))

func TestVerifyServiceTicketAcceptsEd25519SignedTicketWithKID(t *testing.T) {
	verifier := testTicketVerifier()
	raw := issueTestTicket(t, map[string]any{
		"actor_type":          "consumer",
		"actor_id":            "user-1",
		"owner_user_id":       float64(1),
		"binding_id":          float64(101),
		"platform":            "mihomo",
		"platform_account_id": "binding_101_10001",
		"consumer":            "paigram",
		"grant_version":       float64(2),
		"scopes":              []string{"mihomo.profile.read"},
	})

	claims, err := verifier.Verify(raw, testTicketAudience)
	require.NoError(t, err)
	require.Equal(t, "consumer", claims.ActorType)
	require.Equal(t, uint64(101), claims.BindingID)
	require.Equal(t, []string{"mihomo.profile.read"}, claims.Scopes)
}

func TestVerifyServiceTicketRejectsHS256Ticket(t *testing.T) {
	verifier := testTicketVerifier()
	raw := issueHS256TestTicket(t, map[string]any{
		"actor_type":    "consumer",
		"actor_id":      "user-1",
		"owner_user_id": float64(1),
		"binding_id":    float64(101),
		"platform":      "mihomo",
		"consumer":      "paigram",
		"grant_version": float64(2),
	})

	_, err := verifier.Verify(raw, testTicketAudience)
	require.ErrorContains(t, err, "unexpected signing method")
}

func TestVerifyServiceTicketRejectsMissingKID(t *testing.T) {
	verifier := testTicketVerifier()
	raw := issueTestTicketWithHeaders(t, map[string]any{
		"actor_type":    "consumer",
		"actor_id":      "user-1",
		"owner_user_id": float64(1),
		"binding_id":    float64(101),
		"platform":      "mihomo",
		"consumer":      "paigram",
		"grant_version": float64(2),
	}, map[string]any{"typ": "service_ticket"})

	_, err := verifier.Verify(raw, testTicketAudience)
	require.ErrorContains(t, err, "kid")
}

func TestVerifyServiceTicketRejectsWrongKID(t *testing.T) {
	verifier := testTicketVerifier()
	raw := issueTestTicketWithHeaders(t, map[string]any{
		"actor_type":    "consumer",
		"actor_id":      "user-1",
		"owner_user_id": float64(1),
		"binding_id":    float64(101),
		"platform":      "mihomo",
		"consumer":      "paigram",
		"grant_version": float64(2),
	}, map[string]any{"kid": "wrong-key", "typ": "service_ticket"})

	_, err := verifier.Verify(raw, testTicketAudience)
	require.ErrorContains(t, err, "public key")
}

func TestVerifyServiceTicketRejectsWrongType(t *testing.T) {
	verifier := testTicketVerifier()
	raw := issueTestTicketWithHeaders(t, map[string]any{
		"actor_type":    "consumer",
		"actor_id":      "user-1",
		"owner_user_id": float64(1),
		"binding_id":    float64(101),
		"platform":      "mihomo",
		"consumer":      "paigram",
		"grant_version": float64(2),
	}, map[string]any{"kid": testTicketKeyID, "typ": "access_token"})

	_, err := verifier.Verify(raw, testTicketAudience)
	require.ErrorContains(t, err, "service_ticket")
}

func TestVerifyServiceTicketRejectsMissingType(t *testing.T) {
	verifier := testTicketVerifier()
	raw := issueTestTicketWithHeaders(t, map[string]any{
		"actor_type":    "consumer",
		"actor_id":      "user-1",
		"owner_user_id": float64(1),
		"binding_id":    float64(101),
		"platform":      "mihomo",
		"consumer":      "paigram",
		"grant_version": float64(2),
	}, map[string]any{"kid": testTicketKeyID})

	_, err := verifier.Verify(raw, testTicketAudience)
	require.ErrorContains(t, err, "service_ticket")
}

func TestVerifyServiceTicketRejectsMissingTypeHeaderWithPayloadType(t *testing.T) {
	verifier := testTicketVerifier()
	raw := issueTestTicketWithHeaders(t, map[string]any{
		"typ":           "service_ticket",
		"actor_type":    "consumer",
		"actor_id":      "user-1",
		"owner_user_id": float64(1),
		"binding_id":    float64(101),
		"platform":      "mihomo",
		"consumer":      "paigram",
		"grant_version": float64(2),
	}, map[string]any{"kid": testTicketKeyID})

	_, err := verifier.Verify(raw, testTicketAudience)
	require.ErrorContains(t, err, "service_ticket")
}

func TestVerifyServiceTicketRejectsMissingExpiration(t *testing.T) {
	verifier := testTicketVerifier()
	raw := issueTestTicketWithClaimsAndHeaders(t, map[string]any{
		"iss":           testTicketIssuer,
		"aud":           []string{testTicketAudience},
		"actor_type":    "consumer",
		"actor_id":      "user-1",
		"owner_user_id": float64(1),
		"binding_id":    float64(101),
		"platform":      "mihomo",
		"consumer":      "paigram",
		"grant_version": float64(2),
	}, map[string]any{"kid": testTicketKeyID, "typ": "service_ticket"})

	_, err := verifier.Verify(raw, testTicketAudience)
	require.ErrorContains(t, err, "exp")
}

func TestVerifyServiceTicketRejectsWrongIssuer(t *testing.T) {
	verifier := testTicketVerifier()
	raw := issueTestTicketWithClaimsAndHeaders(t, map[string]any{
		"iss":           "wrong-issuer",
		"aud":           []string{testTicketAudience},
		"exp":           time.Now().Add(time.Minute).Unix(),
		"actor_type":    "consumer",
		"actor_id":      "user-1",
		"owner_user_id": float64(1),
		"binding_id":    float64(101),
		"platform":      "mihomo",
		"consumer":      "paigram",
		"grant_version": float64(2),
	}, map[string]any{"kid": testTicketKeyID, "typ": "service_ticket"})

	_, err := verifier.Verify(raw, testTicketAudience)
	require.ErrorContains(t, err, "issuer")
}

func TestVerifyServiceTicketRejectsWrongAudience(t *testing.T) {
	verifier := testTicketVerifier()
	raw := issueTestTicketWithClaimsAndHeaders(t, map[string]any{
		"iss":           testTicketIssuer,
		"aud":           []string{"wrong-audience"},
		"exp":           time.Now().Add(time.Minute).Unix(),
		"actor_type":    "consumer",
		"actor_id":      "user-1",
		"owner_user_id": float64(1),
		"binding_id":    float64(101),
		"platform":      "mihomo",
		"consumer":      "paigram",
		"grant_version": float64(2),
	}, map[string]any{"kid": testTicketKeyID, "typ": "service_ticket"})

	_, err := verifier.Verify(raw, testTicketAudience)
	require.ErrorContains(t, err, "audience")
}

func TestVerifyAcceptsBindingAwareClaims(t *testing.T) {
	verifier := testTicketVerifier()
	raw := issueTestTicket(t, map[string]any{
		"actor_type":          "consumer",
		"actor_id":            "user-1",
		"owner_user_id":       float64(1),
		"binding_id":          float64(101),
		"platform":            "mihomo",
		"platform_account_id": "binding_101_10001",
		"consumer":            "paigram",
		"grant_version":       float64(2),
		"profile_id":          float64(1001),
		"scopes":              []string{"mihomo.profile.read"},
	})

	claims, err := verifier.Verify(raw, testTicketAudience)
	require.NoError(t, err)
	require.Equal(t, "consumer", claims.ActorType)
	require.Equal(t, "user-1", claims.ActorID)
	require.Equal(t, uint64(1), claims.OwnerUserID)
	require.Equal(t, uint64(101), claims.BindingID)
	require.Equal(t, "mihomo", claims.Platform)
	require.Equal(t, "binding_101_10001", claims.PlatformAccountID)
	require.Equal(t, "paigram", claims.Consumer)
	require.Equal(t, uint64(2), claims.GrantVersion)
	require.Equal(t, uint64(1001), claims.ProfileID)
	require.Equal(t, []string{"mihomo.profile.read"}, claims.Scopes)
	require.Equal(t, testTicketAudience, claims.Audience)
	require.Equal(t, uint64(101), claims.PlatformAccountRefID)
}

func TestVerifyNormalizesAllowedActionsClaim(t *testing.T) {
	verifier := testTicketVerifier()
	raw := issueTestTicket(t, map[string]any{
		"actor_type":          "consumer",
		"actor_id":            "user-1",
		"owner_user_id":       float64(1),
		"binding_id":          float64(101),
		"platform":            "mihomo",
		"platform_account_id": "binding_101_10001",
		"consumer":            "paigram",
		"grant_version":       float64(2),
		"allowed_actions":     []string{"mihomo.profile.read", "mihomo.profile.write"},
	})

	claims, err := verifier.Verify(raw, testTicketAudience)
	require.NoError(t, err)
	require.Equal(t, []string{"mihomo.profile.read", "mihomo.profile.write"}, claims.Scopes)
}

func TestVerifyPrefersAllowedActionsOverScopes(t *testing.T) {
	verifier := testTicketVerifier()
	raw := issueTestTicket(t, map[string]any{
		"actor_type":          "consumer",
		"actor_id":            "user-1",
		"owner_user_id":       float64(1),
		"binding_id":          float64(101),
		"platform":            "mihomo",
		"platform_account_id": "binding_101_10001",
		"consumer":            "paigram",
		"grant_version":       float64(2),
		"scopes":              []string{"mihomo.profile.read"},
		"allowed_actions":     []string{"mihomo.profile.write"},
	})

	claims, err := verifier.Verify(raw, testTicketAudience)
	require.NoError(t, err)
	require.Equal(t, []string{"mihomo.profile.write"}, claims.Scopes)
}

func TestVerifyRejectsMissingBindingID(t *testing.T) {
	verifier := testTicketVerifier()
	raw := issueTestTicket(t, map[string]any{
		"actor_type":    "consumer",
		"actor_id":      "user-1",
		"owner_user_id": float64(1),
		"platform":      "mihomo",
		"consumer":      "paigram",
		"grant_version": float64(2),
		"scopes":        []string{"mihomo.profile.read"},
	})

	_, err := verifier.Verify(raw, testTicketAudience)
	require.ErrorContains(t, err, "binding_id")
}

func TestVerifyRejectsConsumerActorWithoutConsumerClaim(t *testing.T) {
	verifier := testTicketVerifier()
	raw := issueTestTicket(t, map[string]any{
		"actor_type":    "consumer",
		"actor_id":      "user-1",
		"owner_user_id": float64(1),
		"binding_id":    float64(101),
		"platform":      "mihomo",
		"grant_version": float64(2),
		"scopes":        []string{"mihomo.profile.read"},
	})

	_, err := verifier.Verify(raw, testTicketAudience)
	require.ErrorContains(t, err, "consumer")
}

func TestVerifyRejectsConsumerActorWithoutGrantVersionClaim(t *testing.T) {
	verifier := testTicketVerifier()
	raw := issueTestTicket(t, map[string]any{
		"actor_type":    "consumer",
		"actor_id":      "user-1",
		"owner_user_id": float64(1),
		"binding_id":    float64(101),
		"platform":      "mihomo",
		"consumer":      "paigram",
		"scopes":        []string{"mihomo.profile.read"},
	})

	_, err := verifier.Verify(raw, testTicketAudience)
	require.ErrorContains(t, err, "grant_version")
}

func TestVerifyAllowsUserAndAdminActorsWithoutGrantVersionClaim(t *testing.T) {
	for _, actorType := range []string{"user", "admin"} {
		t.Run(actorType, func(t *testing.T) {
			verifier := testTicketVerifier()
			raw := issueTestTicket(t, map[string]any{
				"actor_type":    actorType,
				"actor_id":      actorType + "-1",
				"owner_user_id": float64(1),
				"binding_id":    float64(101),
				"platform":      "mihomo",
				"scopes":        []string{"mihomo.profile.read"},
			})

			claims, err := verifier.Verify(raw, testTicketAudience)
			require.NoError(t, err)
			require.Equal(t, actorType, claims.ActorType)
		})
	}
}

func TestVerifyContextRejectsStaleConsumerGrantVersion(t *testing.T) {
	lookup := &fakeGrantVersionLookup{minimum: 5}
	verifier := testTicketVerifier().WithGrantVersionLookup(lookup)
	raw := issueTestTicket(t, map[string]any{
		"actor_type":    "consumer",
		"actor_id":      "user-1",
		"owner_user_id": float64(1),
		"binding_id":    float64(101),
		"platform":      "mihomo",
		"consumer":      "paigram",
		"grant_version": float64(4),
		"scopes":        []string{"mihomo.profile.read"},
	})

	_, err := verifier.VerifyContext(context.Background(), raw, testTicketAudience)
	require.ErrorContains(t, err, "grant version revoked")
	require.Equal(t, uint64(101), lookup.bindingID)
	require.Equal(t, "paigram", lookup.consumer)
}

func TestVerifyAllowsNonConsumerActorTypesWithoutConsumerClaim(t *testing.T) {
	verifier := testTicketVerifier()
	raw := issueTestTicket(t, map[string]any{
		"actor_type":    "robot",
		"actor_id":      "user-1",
		"owner_user_id": float64(1),
		"binding_id":    float64(101),
		"platform":      "mihomo",
		"scopes":        []string{"mihomo.profile.read"},
	})

	claims, err := verifier.Verify(raw, testTicketAudience)
	require.NoError(t, err)
	require.Equal(t, "robot", claims.ActorType)
}

func TestVerifyRejectsMismatchedLegacyPlatformAccountRefID(t *testing.T) {
	verifier := testTicketVerifier()
	raw := issueTestTicket(t, map[string]any{
		"actor_type":              "consumer",
		"actor_id":                "user-1",
		"owner_user_id":           float64(1),
		"binding_id":              float64(101),
		"platform":                "mihomo",
		"consumer":                "paigram",
		"grant_version":           float64(2),
		"platform_account_ref_id": float64(202),
	})

	_, err := verifier.Verify(raw, testTicketAudience)
	require.ErrorContains(t, err, "binding_id does not match platform_account_ref_id")
}

type fakeGrantVersionLookup struct {
	minimum   uint64
	bindingID uint64
	consumer  string
}

func (f *fakeGrantVersionLookup) MinimumVersion(_ context.Context, bindingID uint64, consumer string) (uint64, error) {
	f.bindingID = bindingID
	f.consumer = consumer
	return f.minimum, nil
}

func testTicketVerifier() *TicketVerifier {
	return NewStaticKeyTicketVerifier(testTicketIssuer, testTicketKeyID, testTicketPrivateKey.Public().(ed25519.PublicKey))
}

func issueTestTicket(t *testing.T, claims map[string]any) string {
	t.Helper()

	return issueTestTicketWithHeaders(t, claims, map[string]any{"kid": testTicketKeyID, "typ": "service_ticket"})
}

func issueTestTicketWithHeaders(t *testing.T, claims map[string]any, headers map[string]any) string {
	t.Helper()

	baseClaims := jwt.MapClaims{
		"iss": testTicketIssuer,
		"aud": []string{testTicketAudience},
		"exp": time.Now().Add(time.Minute).Unix(),
	}
	for key, value := range claims {
		baseClaims[key] = value
	}
	return issueTestTicketWithClaimsAndHeaders(t, baseClaims, headers)
}

func issueTestTicketWithClaimsAndHeaders(t *testing.T, claims map[string]any, headers map[string]any) string {
	t.Helper()

	token := jwt.NewWithClaims(jwt.SigningMethodEdDSA, jwt.MapClaims(claims))
	for key, value := range headers {
		token.Header[key] = value
	}
	signed, err := token.SignedString(testTicketPrivateKey)
	require.NoError(t, err)

	return signed
}

func issueHS256TestTicket(t *testing.T, claims map[string]any) string {
	t.Helper()

	baseClaims := jwt.MapClaims{
		"iss": testTicketIssuer,
		"aud": []string{testTicketAudience},
		"exp": time.Now().Add(time.Minute).Unix(),
	}
	for key, value := range claims {
		baseClaims[key] = value
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, baseClaims)
	token.Header["kid"] = testTicketKeyID
	token.Header["typ"] = "service_ticket"
	signed, err := token.SignedString([]byte("0123456789abcdef0123456789abcdef"))
	require.NoError(t, err)

	return signed
}

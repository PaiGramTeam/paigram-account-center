package data

import (
	"context"
	"crypto/ed25519"
	"crypto/x509"
	"encoding/pem"
	"testing"
	"time"

	contractticket "github.com/PaiGramTeam/paigram-account-center/contracts/runtime/go/serviceticket"
	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/require"
)

func TestVerifyAcceptsTicketFromProductionContractIssuer(t *testing.T) {
	privateDER, err := x509.MarshalPKCS8PrivateKey(testTicketPrivateKey)
	require.NoError(t, err)
	issuer, err := contractticket.NewIssuer(contractticket.IssuerConfig{
		Issuer:        testTicketIssuer,
		KeyID:         testTicketKeyID,
		TTL:           time.Minute,
		PrivateKeyPEM: string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: privateDER})),
	})
	require.NoError(t, err)
	raw, _, err := issuer.Issue(contractticket.TypeDelegation, "consumer:paigram", testTicketAudience, contractticket.Claims{
		ActorType: "consumer", ActorID: "paigram", Consumer: "paigram", ConsumerPrincipal: "paigram",
		OwnerUserRef: "usr-1", EntryIdentityRef: "entry-1", OwnerEpoch: 1, ConsumerEpoch: 1, EntryEpoch: 1, CredentialGeneration: 1,
		BindingRef: "binding-101", Platform: "mihomo", AccountKey: "account-101",
		GrantVersion: 2, ProfileRef: "profile-1", Scopes: []string{"mihomo.profile.read"},
		AllowedActions: []string{"mihomo.profile.read"},
	})
	require.NoError(t, err)

	claims, err := testTicketVerifier().Verify(raw, testTicketAudience)
	require.NoError(t, err)
	require.Equal(t, contractticket.TypeDelegation, claims.TicketType)
	require.Equal(t, "binding-101", claims.BindingRef)
	require.Equal(t, "account-101", claims.AccountKey)
	require.Equal(t, "profile-1", claims.ProfileRef)
	require.Equal(t, []string{"mihomo.profile.read"}, claims.Scopes)
}

func TestVerifyAcceptsFullySpecifiedDelegationTicket(t *testing.T) {
	verifier := testTicketVerifier()
	token := jwt.NewWithClaims(contractticket.SigningMethodEd25519, jwt.MapClaims{
		"iss":                   testTicketIssuer,
		"sub":                   "consumer:paigram",
		"aud":                   []string{testTicketAudience},
		"iat":                   time.Now().Add(-time.Second).Unix(),
		"nbf":                   time.Now().Add(-time.Second).Unix(),
		"exp":                   time.Now().Add(time.Minute).Unix(),
		"jti":                   "ticket-1",
		"actor_type":            "consumer",
		"actor_id":              "paigram",
		"consumer":              "paigram",
		"consumer_principal":    "paigram",
		"owner_user_ref":        "usr-1",
		"entry_identity_ref":    "entry-1",
		"owner_epoch":           float64(1),
		"consumer_epoch":        float64(1),
		"entry_epoch":           float64(1),
		"credential_generation": float64(1),
		"binding_ref":           "binding-101",
		"platform":              "mihomo",
		"grant_version":         float64(2),
		"allowed_actions":       []string{"mihomo.profile.read"},
	})
	token.Header["kid"] = testTicketKeyID
	token.Header["typ"] = "paigram-platform-delegation+jwt"
	raw, err := token.SignedString(testTicketPrivateKey)
	require.NoError(t, err)

	claims, err := verifier.Verify(raw, testTicketAudience)
	require.NoError(t, err)
	require.Equal(t, contractticket.TypeDelegation, claims.TicketType)
	require.Equal(t, "paigram", claims.Consumer)
	require.Equal(t, []string{"mihomo.profile.read"}, claims.Scopes)
}

func TestVerifyAcceptsControlTicketWithStableOwnerSubject(t *testing.T) {
	raw := issueTestTicket(t, map[string]any{
		"actor_type":     "user",
		"actor_id":       "session-1",
		"owner_user_ref": "usr-stable-1",
		"sub":            "user:usr-stable-1",
		"binding_ref":    "binding-101",
		"platform":       "mihomo",
		"scopes":         []string{"mihomo.binding.read"},
	})

	claims, err := testTicketVerifier().Verify(raw, testTicketAudience)
	require.NoError(t, err)
	require.Equal(t, "usr-stable-1", claims.OwnerUserRef)
}

const (
	testTicketIssuer   = "paigram-account-center"
	testTicketAudience = "platform-mihomo-service"
	testTicketKeyID    = "service-ticket-test-key"
)

var testTicketPrivateKey = ed25519.NewKeyFromSeed([]byte("0123456789abcdef0123456789abcdef"))

func TestVerifyServiceTicketAcceptsEd25519SignedTicketWithKID(t *testing.T) {
	verifier := testTicketVerifier()
	raw := issueTestTicket(t, map[string]any{
		"actor_type":    "consumer",
		"actor_id":      "user-1",
		"binding_ref":   "binding-101",
		"platform":      "mihomo",
		"account_key":   "binding_101_10001",
		"consumer":      "paigram",
		"grant_version": float64(2),
		"scopes":        []string{"mihomo.profile.read"},
	})

	claims, err := verifier.Verify(raw, testTicketAudience)
	require.NoError(t, err)
	require.Equal(t, "consumer", claims.ActorType)
	require.Equal(t, "binding-101", claims.BindingRef)
	require.Equal(t, []string{"mihomo.profile.read"}, claims.Scopes)
}

func TestVerifyServiceTicketRejectsHS256Ticket(t *testing.T) {
	verifier := testTicketVerifier()
	raw := issueHS256TestTicket(t, map[string]any{
		"actor_type":    "consumer",
		"actor_id":      "user-1",
		"binding_ref":   "binding-101",
		"platform":      "mihomo",
		"consumer":      "paigram",
		"grant_version": float64(2),
	})

	_, err := verifier.Verify(raw, testTicketAudience)
	require.ErrorContains(t, err, "signing method HS256 is invalid")
}

func TestVerifyServiceTicketRejectsMultipleAudiences(t *testing.T) {
	now := time.Now().UTC()
	raw := issueTestTicketWithClaimsAndHeaders(t, jwt.MapClaims{
		"iss": testTicketIssuer, "sub": "user:usr-1", "aud": []string{testTicketAudience, "other-service"},
		"iat": now.Unix(), "nbf": now.Unix(), "exp": now.Add(time.Minute).Unix(), "jti": "multi-audience",
		"actor_type": "user", "actor_id": "session-1", "owner_user_ref": "usr-1",
		"binding_ref": "binding-101", "platform": "mihomo", "scopes": []string{"mihomo.binding.read"},
	}, map[string]any{"kid": testTicketKeyID, "typ": contractticket.TypeControl})

	_, err := testTicketVerifier().Verify(raw, testTicketAudience)
	require.ErrorContains(t, err, "single exact value")
}

func TestVerifyServiceTicketRejectsLifetimeOverFiveMinutes(t *testing.T) {
	now := time.Now().UTC()
	raw := issueTestTicketWithClaimsAndHeaders(t, jwt.MapClaims{
		"iss": testTicketIssuer, "sub": "user:usr-1", "aud": []string{testTicketAudience},
		"iat": now.Unix(), "nbf": now.Unix(), "exp": now.Add(6 * time.Minute).Unix(), "jti": "long-lived",
		"actor_type": "user", "actor_id": "session-1", "owner_user_ref": "usr-1",
		"binding_ref": "binding-101", "platform": "mihomo", "scopes": []string{"mihomo.binding.read"},
	}, map[string]any{"kid": testTicketKeyID, "typ": contractticket.TypeControl})

	_, err := testTicketVerifier().Verify(raw, testTicketAudience)
	require.ErrorContains(t, err, "lifetime")
}

func TestVerifyServiceTicketRejectsMissingKID(t *testing.T) {
	verifier := testTicketVerifier()
	raw := issueTestTicketWithHeaders(t, map[string]any{
		"actor_type":    "consumer",
		"actor_id":      "user-1",
		"binding_ref":   "binding-101",
		"platform":      "mihomo",
		"consumer":      "paigram",
		"grant_version": float64(2),
	}, map[string]any{"typ": contractticket.TypeDelegation})

	_, err := verifier.Verify(raw, testTicketAudience)
	require.ErrorContains(t, err, "kid")
}

func TestVerifyServiceTicketRejectsWrongKID(t *testing.T) {
	verifier := testTicketVerifier()
	raw := issueTestTicketWithHeaders(t, map[string]any{
		"actor_type":    "consumer",
		"actor_id":      "user-1",
		"binding_ref":   "binding-101",
		"platform":      "mihomo",
		"consumer":      "paigram",
		"grant_version": float64(2),
	}, map[string]any{"kid": "wrong-key", "typ": contractticket.TypeDelegation})

	_, err := verifier.Verify(raw, testTicketAudience)
	require.ErrorContains(t, err, "public key")
}

func TestVerifyServiceTicketRejectsWrongType(t *testing.T) {
	verifier := testTicketVerifier()
	raw := issueTestTicketWithHeaders(t, map[string]any{
		"actor_type":    "consumer",
		"actor_id":      "user-1",
		"binding_ref":   "binding-101",
		"platform":      "mihomo",
		"consumer":      "paigram",
		"grant_version": float64(2),
	}, map[string]any{"kid": testTicketKeyID, "typ": "access_token"})

	_, err := verifier.Verify(raw, testTicketAudience)
	require.ErrorContains(t, err, "typ")
}

func TestVerifyServiceTicketRejectsMissingType(t *testing.T) {
	verifier := testTicketVerifier()
	raw := issueTestTicketWithHeaders(t, map[string]any{
		"actor_type":    "consumer",
		"actor_id":      "user-1",
		"binding_ref":   "binding-101",
		"platform":      "mihomo",
		"consumer":      "paigram",
		"grant_version": float64(2),
	}, map[string]any{"kid": testTicketKeyID})

	_, err := verifier.Verify(raw, testTicketAudience)
	require.ErrorContains(t, err, "typ")
}

func TestVerifyServiceTicketRejectsMissingTypeHeaderWithPayloadType(t *testing.T) {
	verifier := testTicketVerifier()
	raw := issueTestTicketWithHeaders(t, map[string]any{
		"typ":           contractticket.TypeDelegation,
		"actor_type":    "consumer",
		"actor_id":      "user-1",
		"binding_ref":   "binding-101",
		"platform":      "mihomo",
		"consumer":      "paigram",
		"grant_version": float64(2),
	}, map[string]any{"kid": testTicketKeyID})

	_, err := verifier.Verify(raw, testTicketAudience)
	require.ErrorContains(t, err, "typ")
}

func TestVerifyServiceTicketRejectsMissingExpiration(t *testing.T) {
	verifier := testTicketVerifier()
	raw := issueTestTicketWithClaimsAndHeaders(t, map[string]any{
		"iss":           testTicketIssuer,
		"aud":           []string{testTicketAudience},
		"actor_type":    "consumer",
		"actor_id":      "user-1",
		"binding_ref":   "binding-101",
		"platform":      "mihomo",
		"consumer":      "paigram",
		"grant_version": float64(2),
	}, map[string]any{"kid": testTicketKeyID, "typ": contractticket.TypeDelegation})

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
		"binding_ref":   "binding-101",
		"platform":      "mihomo",
		"consumer":      "paigram",
		"grant_version": float64(2),
	}, map[string]any{"kid": testTicketKeyID, "typ": contractticket.TypeDelegation})

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
		"binding_ref":   "binding-101",
		"platform":      "mihomo",
		"consumer":      "paigram",
		"grant_version": float64(2),
	}, map[string]any{"kid": testTicketKeyID, "typ": contractticket.TypeDelegation})

	_, err := verifier.Verify(raw, testTicketAudience)
	require.ErrorContains(t, err, "audience")
}

func TestVerifyAcceptsBindingAwareClaims(t *testing.T) {
	verifier := testTicketVerifier()
	raw := issueTestTicket(t, map[string]any{
		"actor_type":    "consumer",
		"actor_id":      "user-1",
		"binding_ref":   "binding-101",
		"platform":      "mihomo",
		"account_key":   "binding_101_10001",
		"consumer":      "paigram",
		"grant_version": float64(2),
		"profile_ref":   "profile-1001",
		"scopes":        []string{"mihomo.profile.read"},
	})

	claims, err := verifier.Verify(raw, testTicketAudience)
	require.NoError(t, err)
	require.Equal(t, "consumer", claims.ActorType)
	require.Equal(t, "user-1", claims.ActorID)
	require.Equal(t, "binding-101", claims.BindingRef)
	require.Equal(t, "mihomo", claims.Platform)
	require.Equal(t, "binding_101_10001", claims.AccountKey)
	require.Equal(t, "paigram", claims.Consumer)
	require.Equal(t, uint64(2), claims.GrantVersion)
	require.Equal(t, "profile-1001", claims.ProfileRef)
	require.Equal(t, []string{"mihomo.profile.read"}, claims.Scopes)
	require.Equal(t, testTicketAudience, claims.Audience)
}

func TestVerifyNormalizesAllowedActionsClaim(t *testing.T) {
	verifier := testTicketVerifier()
	raw := issueTestTicket(t, map[string]any{
		"actor_type":      "consumer",
		"actor_id":        "user-1",
		"binding_ref":     "binding-101",
		"platform":        "mihomo",
		"account_key":     "binding_101_10001",
		"consumer":        "paigram",
		"grant_version":   float64(2),
		"allowed_actions": []string{"mihomo.profile.read", "mihomo.profile.write"},
	})

	claims, err := verifier.Verify(raw, testTicketAudience)
	require.NoError(t, err)
	require.Equal(t, []string{"mihomo.profile.read", "mihomo.profile.write"}, claims.Scopes)
}

func TestVerifyPrefersAllowedActionsOverScopes(t *testing.T) {
	verifier := testTicketVerifier()
	raw := issueTestTicket(t, map[string]any{
		"actor_type":      "consumer",
		"actor_id":        "user-1",
		"binding_ref":     "binding-101",
		"platform":        "mihomo",
		"account_key":     "binding_101_10001",
		"consumer":        "paigram",
		"grant_version":   float64(2),
		"scopes":          []string{"mihomo.profile.read"},
		"allowed_actions": []string{"mihomo.profile.write"},
	})

	claims, err := verifier.Verify(raw, testTicketAudience)
	require.NoError(t, err)
	require.Equal(t, []string{"mihomo.profile.write"}, claims.Scopes)
}

func TestVerifyRejectsMissingBindingRef(t *testing.T) {
	verifier := testTicketVerifier()
	raw := issueTestTicket(t, map[string]any{
		"actor_type":    "consumer",
		"actor_id":      "user-1",
		"platform":      "mihomo",
		"consumer":      "paigram",
		"grant_version": float64(2),
		"scopes":        []string{"mihomo.profile.read"},
	})

	_, err := verifier.Verify(raw, testTicketAudience)
	require.ErrorContains(t, err, "binding_ref")
}

func TestVerifyRejectsConsumerActorWithoutConsumerClaim(t *testing.T) {
	verifier := testTicketVerifier()
	raw := issueTestTicket(t, map[string]any{
		"actor_type":    "consumer",
		"actor_id":      "user-1",
		"binding_ref":   "binding-101",
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
		"actor_type":  "consumer",
		"actor_id":    "user-1",
		"binding_ref": "binding-101",
		"platform":    "mihomo",
		"consumer":    "paigram",
		"scopes":      []string{"mihomo.profile.read"},
	})

	_, err := verifier.Verify(raw, testTicketAudience)
	require.ErrorContains(t, err, "authorization version")
}

func TestVerifyAllowsUserAndAdminActorsWithoutGrantVersionClaim(t *testing.T) {
	for _, actorType := range []string{"user", "admin"} {
		t.Run(actorType, func(t *testing.T) {
			verifier := testTicketVerifier()
			raw := issueTestTicket(t, map[string]any{
				"actor_type":  actorType,
				"actor_id":    actorType + "-1",
				"binding_ref": "binding-101",
				"platform":    "mihomo",
				"scopes":      []string{"mihomo.profile.read"},
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
		"binding_ref":   "binding-101",
		"platform":      "mihomo",
		"consumer":      "paigram",
		"grant_version": float64(4),
		"scopes":        []string{"mihomo.profile.read"},
	})

	_, err := verifier.VerifyContext(context.Background(), raw, testTicketAudience)
	require.ErrorContains(t, err, "grant version revoked")
	require.Equal(t, "binding-101", lookup.bindingRef)
	require.Equal(t, "paigram", lookup.consumer)
}

func TestVerifyContextRejectsStaleAuthorizationState(t *testing.T) {
	tests := []struct {
		name  string
		state AuthorizationState
	}{
		{name: "grant", state: AuthorizationState{MinimumGrantVersion: 3, CredentialGeneration: 1}},
		{name: "owner", state: AuthorizationState{MinimumOwnerEpoch: 2, CredentialGeneration: 1}},
		{name: "consumer", state: AuthorizationState{MinimumConsumerEpoch: 2, CredentialGeneration: 1}},
		{name: "entry", state: AuthorizationState{MinimumEntryEpoch: 2, CredentialGeneration: 1}},
		{name: "generation", state: AuthorizationState{CredentialGeneration: 2}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			verifier := testTicketVerifier().WithAuthorizationStateLookup(fakeAuthorizationStateLookup{state: test.state})
			raw := issueTestTicket(t, map[string]any{
				"actor_type": "consumer", "actor_id": "paigram",
				"binding_ref": "binding-101", "platform": "mihomo", "consumer": "paigram",
				"grant_version": float64(2), "scopes": []string{"mihomo.profile.read"},
			})

			_, err := verifier.VerifyContext(context.Background(), raw, testTicketAudience)
			require.ErrorIs(t, err, ErrGrantVersionRevoked)
		})
	}
}

func TestVerifyRejectsUnsupportedNonConsumerActorType(t *testing.T) {
	verifier := testTicketVerifier()
	raw := issueTestTicket(t, map[string]any{
		"actor_type":  "robot",
		"actor_id":    "user-1",
		"binding_ref": "binding-101",
		"platform":    "mihomo",
		"scopes":      []string{"mihomo.profile.read"},
	})

	_, err := verifier.Verify(raw, testTicketAudience)
	require.ErrorContains(t, err, "unsupported")
}

type fakeGrantVersionLookup struct {
	minimum    uint64
	bindingRef string
	consumer   string
}

type fakeAuthorizationStateLookup struct {
	state AuthorizationState
}

func (f fakeAuthorizationStateLookup) LookupAuthorizationState(context.Context, string, string) (AuthorizationState, error) {
	return f.state, nil
}

func (f *fakeGrantVersionLookup) MinimumVersion(_ context.Context, bindingRef string, consumer string) (uint64, error) {
	f.bindingRef = bindingRef
	f.consumer = consumer
	return f.minimum, nil
}

func testTicketVerifier() *TicketVerifier {
	return NewStaticKeyTicketVerifier(testTicketIssuer, testTicketKeyID, testTicketPrivateKey.Public().(ed25519.PublicKey))
}

func issueTestTicket(t *testing.T, claims map[string]any) string {
	t.Helper()

	typ := contractticket.TypeControl
	if claims["actor_type"] == "consumer" {
		typ = contractticket.TypeDelegation
	}
	return issueTestTicketWithHeaders(t, claims, map[string]any{"kid": testTicketKeyID, "typ": typ})
}

func issueTestTicketWithHeaders(t *testing.T, claims map[string]any, headers map[string]any) string {
	t.Helper()

	baseClaims := jwt.MapClaims{
		"iss": testTicketIssuer,
		"aud": []string{testTicketAudience},
		"iat": time.Now().Add(-time.Second).Unix(),
		"nbf": time.Now().Add(-time.Second).Unix(),
		"exp": time.Now().Add(time.Minute).Unix(),
		"jti": "test-ticket",
	}
	for key, value := range claims {
		baseClaims[key] = value
	}
	if baseClaims["actor_type"] == "consumer" {
		if consumer, ok := baseClaims["consumer"].(string); ok && consumer != "" {
			defaults := map[string]any{
				"consumer_principal":    consumer,
				"owner_user_ref":        "usr-1",
				"entry_identity_ref":    "entry-1",
				"owner_epoch":           float64(1),
				"consumer_epoch":        float64(1),
				"entry_epoch":           float64(1),
				"credential_generation": float64(1),
			}
			for key, value := range defaults {
				if _, exists := baseClaims[key]; !exists {
					baseClaims[key] = value
				}
			}
		}
	}
	if _, exists := baseClaims["sub"]; !exists {
		if consumer, ok := baseClaims["consumer"].(string); ok && consumer != "" {
			baseClaims["sub"] = "consumer:" + consumer
		} else {
			if _, exists := baseClaims["owner_user_ref"]; !exists {
				baseClaims["owner_user_ref"] = "usr-1"
			}
			baseClaims["sub"] = "user:usr-1"
		}
	}
	return issueTestTicketWithClaimsAndHeaders(t, baseClaims, headers)
}

func issueTestTicketWithClaimsAndHeaders(t *testing.T, claims map[string]any, headers map[string]any) string {
	t.Helper()

	token := jwt.NewWithClaims(contractticket.SigningMethodEd25519, jwt.MapClaims(claims))
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
		"sub": "consumer:paigram",
		"aud": []string{testTicketAudience},
		"iat": time.Now().Add(-time.Second).Unix(),
		"nbf": time.Now().Add(-time.Second).Unix(),
		"exp": time.Now().Add(time.Minute).Unix(),
		"jti": "test-ticket",
	}
	for key, value := range claims {
		baseClaims[key] = value
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, baseClaims)
	token.Header["kid"] = testTicketKeyID
	token.Header["typ"] = contractticket.TypeDelegation
	signed, err := token.SignedString([]byte("0123456789abcdef0123456789abcdef"))
	require.NoError(t, err)

	return signed
}

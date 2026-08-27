package auth

import (
	"testing"

	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"paigram/internal/config"
	"paigram/internal/model"
)

func TestResolveOAuthIdentityIssuerUsesVerifiedClaims(t *testing.T) {
	issuer, err := resolveOAuthIdentityIssuer("custom", config.OAuthProviderConfig{}, &oidcIDTokenClaims{
		RegisteredClaims: jwt.RegisteredClaims{Issuer: "https://issuer.example/tenant"},
	})
	require.NoError(t, err)
	assert.Equal(t, "https://issuer.example/tenant", issuer)
}

func TestResolveOAuthIdentityIssuerUsesCanonicalGitHubIssuer(t *testing.T) {
	issuer, err := resolveOAuthIdentityIssuer("github", config.OAuthProviderConfig{Issuer: "https://attacker.example"}, nil)
	require.NoError(t, err)
	assert.Equal(t, model.GitHubIdentityIssuer, issuer)
}

func TestResolveOAuthIdentityIssuerRejectsMissingOrInsecureIssuer(t *testing.T) {
	for _, issuer := range []string{"", "http://issuer.example", "https://user@issuer.example", "https://issuer.example?tenant=other"} {
		_, err := resolveOAuthIdentityIssuer("custom", config.OAuthProviderConfig{Issuer: issuer}, nil)
		require.ErrorIs(t, err, errOAuthIdentityIssuerInvalid)
	}
}

func TestResolveOAuthIdentityUsesVerifiedOIDCSubject(t *testing.T) {
	identity, err := resolveOAuthIdentity(
		"google",
		config.OAuthProviderConfig{},
		&oidcIDTokenClaims{RegisteredClaims: jwt.RegisteredClaims{
			Issuer:  model.GoogleIdentityIssuer,
			Subject: "stable-oidc-subject",
		}},
		&oauthUserInfo{ID: "provider-profile-id", Subject: "stable-oidc-subject"},
	)
	require.NoError(t, err)
	assert.Equal(t, model.GoogleIdentityIssuer, identity.Issuer)
	assert.Equal(t, "stable-oidc-subject", identity.Subject)
}

func TestResolveOAuthIdentityRejectsNonCanonicalBuiltInIssuer(t *testing.T) {
	_, err := resolveOAuthIdentity(
		"google",
		config.OAuthProviderConfig{Issuer: "https://attacker.example"},
		&oidcIDTokenClaims{RegisteredClaims: jwt.RegisteredClaims{
			Issuer:  "https://attacker.example",
			Subject: "subject",
		}},
		&oauthUserInfo{Subject: "subject"},
	)
	require.ErrorIs(t, err, errOAuthIdentityIssuerInvalid)
}

func TestResolveOAuthIdentityRejectsMismatchedUserInfoSubject(t *testing.T) {
	_, err := resolveOAuthIdentity(
		"google",
		config.OAuthProviderConfig{},
		&oidcIDTokenClaims{RegisteredClaims: jwt.RegisteredClaims{
			Issuer:  model.GoogleIdentityIssuer,
			Subject: "verified-subject",
		}},
		&oauthUserInfo{Subject: "different-subject"},
	)
	require.ErrorIs(t, err, errOAuthIdentityIssuerInvalid)
}

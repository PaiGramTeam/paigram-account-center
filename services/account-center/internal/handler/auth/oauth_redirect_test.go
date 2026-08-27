package auth

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"paigram/internal/config"
)

func TestResolveOAuthRedirectURIRequiresExactProviderAllowlistMatch(t *testing.T) {
	providerConfig := config.OAuthProviderConfig{
		RedirectURL:  "https://app.example.com/auth/callback/github",
		RedirectURLs: []string{"https://app.example.com/admin/auth/callback/github"},
	}

	redirectURI, err := resolveOAuthRedirectURI(
		"https://app.example.com/admin/auth/callback/github",
		providerConfig,
		"",
	)
	require.NoError(t, err)
	assert.Equal(t, "https://app.example.com/admin/auth/callback/github", redirectURI)

	_, err = resolveOAuthRedirectURI("https://attacker.example/callback", providerConfig, "")
	require.ErrorIs(t, err, errOAuthRedirectURIInvalid)
}

func TestResolveOAuthRedirectURIOnlyAllowsPlainHTTPOnLoopback(t *testing.T) {
	providerConfig := config.OAuthProviderConfig{RedirectURL: "http://localhost:8080/auth/callback/github"}

	redirectURI, err := resolveOAuthRedirectURI("", providerConfig, "")
	require.NoError(t, err)
	assert.Equal(t, providerConfig.RedirectURL, redirectURI)

	providerConfig.RedirectURL = "http://app.example.com/auth/callback/github"
	_, err = resolveOAuthRedirectURI("", providerConfig, "")
	require.ErrorIs(t, err, errOAuthRedirectURIInvalid)
}

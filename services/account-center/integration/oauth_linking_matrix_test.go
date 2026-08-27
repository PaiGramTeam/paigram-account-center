//go:build integration

package integration

import (
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"paigram/internal/config"
	"paigram/internal/model"
)

func TestOAuthExplicitLinkingMatrixResolvesSameUser(t *testing.T) {
	provider := newOAuthLinkingMatrixProvider(t)
	stack := newIntegrationStackWithConfig(t, func(cfg *config.Config) {
		cfg.Auth.OAuthStateTTLSeconds = 300
		cfg.RateLimit.Auth.OAuth = "20-M"
		cfg.Auth.AllowedOAuthProviders = []string{"github", "google", "telegram"}
		cfg.Auth.OAuthProviders = provider.configs()
	})

	userID, accessToken, _, _, _ := registerVerifyAndLogin(t, stack, "oauth-linking-matrix")
	for _, providerName := range []string{"github", "google", "telegram"} {
		boundUserID := completeOAuthJourney(t, stack, providerName, accessToken, true)
		require.Equal(t, userID, boundUserID)

		loginUserID := completeOAuthJourney(t, stack, providerName, "", false)
		require.Equal(t, userID, loginUserID)
	}

	var credentials []model.UserCredential
	require.NoError(t, stack.DB.Where("user_id = ?", userID).Order("issuer ASC").Find(&credentials).Error)
	require.Len(t, credentials, 4)
	identitySubjects := make(map[string]string, len(credentials))
	for _, credential := range credentials {
		identitySubjects[credential.Issuer] = credential.ProviderAccountID
	}
	assert.Equal(t, "424242", identitySubjects[model.GitHubIdentityIssuer])
	assert.Equal(t, "matrix-google-subject", identitySubjects[model.GoogleIdentityIssuer])
	assert.Equal(t, "matrix-telegram-subject", identitySubjects[model.TelegramIdentityIssuer])

	var userCount int64
	require.NoError(t, stack.DB.Model(&model.User{}).Count(&userCount).Error)
	assert.Equal(t, int64(1), userCount)
}

func TestOAuthBuiltInProvidersDoNotMergeSameEmailWithoutExplicitLink(t *testing.T) {
	for _, providerName := range []string{"github", "google", "telegram"} {
		t.Run(providerName, func(t *testing.T) {
			provider := newOAuthLinkingMatrixProvider(t)
			stack := newIntegrationStackWithConfig(t, func(cfg *config.Config) {
				cfg.Auth.OAuthStateTTLSeconds = 300
				cfg.RateLimit.Auth.OAuth = "20-M"
				cfg.Auth.AllowedOAuthProviders = []string{providerName}
				cfg.Auth.OAuthProviders = provider.configs()
			})

			firstUserID := completeOAuthJourney(t, stack, providerName, "", false)
			provider.useAlternateIdentity()
			secondUserID := completeOAuthJourney(t, stack, providerName, "", false)

			require.NotEqual(t, firstUserID, secondUserID)
			var userCount int64
			require.NoError(t, stack.DB.Model(&model.User{}).Count(&userCount).Error)
			assert.Equal(t, int64(2), userCount)
		})
	}
}

func completeOAuthJourney(
	t *testing.T,
	stack *integrationStack,
	providerName, accessToken string,
	bind bool,
) uint64 {
	t.Helper()
	callbackURI := "https://app.example.com/auth/callback/" + providerName
	method := http.MethodPost
	path := "/api/v1/auth/oauth/" + providerName + "/init"
	headers := map[string]string(nil)
	if bind {
		method = http.MethodPut
		path = "/api/v1/me/login-methods/" + providerName
		headers = authHeaders(accessToken)
	}
	initResponse := performJSONRequest(t, stack.Router, method, path, map[string]any{
		"redirect_to": callbackURI,
	}, headers)
	require.Equal(t, http.StatusOK, initResponse.Code, initResponse.Body.String())
	initData := decodeResponseData(t, initResponse)
	authorizationURL, _ := initData["auth_url"].(string)
	require.NotEmpty(t, authorizationURL)
	parsedAuthorizationURL, err := url.Parse(authorizationURL)
	require.NoError(t, err)
	require.Equal(t, callbackURI, parsedAuthorizationURL.Query().Get("redirect_uri"))
	state := parsedAuthorizationURL.Query().Get("state")
	nonce := parsedAuthorizationURL.Query().Get("nonce")
	require.NotEmpty(t, state)
	require.NotEmpty(t, nonce)

	callbackResponse := performJSONRequest(t, stack.Router, http.MethodPost, "/api/v1/auth/oauth/"+providerName+"/callback", map[string]any{
		"state": state,
		"code":  nonce,
	}, headers)
	require.Equal(t, http.StatusOK, callbackResponse.Code, callbackResponse.Body.String())
	callbackData := decodeResponseData(t, callbackResponse)
	callbackUserID, ok := callbackData["user_id"].(float64)
	require.True(t, ok)
	require.Positive(t, callbackUserID)
	if bind {
		assert.Equal(t, true, callbackData["bound"])
	}
	return uint64(callbackUserID)
}

type oauthLinkingMatrixProvider struct {
	server     *httptest.Server
	privateKey *rsa.PrivateKey
	clientID   string
	kid        string
	mu         sync.RWMutex
	alternate  bool
}

func newOAuthLinkingMatrixProvider(t *testing.T) *oauthLinkingMatrixProvider {
	t.Helper()
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	provider := &oauthLinkingMatrixProvider{
		privateKey: privateKey,
		clientID:   "oauth-linking-matrix-client",
		kid:        "oauth-linking-matrix-key",
	}
	provider.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		provider.serve(t, w, request)
	}))
	t.Cleanup(provider.server.Close)
	return provider
}

func (provider *oauthLinkingMatrixProvider) configs() map[string]config.OAuthProviderConfig {
	configs := make(map[string]config.OAuthProviderConfig, 3)
	for _, providerName := range []string{"github", "google", "telegram"} {
		configs[providerName] = config.OAuthProviderConfig{
			ClientID:     provider.clientID,
			ClientSecret: "oauth-linking-matrix-secret",
			RedirectURL:  "https://app.example.com/auth/callback/" + providerName,
			AuthURL:      provider.server.URL + "/authorize/" + providerName,
			TokenURL:     provider.server.URL + "/token/" + providerName,
			UserInfoURL:  provider.server.URL + "/userinfo/" + providerName,
			Scopes:       []string{"user:email"},
		}
	}
	google := configs["google"]
	google.Issuer = "https://attacker.example"
	google.JWKSURL = provider.server.URL + "/jwks"
	google.Scopes = []string{"openid", "profile", "email"}
	configs["google"] = google
	telegram := configs["telegram"]
	telegram.Issuer = "https://attacker.example"
	telegram.JWKSURL = provider.server.URL + "/jwks"
	telegram.Scopes = []string{"openid", "profile"}
	configs["telegram"] = telegram
	return configs
}

func (provider *oauthLinkingMatrixProvider) useAlternateIdentity() {
	provider.mu.Lock()
	provider.alternate = true
	provider.mu.Unlock()
}

func (provider *oauthLinkingMatrixProvider) identitySubject(providerName string) string {
	provider.mu.RLock()
	alternate := provider.alternate
	provider.mu.RUnlock()
	if !alternate {
		switch providerName {
		case "github":
			return "424242"
		case "google":
			return "matrix-google-subject"
		default:
			return "matrix-telegram-subject"
		}
	}
	switch providerName {
	case "github":
		return "434343"
	case "google":
		return "unlinked-google-subject"
	default:
		return "unlinked-telegram-subject"
	}
}

func (provider *oauthLinkingMatrixProvider) serve(t *testing.T, writer http.ResponseWriter, request *http.Request) {
	t.Helper()
	switch request.URL.Path {
	case "/jwks":
		require.NoError(t, json.NewEncoder(writer).Encode(map[string]any{
			"keys": []map[string]any{{
				"kty": "RSA",
				"kid": provider.kid,
				"alg": "RS256",
				"use": "sig",
				"n":   base64.RawURLEncoding.EncodeToString(provider.privateKey.PublicKey.N.Bytes()),
				"e":   base64.RawURLEncoding.EncodeToString(big.NewInt(int64(provider.privateKey.PublicKey.E)).Bytes()),
			}},
		}))
		return
	case "/userinfo/github":
		githubID, ok := new(big.Int).SetString(provider.identitySubject("github"), 10)
		require.True(t, ok)
		require.NoError(t, json.NewEncoder(writer).Encode(map[string]any{
			"id": githubID.Uint64(), "email": "oauth-linking-matrix@example.com", "email_verified": true, "login": "matrix",
		}))
		return
	case "/userinfo/google":
		require.NoError(t, json.NewEncoder(writer).Encode(map[string]any{
			"id": 987654, "sub": provider.identitySubject("google"), "email": "oauth-linking-matrix@example.com",
			"email_verified": true, "name": "Matrix User",
		}))
		return
	}

	providerName := request.URL.Path[len("/token/"):]
	require.Contains(t, []string{"github", "google", "telegram"}, providerName)
	require.NoError(t, request.ParseForm())
	assert.Equal(t, "https://app.example.com/auth/callback/"+providerName, request.Form.Get("redirect_uri"))
	nonce := request.Form.Get("code")
	require.NotEmpty(t, nonce)
	response := map[string]any{
		"access_token": "access-token-" + providerName,
		"token_type":   "Bearer",
		"expires_in":   3600,
		"scope":        "openid profile email",
	}
	if providerName == "google" {
		response["id_token"] = provider.signIDToken(t, model.GoogleIdentityIssuer, provider.identitySubject("google"), nonce)
	}
	if providerName == "telegram" {
		response["id_token"] = provider.signIDToken(t, model.TelegramIdentityIssuer, provider.identitySubject("telegram"), nonce)
	}
	require.NoError(t, json.NewEncoder(writer).Encode(response))
}

func (provider *oauthLinkingMatrixProvider) signIDToken(t *testing.T, issuer, subject, nonce string) string {
	t.Helper()
	now := time.Now().UTC()
	token := jwt.NewWithClaims(jwt.SigningMethodRS256, jwt.MapClaims{
		"iss": issuer, "sub": subject, "aud": provider.clientID, "iat": now.Unix(), "exp": now.Add(time.Hour).Unix(),
		"nonce": nonce, "name": "Matrix User", "preferred_username": "matrix",
	})
	token.Header["kid"] = provider.kid
	signed, err := token.SignedString(provider.privateKey)
	require.NoError(t, err)
	return signed
}

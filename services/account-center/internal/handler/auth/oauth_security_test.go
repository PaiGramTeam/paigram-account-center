package auth

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"paigram/internal/config"
	"paigram/internal/model"
)

func TestOAuthCallbackDoesNotExposeProviderErrorBody(t *testing.T) {
	db := setupTestDB(t)
	ensureUserOAuthStatesTable(t, db)
	handler := setupOAuthTestHandler(t, db)
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, err := w.Write([]byte(`{"error":"invalid_grant","client_secret":"must-not-leak"}`))
		require.NoError(t, err)
	}))
	t.Cleanup(provider.Close)
	handler.cfg.AllowedOAuthProviders = []string{"custom"}
	handler.cfg.OAuthProviders = map[string]config.OAuthProviderConfig{
		"custom": {
			ClientID:     "client-id",
			ClientSecret: "client-secret",
			TokenURL:     provider.URL,
			Issuer:       "https://issuer.example",
		},
	}
	require.NoError(t, db.Create(&model.UserOAuthState{
		Provider: "custom", State: "safe-provider-error", Purpose: string(model.OAuthPurposeLogin),
		ClientIP: testHTTPRequestClientIP, UserAgent: testHTTPRequestUserAgent,
		ExpiresAt: time.Now().UTC().Add(time.Minute),
	}).Error)

	responseRecorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(responseRecorder)
	context.Request = httptest.NewRequest(http.MethodPost, "/api/v1/auth/oauth/custom/callback", bytes.NewBufferString(`{"state":"safe-provider-error","code":"bad-code"}`))
	context.Request.Header.Set("Content-Type", "application/json")
	context.Params = gin.Params{{Key: "provider", Value: "custom"}}

	handler.HandleOAuthCallback(context)

	require.Equal(t, http.StatusBadRequest, responseRecorder.Code)
	assert.Equal(t, "OAUTH_TOKEN_EXCHANGE_FAILED", decodeOAuthErrorCode(t, responseRecorder))
	assert.NotContains(t, responseRecorder.Body.String(), "must-not-leak")
	assert.NotContains(t, responseRecorder.Body.String(), "client-secret")
}

func TestReadOAuthProviderResponseRejectsOversizedBody(t *testing.T) {
	_, err := readOAuthProviderResponse(strings.NewReader(strings.Repeat("x", maxOAuthProviderResponseBytes+1)))
	require.ErrorIs(t, err, errOAuthProviderResponseTooLarge)
}

func TestOAuthUserInfoDecodesNumericGitHubID(t *testing.T) {
	var userInfo oauthUserInfo
	require.NoError(t, json.Unmarshal([]byte(`{"id":123456789,"login":"octocat"}`), &userInfo))
	assert.Equal(t, "123456789", userInfo.ID)
}

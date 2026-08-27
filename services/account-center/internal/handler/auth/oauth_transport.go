package auth

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"paigram/internal/config"
	"paigram/internal/handler/shared"
	"paigram/internal/model"
)

const maxOAuthProviderResponseBytes = 1 << 20

var errOAuthProviderResponseTooLarge = errors.New("oauth provider response exceeds size limit")

type oauthTokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int    `json:"expires_in"`
	Scope        string `json:"scope"`
	IDToken      string `json:"id_token,omitempty"`
}

type oauthUserInfo struct {
	Issuer        string `json:"-"`
	ID            string `json:"-"`
	Subject       string `json:"sub"`
	Email         string `json:"email"`
	EmailVerified bool   `json:"email_verified"`
	Name          string `json:"name"`
	GivenName     string `json:"given_name"`
	FamilyName    string `json:"family_name"`
	Picture       string `json:"picture"`
	Login         string `json:"login"`
	AvatarURL     string `json:"avatar_url"`
}

type oidcIDTokenClaims struct {
	jwt.RegisteredClaims
	Nonce             string `json:"nonce,omitempty"`
	Name              string `json:"name,omitempty"`
	PreferredUsername string `json:"preferred_username,omitempty"`
	Picture           string `json:"picture,omitempty"`
	PhoneNumber       string `json:"phone_number,omitempty"`
}

func readOAuthProviderResponse(reader io.Reader) ([]byte, error) {
	limited, err := io.ReadAll(io.LimitReader(reader, maxOAuthProviderResponseBytes+1))
	if err != nil {
		return nil, err
	}
	if len(limited) > maxOAuthProviderResponseBytes {
		return nil, fmt.Errorf("%w: maximum %d bytes", errOAuthProviderResponseTooLarge, maxOAuthProviderResponseBytes)
	}
	return limited, nil
}

func (info *oauthUserInfo) UnmarshalJSON(data []byte) error {
	type oauthUserInfoAlias oauthUserInfo
	decoded := struct {
		*oauthUserInfoAlias
		ID json.RawMessage `json:"id"`
	}{oauthUserInfoAlias: (*oauthUserInfoAlias)(info)}
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}

	rawID := bytes.TrimSpace(decoded.ID)
	if len(rawID) == 0 || bytes.Equal(rawID, []byte("null")) {
		return nil
	}
	if rawID[0] == '"' {
		return json.Unmarshal(rawID, &info.ID)
	}
	numericID, err := strconv.ParseUint(string(rawID), 10, 64)
	if err != nil {
		return fmt.Errorf("decode provider account id: %w", err)
	}
	info.ID = strconv.FormatUint(numericID, 10)
	return nil
}

func (h *Handler) exchangeCodeForToken(
	ctx context.Context,
	provider, code, codeVerifier, redirectURI string,
	cfg config.OAuthProviderConfig,
) (*oauthTokenResponse, error) {
	if cfg.TokenURL == "" {
		return nil, fmt.Errorf("token_url not configured for provider %s", provider)
	}

	data := url.Values{
		"grant_type": []string{"authorization_code"},
		"code":       []string{code},
		"client_id":  []string{cfg.ClientID},
	}
	if !strings.EqualFold(provider, "telegram") {
		data.Set("client_secret", cfg.ClientSecret)
	}
	if redirectURI != "" {
		data.Set("redirect_uri", redirectURI)
	}
	if codeVerifier != "" {
		data.Set("code_verifier", codeVerifier)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, cfg.TokenURL, strings.NewReader(data.Encode()))
	if err != nil {
		return nil, fmt.Errorf("create token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	if strings.EqualFold(provider, "telegram") && cfg.ClientID != "" && cfg.ClientSecret != "" {
		credentials := base64.StdEncoding.EncodeToString([]byte(cfg.ClientID + ":" + cfg.ClientSecret))
		req.Header.Set("Authorization", "Basic "+credentials)
	}

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("execute token request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("token exchange failed with status %d", resp.StatusCode)
	}
	body, err := readOAuthProviderResponse(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read token response: %w", err)
	}

	var tokenResp oauthTokenResponse
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return nil, fmt.Errorf("parse token response: %w", err)
	}
	if tokenResp.AccessToken == "" {
		return nil, errors.New("no access_token in response")
	}
	return &tokenResp, nil
}

func (h *Handler) fetchUserInfo(ctx context.Context, provider, accessToken string, cfg config.OAuthProviderConfig, idTokenClaims *oidcIDTokenClaims) (*oauthUserInfo, error) {
	if strings.EqualFold(provider, "telegram") {
		return oauthUserInfoFromTelegramClaims(idTokenClaims)
	}
	if cfg.UserInfoURL == "" {
		return nil, fmt.Errorf("user_info_url not configured for provider %s", provider)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, cfg.UserInfoURL, nil)
	if err != nil {
		return nil, fmt.Errorf("create userinfo request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Accept", "application/json")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("execute userinfo request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("userinfo request failed with status %d", resp.StatusCode)
	}
	body, err := readOAuthProviderResponse(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read userinfo response: %w", err)
	}

	var userInfo oauthUserInfo
	if err := json.Unmarshal(body, &userInfo); err != nil {
		return nil, fmt.Errorf("parse userinfo response: %w", err)
	}
	if strings.EqualFold(provider, "github") {
		if userInfo.Name == "" {
			userInfo.Name = userInfo.Login
		}
		if userInfo.Picture == "" {
			userInfo.Picture = userInfo.AvatarURL
		}
	}
	if userInfo.ID == "" && userInfo.Subject == "" {
		return nil, errors.New("provider did not return a subject")
	}
	return &userInfo, nil
}

func oauthUserInfoFromTelegramClaims(claims *oidcIDTokenClaims) (*oauthUserInfo, error) {
	if claims == nil {
		return nil, errors.New("missing telegram id token claims")
	}
	if claims.Subject == "" {
		return nil, errors.New("telegram id token missing subject")
	}

	userInfo := &oauthUserInfo{
		ID:      claims.Subject,
		Name:    strings.TrimSpace(claims.Name),
		Picture: strings.TrimSpace(claims.Picture),
		Login:   strings.TrimSpace(claims.PreferredUsername),
	}
	if userInfo.Name == "" {
		userInfo.Name = userInfo.Login
	}
	return userInfo, nil
}

func (h *Handler) refreshOAuthToken(ctx context.Context, credential *model.UserCredential, cfg config.OAuthProviderConfig) error {
	if cfg.TokenURL == "" {
		return fmt.Errorf("token_url not configured for provider %s", credential.Provider)
	}

	refreshToken, err := credential.GetRefreshToken()
	if err != nil {
		return fmt.Errorf("decrypt refresh token: %w", err)
	}
	if refreshToken == "" {
		return errors.New("no refresh token available")
	}

	data := url.Values{
		"grant_type":    []string{"refresh_token"},
		"refresh_token": []string{refreshToken},
		"client_id":     []string{cfg.ClientID},
		"client_secret": []string{cfg.ClientSecret},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, cfg.TokenURL, strings.NewReader(data.Encode()))
	if err != nil {
		return fmt.Errorf("create refresh request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("execute refresh request: %w", err)
	}
	defer resp.Body.Close()

	body, err := readOAuthProviderResponse(resp.Body)
	if err != nil {
		return fmt.Errorf("read refresh response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("token refresh failed with status %d", resp.StatusCode)
	}

	var tokenResp oauthTokenResponse
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return fmt.Errorf("parse refresh response: %w", err)
	}
	if tokenResp.AccessToken == "" {
		return errors.New("no access_token in refresh response")
	}

	now := time.Now().UTC()
	if err := credential.SetAccessToken(tokenResp.AccessToken); err != nil {
		return fmt.Errorf("encrypt new access token: %w", err)
	}
	if tokenResp.RefreshToken != "" {
		if err := credential.SetRefreshToken(tokenResp.RefreshToken); err != nil {
			return fmt.Errorf("encrypt new refresh token: %w", err)
		}
	}
	if tokenResp.ExpiresIn > 0 {
		credential.TokenExpiry = shared.MakeNullTime(now.Add(time.Duration(tokenResp.ExpiresIn) * time.Second))
	} else {
		credential.TokenExpiry = shared.ClearNullTime()
	}
	credential.LastSyncAt = shared.MakeNullTime(now)

	if err := h.db.Save(credential).Error; err != nil {
		return fmt.Errorf("save refreshed credential: %w", err)
	}
	return nil
}

func (h *Handler) RefreshOAuthTokenPublic(ctx context.Context, credential *model.UserCredential, cfg config.OAuthProviderConfig) error {
	return h.refreshOAuthToken(ctx, credential, cfg)
}

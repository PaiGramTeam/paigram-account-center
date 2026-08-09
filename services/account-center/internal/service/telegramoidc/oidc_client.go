package telegramoidc

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"go.uber.org/zap"
)

// ErrTokenExchangeFailed: 5xx / network. Retry-eligible.
// ErrTokenExchangeRejected: 4xx with body. Permanent (bad code, bad client).
// ErrIDTokenInvalid: signature / claims verification failed.
// ErrTelegramIDMissing: id_token verified but no `id` claim.
var (
	ErrTokenExchangeFailed   = errors.New("telegramoidc: token exchange failed")
	ErrTokenExchangeRejected = errors.New("telegramoidc: token exchange rejected")
	ErrIDTokenInvalid        = errors.New("telegramoidc: id_token invalid")
	ErrTelegramIDMissing     = errors.New("telegramoidc: id_token missing telegram id")
)

// TokenResponse mirrors the relevant subset of oauth.telegram.org/token JSON.
type TokenResponse struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	ExpiresIn   int    `json:"expires_in"`
	IDToken     string `json:"id_token"`
}

// Client orchestrates the OIDC Authorization Code + PKCE flow against
// oauth.telegram.org. Stateless beyond config + JWKS cache.
type Client struct {
	cfg        Config
	httpClient *http.Client
	jwks       *JWKSCache
	logger     *zap.Logger
}

func NewClient(cfg Config, logger *zap.Logger) *Client {
	cfg.applyDefaults()
	httpClient := &http.Client{Timeout: 10 * time.Second}
	return &Client{
		cfg:        cfg,
		httpClient: httpClient,
		jwks:       NewJWKSCache(cfg.JWKSEndpoint, httpClient, logger),
		logger:     logger,
	}
}

// AuthorizeURL builds the redirect URL the user's browser will follow to
// reach Telegram's consent screen.
func (c *Client) AuthorizeURL(state, codeChallenge string) string {
	q := url.Values{}
	q.Set("client_id", c.cfg.ClientID)
	q.Set("redirect_uri", c.cfg.RedirectURI)
	q.Set("response_type", "code")
	q.Set("scope", "openid profile")
	q.Set("state", state)
	q.Set("code_challenge", codeChallenge)
	q.Set("code_challenge_method", "S256")
	return c.cfg.AuthorizeEndpoint + "?" + q.Encode()
}

// ExchangeCode swaps the authorization code + PKCE verifier for tokens.
func (c *Client) ExchangeCode(ctx context.Context, code, codeVerifier string) (*TokenResponse, error) {
	body := url.Values{}
	body.Set("grant_type", "authorization_code")
	body.Set("code", code)
	body.Set("redirect_uri", c.cfg.RedirectURI)
	body.Set("code_verifier", codeVerifier)

	req, err := http.NewRequestWithContext(
		ctx, http.MethodPost, c.cfg.TokenEndpoint, strings.NewReader(body.Encode()),
	)
	if err != nil {
		return nil, fmt.Errorf("%w: build request: %v", ErrTokenExchangeFailed, err)
	}
	req.SetBasicAuth(c.cfg.ClientID, c.cfg.ClientSecret)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrTokenExchangeFailed, err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 500 {
		c.logger.Warn("token endpoint 5xx",
			zap.Int("status", resp.StatusCode), zap.ByteString("body_head", trim(respBody, 200)))
		return nil, fmt.Errorf("%w: status %d", ErrTokenExchangeFailed, resp.StatusCode)
	}
	if resp.StatusCode >= 400 {
		c.logger.Warn("token endpoint 4xx",
			zap.Int("status", resp.StatusCode), zap.ByteString("body_head", trim(respBody, 200)))
		return nil, fmt.Errorf("%w: status %d", ErrTokenExchangeRejected, resp.StatusCode)
	}

	var tr TokenResponse
	if err := json.Unmarshal(respBody, &tr); err != nil {
		return nil, fmt.Errorf("%w: unmarshal: %v", ErrTokenExchangeFailed, err)
	}
	return &tr, nil
}

// VerifyIDToken validates the RS256 JWT against the JWKS, enforces iss / aud
// / exp, and returns the decoded claims. Never logs the raw id_token.
func (c *Client) VerifyIDToken(ctx context.Context, idToken string) (*Claims, error) {
	parsed, err := jwt.Parse(idToken, func(t *jwt.Token) (any, error) {
		if t.Method.Alg() != "RS256" {
			return nil, fmt.Errorf("unexpected alg %q", t.Method.Alg())
		}
		kid, _ := t.Header["kid"].(string)
		if kid == "" {
			return nil, errors.New("missing kid header")
		}
		return c.jwks.Get(ctx, kid)
	})
	if err != nil {
		if errors.Is(err, ErrJWKSUnavailable) {
			return nil, err
		}
		return nil, fmt.Errorf("%w: %v", ErrIDTokenInvalid, err)
	}
	if !parsed.Valid {
		return nil, ErrIDTokenInvalid
	}

	mapClaims, ok := parsed.Claims.(jwt.MapClaims)
	if !ok {
		return nil, ErrIDTokenInvalid
	}
	raw, err := json.Marshal(mapClaims)
	if err != nil {
		return nil, fmt.Errorf("%w: marshal claims: %v", ErrIDTokenInvalid, err)
	}
	var claims Claims
	if err := json.Unmarshal(raw, &claims); err != nil {
		return nil, fmt.Errorf("%w: unmarshal claims: %v", ErrIDTokenInvalid, err)
	}

	if claims.Iss != c.cfg.ExpectedIssuer {
		return nil, fmt.Errorf("%w: iss %s", ErrIDTokenInvalid, claims.Iss)
	}
	if claims.Aud != c.cfg.ClientID {
		return nil, fmt.Errorf("%w: aud %s", ErrIDTokenInvalid, claims.Aud)
	}
	if claims.Exp < time.Now().Unix() {
		return nil, fmt.Errorf("%w: expired", ErrIDTokenInvalid)
	}
	if claims.ID == 0 {
		return nil, ErrTelegramIDMissing
	}
	return &claims, nil
}

func trim(b []byte, n int) []byte {
	if len(b) <= n {
		return b
	}
	return b[:n]
}

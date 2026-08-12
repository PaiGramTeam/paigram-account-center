package credentials

import (
	"fmt"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

// Token issuer / token-type defaults for the OAuth 2.0 client_credentials
// grant. All access tokens carry these unless overridden by config.
const (
	defaultOAuthIssuer           = "account-center"
	defaultAccessTokenTTLSeconds = 3600
	defaultExpectedAudience      = "account-center"
	bearerTokenType              = "Bearer"
	minSigningKeyBytes           = 32
)

// TokenServiceConfig configures the HS256 OAuth access-token issuer and verifier.
// Service tickets use a separate Ed25519 key pair and token profile.
type TokenServiceConfig struct {
	Issuer                string
	AccessTokenTTLSeconds int
	SigningKey            []byte
}

// TokenService issues and validates HS256 OAuth 2.0 access tokens against
// the credentials registry (model.ServiceCredential rows).
type TokenService struct {
	credentials *Service
	cfg         TokenServiceConfig
}

// IssueClientCredentialsInput is the form-decoded /oauth/token request
// after handler-side validation. RequestedScopes is the optional
// space-delimited "scope" form field per RFC 6749 §3.3.
type IssueClientCredentialsInput struct {
	ClientID        string
	ClientSecret    string
	Audience        string
	RequestedScopes []string
}

// IssuedToken is the RFC 6749 §5.1 token response body. Marshalled
// directly to JSON by the /oauth/token handler.
type IssuedToken struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	ExpiresIn   int64  `json:"expires_in"`
	Scope       string `json:"scope"`
}

// AccessClaims is the on-the-wire HS256 JWT payload. Scope uses the
// space-delimited OAuth representation defined by RFC 6749.
type AccessClaims struct {
	ClientID string `json:"client_id"`
	BotID    string `json:"bot_id"`
	Scope    string `json:"scope"`
	jwt.RegisteredClaims
}

// ScopeList returns the access token's scopes split on whitespace, per
// RFC 6749 §3.3.
func (c *AccessClaims) ScopeList() []string {
	if c == nil {
		return nil
	}
	return splitScope(c.Scope)
}

// NewTokenService wires the issuer/verifier against a credentials service
// and an HS256 signing key. The signing key MUST be ≥ 32 bytes; this
// constructor rejects a shorter key rather than silently weakening the
// access-token signature.
func NewTokenService(credentials *Service, cfg TokenServiceConfig) (*TokenService, error) {
	if len(cfg.SigningKey) < minSigningKeyBytes {
		return nil, fmt.Errorf(
			"credentials: HS256 signing key must be at least %d bytes: got %d",
			minSigningKeyBytes, len(cfg.SigningKey),
		)
	}
	if cfg.Issuer == "" {
		cfg.Issuer = defaultOAuthIssuer
	}
	if cfg.AccessTokenTTLSeconds <= 0 {
		cfg.AccessTokenTTLSeconds = defaultAccessTokenTTLSeconds
	}
	return &TokenService{credentials: credentials, cfg: cfg}, nil
}

// IssueClientCredentials runs the RFC 6749 §4.4 client_credentials grant:
//   - verify (client_id, client_secret) against service_credentials;
//   - verify the requested audience is in the credential's audience list;
//   - intersect requested scopes with granted scopes;
//   - mint an HS256 JWT of TTL `cfg.AccessTokenTTLSeconds` (default 1 h).
//
// No token row is written. On success last_used_at is best-effort updated.
func (s *TokenService) IssueClientCredentials(input IssueClientCredentialsInput) (*IssuedToken, error) {
	if input.ClientID == "" {
		return nil, ErrEmptyClientID
	}
	credential, err := s.credentials.VerifySecret(input.ClientID, input.ClientSecret)
	if err != nil {
		return nil, err
	}

	audiences, err := DecodeStringList(credential.Audiences)
	if err != nil {
		return nil, err
	}
	if !contains(audiences, input.Audience) {
		return nil, ErrInvalidAudience
	}

	grantedScopes, err := DecodeStringList(credential.Scopes)
	if err != nil {
		return nil, err
	}
	resolvedScopes, err := resolveRequestedScopes(grantedScopes, input.RequestedScopes)
	if err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	expiresAt := now.Add(time.Duration(s.cfg.AccessTokenTTLSeconds) * time.Second)
	scopeStr := strings.Join(resolvedScopes, " ")
	claims := AccessClaims{
		ClientID: credential.ClientID,
		BotID:    credential.BotID,
		Scope:    scopeStr,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    s.cfg.Issuer,
			Subject:   credential.ClientID,
			Audience:  jwt.ClaimStrings{input.Audience},
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(expiresAt),
			ID:        uuid.NewString(),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString(s.cfg.SigningKey)
	if err != nil {
		return nil, err
	}

	// Best-effort touch of last_used_at; do not fail token issue on update error.
	_ = s.credentials.MarkUsed(credential.ClientID, now)

	return &IssuedToken{
		AccessToken: signed,
		TokenType:   bearerTokenType,
		ExpiresIn:   int64(s.cfg.AccessTokenTTLSeconds),
		Scope:       scopeStr,
	}, nil
}

// ValidateAccessToken verifies the HS256 signature, the issuer (must equal
// cfg.Issuer), the audience (must equal expectedAudience), and the
// expiry. It then confirms the credential row is still active in the DB,
// so disabling a credential immediately rejects its previously issued tokens.
func (s *TokenService) ValidateAccessToken(raw, expectedAudience string) (*AccessClaims, error) {
	if expectedAudience == "" {
		expectedAudience = defaultExpectedAudience
	}
	claims := &AccessClaims{}
	token, err := jwt.ParseWithClaims(raw, claims, func(token *jwt.Token) (any, error) {
		if token.Method != jwt.SigningMethodHS256 {
			return nil, jwt.ErrTokenSignatureInvalid
		}
		return s.cfg.SigningKey, nil
	},
		jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}),
		jwt.WithIssuer(s.cfg.Issuer),
		jwt.WithAudience(expectedAudience),
		jwt.WithExpirationRequired(),
	)
	if err != nil {
		return nil, err
	}
	if !token.Valid {
		return nil, jwt.ErrTokenInvalidClaims
	}

	credential, err := s.credentials.GetByClientID(claims.ClientID)
	if err != nil {
		return nil, err
	}
	// Defense-in-depth: the JWT's sub must match the credential row.
	if credential.ClientID != claims.Subject || credential.BotID != claims.BotID {
		return nil, jwt.ErrTokenInvalidClaims
	}

	return claims, nil
}

// HasScope returns true iff the claims carry every required scope. The
// special "admin.all" scope grants any scope (parity with the legacy
// CheckScope helper in interceptor/auth.go).
func HasScope(claims *AccessClaims, requiredScopes ...string) bool {
	if claims == nil {
		return false
	}
	if len(requiredScopes) == 0 {
		return true
	}
	available := splitScope(claims.Scope)
	have := make(map[string]struct{}, len(available))
	for _, scope := range available {
		have[scope] = struct{}{}
	}
	if _, ok := have["admin.all"]; ok {
		return true
	}
	for _, required := range requiredScopes {
		if _, ok := have[required]; !ok {
			return false
		}
	}
	return true
}

// resolveRequestedScopes intersects the operator-granted scopes (from the
// credential row) with the optional caller-requested scopes (from the
// /oauth/token "scope" form field). Per RFC 6749 §3.3:
//   - if the caller requests no scopes, the granted scope set is used;
//   - if the caller requests any scope NOT in the granted set, return
//     ErrInsufficientScope.
func resolveRequestedScopes(granted, requested []string) ([]string, error) {
	if len(requested) == 0 {
		return granted, nil
	}
	have := make(map[string]struct{}, len(granted))
	for _, scope := range granted {
		have[scope] = struct{}{}
	}
	for _, scope := range requested {
		if _, ok := have[scope]; !ok {
			return nil, ErrInsufficientScope
		}
	}
	return requested, nil
}

// SplitScope parses an RFC 6749 §3.3 space-delimited scope string into a
// trimmed slice, dropping empty fragments produced by repeated spaces.
// Exported because handler code and tests need the same parser.
func SplitScope(raw string) []string {
	return splitScope(raw)
}

func splitScope(raw string) []string {
	if raw == "" {
		return nil
	}
	fields := strings.Fields(raw)
	if len(fields) == 0 {
		return nil
	}
	return fields
}

func contains(values []string, expected string) bool {
	for _, v := range values {
		if v == expected {
			return true
		}
	}
	return false
}

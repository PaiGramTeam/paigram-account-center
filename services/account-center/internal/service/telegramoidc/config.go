package telegramoidc

import (
	"errors"
	"net/url"
	"strings"
)

// Config carries the BotFather-issued OIDC credentials. Bot token is NOT a
// substitute for ClientSecret — Telegram OIDC issues a separate secret.
type Config struct {
	ClientID     string
	ClientSecret string
	RedirectURI  string

	// AuthorizeEndpoint defaults to https://oauth.telegram.org/auth.
	// Tests inject a httptest.Server URL here.
	AuthorizeEndpoint string
	// TokenEndpoint defaults to https://oauth.telegram.org/token.
	TokenEndpoint string
	// JWKSEndpoint defaults to https://oauth.telegram.org/.well-known/jwks.json.
	JWKSEndpoint string
	// ExpectedIssuer defaults to https://oauth.telegram.org.
	ExpectedIssuer string
}

const (
	defaultAuthorizeEndpoint = "https://oauth.telegram.org/auth"
	defaultTokenEndpoint     = "https://oauth.telegram.org/token"
	defaultJWKSEndpoint      = "https://oauth.telegram.org/.well-known/jwks.json"
	defaultIssuer            = "https://oauth.telegram.org"
)

// Validate enforces boot-time configuration invariants. Returns the first
// error encountered.
func (c *Config) Validate() error {
	if strings.TrimSpace(c.ClientID) == "" {
		return errors.New("telegramoidc: ClientID required")
	}
	if strings.TrimSpace(c.ClientSecret) == "" {
		return errors.New("telegramoidc: ClientSecret required")
	}
	u, err := url.Parse(c.RedirectURI)
	if err != nil || u.Scheme != "https" {
		return errors.New("telegramoidc: RedirectURI must be a valid https URL")
	}
	return nil
}

func (c *Config) applyDefaults() {
	if c.AuthorizeEndpoint == "" {
		c.AuthorizeEndpoint = defaultAuthorizeEndpoint
	}
	if c.TokenEndpoint == "" {
		c.TokenEndpoint = defaultTokenEndpoint
	}
	if c.JWKSEndpoint == "" {
		c.JWKSEndpoint = defaultJWKSEndpoint
	}
	if c.ExpectedIssuer == "" {
		c.ExpectedIssuer = defaultIssuer
	}
}

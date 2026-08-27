package auth

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"

	"paigram/internal/config"
	"paigram/internal/model"
	"paigram/internal/oidc"
)

var knownOIDCProviderDefaults = map[string]struct {
	Issuer  string
	JWKSURL string
}{
	"google": {
		Issuer:  model.GoogleIdentityIssuer,
		JWKSURL: "https://www.googleapis.com/oauth2/v3/certs",
	},
	"telegram": {
		Issuer:  model.TelegramIdentityIssuer,
		JWKSURL: "https://oauth.telegram.org/.well-known/jwks.json",
	},
}

type oidcVerifierCache struct {
	mu        sync.Mutex
	verifiers map[string]*oidc.Verifier
}

func newOIDCVerifierCache() *oidcVerifierCache {
	return &oidcVerifierCache{verifiers: map[string]*oidc.Verifier{}}
}

func (c *oidcVerifierCache) verifierFor(provider string, providerCfg config.OAuthProviderConfig) (*oidc.Verifier, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if verifier, ok := c.verifiers[provider]; ok {
		return verifier, nil
	}

	issuer := strings.TrimSpace(providerCfg.Issuer)
	jwksURL := strings.TrimSpace(providerCfg.JWKSURL)
	if defaults, ok := knownOIDCProviderDefaults[strings.ToLower(provider)]; ok {
		issuer = defaults.Issuer
		if jwksURL == "" {
			jwksURL = defaults.JWKSURL
		}
	}
	audience := strings.TrimSpace(providerCfg.ClientID)

	if issuer == "" || jwksURL == "" || audience == "" {
		return nil, fmt.Errorf(
			"oidc verifier not configured for provider %q: issuer=%q jwks_url=%q client_id_set=%t",
			provider, issuer, jwksURL, audience != "",
		)
	}

	verifier, err := oidc.NewVerifier(oidc.Config{
		Issuer:   issuer,
		Audience: audience,
		JWKSURL:  jwksURL,
	})
	if err != nil {
		return nil, err
	}

	c.verifiers[provider] = verifier
	return verifier, nil
}

func (h *Handler) verifyIDToken(ctx context.Context, provider, idToken string, cfg config.OAuthProviderConfig, expectedNonce string) (*oidcIDTokenClaims, error) {
	if idToken == "" {
		if requiresVerifiedIDToken(provider, cfg) {
			return nil, errors.New("OIDC provider response is missing an ID token")
		}
		return nil, nil
	}
	if h == nil || h.oidcVerifiers == nil {
		return nil, errors.New("oidc verifier cache not initialized")
	}

	verifier, err := h.oidcVerifiers.verifierFor(provider, cfg)
	if err != nil {
		return nil, fmt.Errorf("oidc verifier unavailable: %w", err)
	}
	claims, err := verifier.Verify(ctx, idToken, expectedNonce)
	if err != nil {
		return nil, err
	}
	return convertOIDCClaims(claims), nil
}

func requiresVerifiedIDToken(provider string, cfg config.OAuthProviderConfig) bool {
	provider = strings.ToLower(strings.TrimSpace(provider))
	if _, known := knownOIDCProviderDefaults[provider]; known {
		return true
	}
	if strings.TrimSpace(cfg.JWKSURL) != "" {
		return true
	}
	for _, scope := range cfg.Scopes {
		if strings.EqualFold(strings.TrimSpace(scope), "openid") {
			return true
		}
	}
	return false
}

func convertOIDCClaims(claims *oidc.Claims) *oidcIDTokenClaims {
	if claims == nil {
		return nil
	}
	return &oidcIDTokenClaims{
		RegisteredClaims:  claims.RegisteredClaims,
		Nonce:             claims.Nonce,
		Name:              claims.Name,
		PreferredUsername: claims.PreferredUsername,
		Picture:           claims.Picture,
	}
}

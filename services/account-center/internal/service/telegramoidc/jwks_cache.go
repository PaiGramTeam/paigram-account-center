package telegramoidc

import (
	"context"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"sync"
	"time"

	"go.uber.org/zap"
)

const jwksCacheTTL = 1 * time.Hour

// ErrJWKSUnavailable is returned when the JWKS endpoint is unreachable, the
// payload is malformed, or no key matches the requested kid.
var ErrJWKSUnavailable = errors.New("telegramoidc: jwks unavailable")

type jwk struct {
	Kty string `json:"kty"`
	Kid string `json:"kid"`
	N   string `json:"n"`
	E   string `json:"e"`
	Alg string `json:"alg"`
	Use string `json:"use"`
}

type jwksDocument struct {
	Keys []jwk `json:"keys"`
}

// JWKSCache fetches and caches the Telegram OIDC public keys.
type JWKSCache struct {
	endpoint string
	client   *http.Client
	ttl      time.Duration
	logger   *zap.Logger

	mu        sync.RWMutex
	fetchedAt time.Time
	keys      map[string]*rsa.PublicKey
}

func NewJWKSCache(endpoint string, client *http.Client, logger *zap.Logger) *JWKSCache {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &JWKSCache{
		endpoint: endpoint,
		client:   client,
		ttl:      jwksCacheTTL,
		logger:   logger,
		keys:     make(map[string]*rsa.PublicKey),
	}
}

// Get returns the RSA public key for the given kid, refreshing the cache if
// stale or if the kid is missing on first lookup.
func (c *JWKSCache) Get(ctx context.Context, kid string) (*rsa.PublicKey, error) {
	c.mu.RLock()
	if time.Since(c.fetchedAt) < c.ttl {
		if key, ok := c.keys[kid]; ok {
			c.mu.RUnlock()
			return key, nil
		}
	}
	c.mu.RUnlock()
	return c.refreshAndGet(ctx, kid)
}

func (c *JWKSCache) refreshAndGet(ctx context.Context, kid string) (*rsa.PublicKey, error) {
	// Hold the write lock during the entire HTTP fetch. This intentionally
	// serializes concurrent refresh attempts against the rate-limited Telegram
	// JWKS endpoint; the small latency penalty on contemporary verifies is
	// preferable to thundering-herd requests that risk getting throttled.
	c.mu.Lock()
	defer c.mu.Unlock()
	if time.Since(c.fetchedAt) < c.ttl {
		if key, ok := c.keys[kid]; ok {
			return key, nil
		}
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.endpoint, nil)
	if err != nil {
		c.logger.Warn("jwks build request failed", zap.Error(err))
		return nil, fmt.Errorf("%w: build request: %v", ErrJWKSUnavailable, err)
	}
	resp, err := c.client.Do(req)
	if err != nil {
		c.logger.Warn("jwks fetch failed", zap.Error(err))
		return nil, fmt.Errorf("%w: fetch: %v", ErrJWKSUnavailable, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		c.logger.Warn("jwks non-200", zap.Int("status", resp.StatusCode))
		return nil, fmt.Errorf("%w: status %d", ErrJWKSUnavailable, resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		c.logger.Warn("jwks body read failed", zap.Error(err))
		return nil, fmt.Errorf("%w: read body: %v", ErrJWKSUnavailable, err)
	}
	var doc jwksDocument
	if err := json.Unmarshal(body, &doc); err != nil {
		c.logger.Warn("jwks unmarshal failed", zap.Error(err))
		return nil, fmt.Errorf("%w: unmarshal: %v", ErrJWKSUnavailable, err)
	}

	keys := make(map[string]*rsa.PublicKey, len(doc.Keys))
	for _, k := range doc.Keys {
		if k.Kty != "RSA" {
			continue
		}
		nBytes, err := base64.RawURLEncoding.DecodeString(k.N)
		if err != nil {
			continue
		}
		eBytes, err := base64.RawURLEncoding.DecodeString(k.E)
		if err != nil {
			continue
		}
		// Defense-in-depth: cap exponent length. Standard RSA exponent is
		// 65537 (3 bytes); rejecting >8 bytes prevents silent truncation of
		// a malformed or malicious JWK with an oversized exponent.
		if len(eBytes) > 8 {
			continue
		}
		e := 0
		for _, b := range eBytes {
			e = e<<8 + int(b)
		}
		keys[k.Kid] = &rsa.PublicKey{
			N: new(big.Int).SetBytes(nBytes),
			E: e,
		}
	}
	c.keys = keys
	c.fetchedAt = time.Now()

	if key, ok := c.keys[kid]; ok {
		return key, nil
	}
	c.logger.Warn("jwks kid not found", zap.String("kid", kid))
	return nil, fmt.Errorf("%w: kid %s not found", ErrJWKSUnavailable, kid)
}

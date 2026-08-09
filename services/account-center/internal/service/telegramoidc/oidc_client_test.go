package telegramoidc_test

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"paigram/internal/service/telegramoidc"
)

func newClient(t *testing.T, tokenURL, jwksURL string) *telegramoidc.Client {
	t.Helper()
	return telegramoidc.NewClient(telegramoidc.Config{
		ClientID:          "test-client",
		ClientSecret:      "test-secret",
		RedirectURI:       "https://example.test/cb",
		AuthorizeEndpoint: "https://oauth.telegram.org/auth",
		TokenEndpoint:     tokenURL,
		JWKSEndpoint:      jwksURL,
		ExpectedIssuer:    "https://oauth.telegram.org",
	}, zap.NewNop())
}

func TestAuthorizeURL_ContainsPKCE(t *testing.T) {
	c := newClient(t, "", "")
	u := c.AuthorizeURL("state_x", "challenge_y")
	assert.Contains(t, u, "client_id=test-client")
	assert.Contains(t, u, "response_type=code")
	assert.Contains(t, u, "scope=openid+profile")
	assert.Contains(t, u, "code_challenge=challenge_y")
	assert.Contains(t, u, "code_challenge_method=S256")
	assert.Contains(t, u, "state=state_x")
	assert.Contains(t, u, "redirect_uri=https%3A%2F%2Fexample.test%2Fcb")
}

func TestExchangeCode_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u, p, ok := r.BasicAuth()
		require.True(t, ok)
		assert.Equal(t, "test-client", u)
		assert.Equal(t, "test-secret", p)
		require.NoError(t, r.ParseForm())
		assert.Equal(t, "authorization_code", r.Form.Get("grant_type"))
		assert.Equal(t, "the_code", r.Form.Get("code"))
		assert.Equal(t, "the_verifier", r.Form.Get("code_verifier"))
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"access_token":"a","token_type":"Bearer","expires_in":3600,"id_token":"t"}`)
	}))
	defer srv.Close()
	c := newClient(t, srv.URL, "")

	tr, err := c.ExchangeCode(context.Background(), "the_code", "the_verifier")
	require.NoError(t, err)
	assert.Equal(t, "t", tr.IDToken)
}

func TestExchangeCode_400InvalidGrant(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprint(w, `{"error":"invalid_grant"}`)
	}))
	defer srv.Close()
	c := newClient(t, srv.URL, "")
	_, err := c.ExchangeCode(context.Background(), "x", "y")
	assert.ErrorIs(t, err, telegramoidc.ErrTokenExchangeRejected)
}

func TestExchangeCode_503Network(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()
	c := newClient(t, srv.URL, "")
	_, err := c.ExchangeCode(context.Background(), "x", "y")
	assert.ErrorIs(t, err, telegramoidc.ErrTokenExchangeFailed)
}

// --- id_token tests ---

func signTestToken(t *testing.T, key *rsa.PrivateKey, kid string, claims jwt.MapClaims) string {
	t.Helper()
	tok := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	tok.Header["kid"] = kid
	s, err := tok.SignedString(key)
	require.NoError(t, err)
	return s
}

func newJWKSServer(t *testing.T, key *rsa.PublicKey, kid string) *httptest.Server {
	t.Helper()
	nBytes := key.N.Bytes()
	eBytes := []byte{byte(key.E >> 16), byte(key.E >> 8), byte(key.E)}
	for len(eBytes) > 1 && eBytes[0] == 0 {
		eBytes = eBytes[1:]
	}
	body := map[string]any{
		"keys": []map[string]string{{
			"kty": "RSA",
			"kid": kid,
			"alg": "RS256",
			"use": "sig",
			"n":   base64.RawURLEncoding.EncodeToString(nBytes),
			"e":   base64.RawURLEncoding.EncodeToString(eBytes),
		}},
	}
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(body)
	}))
}

func goodClaims() jwt.MapClaims {
	return jwt.MapClaims{
		"sub":  "u-sub",
		"id":   float64(987654321),
		"iss":  "https://oauth.telegram.org",
		"aud":  "test-client",
		"iat":  float64(time.Now().Unix()),
		"exp":  float64(time.Now().Add(1 * time.Hour).Unix()),
		"name": "John Doe",
	}
}

func TestVerifyIDToken_Valid(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	srv := newJWKSServer(t, &key.PublicKey, "test-kid")
	defer srv.Close()
	c := newClient(t, "", srv.URL)

	tok := signTestToken(t, key, "test-kid", goodClaims())
	claims, err := c.VerifyIDToken(context.Background(), tok)
	require.NoError(t, err)
	assert.Equal(t, int64(987654321), claims.ID)
	assert.Equal(t, "John Doe", claims.Name)
}

func TestVerifyIDToken_BadIss(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	srv := newJWKSServer(t, &key.PublicKey, "k")
	defer srv.Close()
	c := newClient(t, "", srv.URL)
	bad := goodClaims()
	bad["iss"] = "https://evil.test"
	tok := signTestToken(t, key, "k", bad)
	_, err = c.VerifyIDToken(context.Background(), tok)
	assert.ErrorIs(t, err, telegramoidc.ErrIDTokenInvalid)
}

func TestVerifyIDToken_BadAud(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	srv := newJWKSServer(t, &key.PublicKey, "k")
	defer srv.Close()
	c := newClient(t, "", srv.URL)
	bad := goodClaims()
	bad["aud"] = "wrong"
	tok := signTestToken(t, key, "k", bad)
	_, err = c.VerifyIDToken(context.Background(), tok)
	assert.ErrorIs(t, err, telegramoidc.ErrIDTokenInvalid)
}

func TestVerifyIDToken_Expired(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	srv := newJWKSServer(t, &key.PublicKey, "k")
	defer srv.Close()
	c := newClient(t, "", srv.URL)
	bad := goodClaims()
	bad["exp"] = float64(time.Now().Add(-1 * time.Minute).Unix())
	tok := signTestToken(t, key, "k", bad)
	_, err = c.VerifyIDToken(context.Background(), tok)
	assert.ErrorIs(t, err, telegramoidc.ErrIDTokenInvalid)
}

func TestVerifyIDToken_MissingIDClaim(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	srv := newJWKSServer(t, &key.PublicKey, "k")
	defer srv.Close()
	c := newClient(t, "", srv.URL)
	bad := goodClaims()
	delete(bad, "id")
	tok := signTestToken(t, key, "k", bad)
	_, err = c.VerifyIDToken(context.Background(), tok)
	assert.ErrorIs(t, err, telegramoidc.ErrTelegramIDMissing)
}

func TestVerifyIDToken_NoKidMatch(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	srv := newJWKSServer(t, &key.PublicKey, "real-kid")
	defer srv.Close()
	c := newClient(t, "", srv.URL)
	tok := signTestToken(t, key, "wrong-kid", goodClaims())
	_, err = c.VerifyIDToken(context.Background(), tok)
	assert.ErrorIs(t, err, telegramoidc.ErrJWKSUnavailable)
}

func TestVerifyIDToken_BadSignature(t *testing.T) {
	key1, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	key2, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	srv := newJWKSServer(t, &key1.PublicKey, "k") // serve key1 pubkey
	defer srv.Close()
	c := newClient(t, "", srv.URL)
	tok := signTestToken(t, key2, "k", goodClaims()) // sign with key2
	_, err = c.VerifyIDToken(context.Background(), tok)
	assert.ErrorIs(t, err, telegramoidc.ErrIDTokenInvalid)
}

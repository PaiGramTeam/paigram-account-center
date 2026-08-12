package testutil

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"testing"

	"github.com/stretchr/testify/require"

	"paigram/internal/config"
)

func NewAuthConfig(t testing.TB) (config.AuthConfig, ed25519.PublicKey) {
	t.Helper()
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	privateDER, err := x509.MarshalPKCS8PrivateKey(privateKey)
	require.NoError(t, err)

	return config.AuthConfig{
		AccessTokenTTLSeconds:      900,
		RefreshTokenTTLSeconds:     604800,
		ServiceTicketTTLSeconds:    300,
		ServiceTicketIssuer:        "paigram-account-center",
		ServiceTicketKeyID:         "test-key",
		ServiceTicketPrivateKeyPEM: string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: privateDER})),
		OAuthIssuer:                "account-center",
		OAuthAccessTokenTTLSeconds: 3600,
		OAuthSigningKey:            "0123456789abcdef0123456789abcdef",
	}, publicKey
}

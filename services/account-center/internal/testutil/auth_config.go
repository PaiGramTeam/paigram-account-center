package testutil

import (
	"crypto/ed25519"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	contractticket "github.com/PaiGramTeam/paigram-account-center/contracts/runtime/go/serviceticket"
	"github.com/stretchr/testify/require"

	"paigram/internal/config"
)

func NewAuthConfig(t testing.TB) (config.AuthConfig, ed25519.PublicKey) {
	t.Helper()
	privateKeyPEM, publicKeyPEM, err := contractticket.GenerateKeyPairPEM()
	require.NoError(t, err)
	publicKey, err := contractticket.ParsePublicKeyPEM(publicKeyPEM)
	require.NoError(t, err)
	signingKeyFile := WriteServiceTicketSigningKey(t, "test-key", privateKeyPEM)

	return config.AuthConfig{
		AccessTokenTTLSeconds:       900,
		RefreshTokenTTLSeconds:      604800,
		ServiceTicketTTLSeconds:     300,
		ServiceTicketIssuer:         "paigram-account-center",
		ServiceTicketSigningKeyFile: signingKeyFile,
		OAuthIssuer:                 "account-center",
		OAuthAccessTokenTTLSeconds:  3600,
		OAuthSigningKey:             "0123456789abcdef0123456789abcdef",
	}, publicKey
}

func WriteServiceTicketSigningKey(t testing.TB, keyID, privateKeyPEM string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "service-ticket-signing-key.json")
	raw, err := json.Marshal(contractticket.SigningKeyFile{KeyID: keyID, PrivateKeyPEM: privateKeyPEM})
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, raw, 0o600))
	return path
}

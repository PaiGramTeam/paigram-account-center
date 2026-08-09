package data

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseEd25519PublicKeyPEM(t *testing.T) {
	publicKey, _, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)
	pemBytes := ed25519PublicKeyPEM(t, publicKey)

	parsed, err := ParseEd25519PublicKeyPEM(string(pemBytes))

	require.NoError(t, err)
	require.Equal(t, publicKey, parsed)
}

func TestParseEd25519PublicKeyPEMRejectsInvalidPEM(t *testing.T) {
	_, err := ParseEd25519PublicKeyPEM("not pem")

	require.ErrorContains(t, err, "public key PEM")
}

func TestParseEd25519PublicKeyPEMRejectsNonEd25519Key(t *testing.T) {
	pemBytes := pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: []byte("not der")})

	_, err := ParseEd25519PublicKeyPEM(string(pemBytes))

	require.Error(t, err)
}

func TestParseEd25519PublicKeyPEMRejectsRSAPublicKey(t *testing.T) {
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	der, err := x509.MarshalPKIXPublicKey(&privateKey.PublicKey)
	require.NoError(t, err)
	pemBytes := pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: der})

	_, err = ParseEd25519PublicKeyPEM(string(pemBytes))

	require.ErrorContains(t, err, "Ed25519")
}

func ed25519PublicKeyPEM(t *testing.T, publicKey ed25519.PublicKey) []byte {
	t.Helper()

	der, err := x509.MarshalPKIXPublicKey(publicKey)
	require.NoError(t, err)
	return pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: der})
}

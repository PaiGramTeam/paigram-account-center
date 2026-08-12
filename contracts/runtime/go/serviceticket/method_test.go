package serviceticket

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"strings"
	"testing"

	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/require"
)

func TestFullySpecifiedEd25519SignsAndVerifies(t *testing.T) {
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)

	raw, err := jwt.NewWithClaims(SigningMethodEd25519, jwt.MapClaims{"sub": "consumer:paigram"}).SignedString(privateKey)
	require.NoError(t, err)

	parsed, err := jwt.Parse(raw, func(token *jwt.Token) (any, error) {
		require.Equal(t, AlgorithmEd25519, token.Method.Alg())
		return publicKey, nil
	}, jwt.WithValidMethods([]string{AlgorithmEd25519}))
	require.NoError(t, err)
	require.True(t, parsed.Valid)

	_, err = jwt.Parse(raw, func(_ *jwt.Token) (any, error) {
		return publicKey, nil
	}, jwt.WithValidMethods([]string{jwt.SigningMethodEdDSA.Alg()}))
	require.Error(t, err)
}

func TestParseEd25519PEMKeyPair(t *testing.T) {
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)

	privateDER, err := x509.MarshalPKCS8PrivateKey(privateKey)
	require.NoError(t, err)
	publicDER, err := x509.MarshalPKIXPublicKey(publicKey)
	require.NoError(t, err)

	parsedPrivate, err := ParsePrivateKeyPEM(string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: privateDER})))
	require.NoError(t, err)
	require.Equal(t, privateKey, parsedPrivate)

	parsedPublic, err := ParsePublicKeyPEM(string(pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: publicDER})))
	require.NoError(t, err)
	require.Equal(t, publicKey, parsedPublic)

	escapedPrivate := strings.ReplaceAll(string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: privateDER})), "\n", `\n`)
	parsedEscapedPrivate, err := ParsePrivateKeyPEM(escapedPrivate)
	require.NoError(t, err)
	require.Equal(t, privateKey, parsedEscapedPrivate)

	escapedPublic := strings.ReplaceAll(string(pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: publicDER})), "\n", `\n`)
	parsedEscapedPublic, err := ParsePublicKeyPEM(escapedPublic)
	require.NoError(t, err)
	require.Equal(t, publicKey, parsedEscapedPublic)
}

func TestGenerateKeyPairPEMRoundTrips(t *testing.T) {
	privateKeyPEM, publicKeyPEM, err := GenerateKeyPairPEM()
	require.NoError(t, err)
	privateKey, err := ParsePrivateKeyPEM(privateKeyPEM)
	require.NoError(t, err)
	publicKey, err := ParsePublicKeyPEM(publicKeyPEM)
	require.NoError(t, err)
	require.Equal(t, publicKey, privateKey.Public())
}

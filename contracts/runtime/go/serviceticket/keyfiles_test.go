package serviceticket

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/require"
)

func TestFileIssuerReloadsSigningKeyAndPublicKeyringSupportsOverlap(t *testing.T) {
	dir := t.TempDir()
	signingPath := filepath.Join(dir, "signing.json")
	keyringPath := filepath.Join(dir, "keyring.json")
	oldPrivate, oldPublic, err := GenerateKeyPairPEM()
	require.NoError(t, err)
	newPrivate, newPublic, err := GenerateKeyPairPEM()
	require.NoError(t, err)

	writeKeyJSON(t, signingPath, SigningKeyFile{KeyID: "old", PrivateKeyPEM: oldPrivate})
	writeKeyJSON(t, keyringPath, PublicKeyringFile{Keys: []PublicKeyEntry{{KeyID: "old", PublicKeyPEM: oldPublic}}})
	issuer, err := NewFileIssuer(FileIssuerConfig{Issuer: "account", TTL: time.Minute, SigningKeyFile: signingPath})
	require.NoError(t, err)
	oldTicket := issueRotationTicket(t, issuer)
	require.NoError(t, verifyRotationTicket(keyringPath, oldTicket))

	writeKeyJSON(t, keyringPath, PublicKeyringFile{Keys: []PublicKeyEntry{
		{KeyID: "old", PublicKeyPEM: oldPublic},
		{KeyID: "new", PublicKeyPEM: newPublic},
	}})
	writeKeyJSON(t, signingPath, SigningKeyFile{KeyID: "new", PrivateKeyPEM: newPrivate})
	newTicket := issueRotationTicket(t, issuer)
	require.NoError(t, verifyRotationTicket(keyringPath, oldTicket))
	require.NoError(t, verifyRotationTicket(keyringPath, newTicket))

	writeKeyJSON(t, keyringPath, PublicKeyringFile{Keys: []PublicKeyEntry{{KeyID: "new", PublicKeyPEM: newPublic}}})
	require.Error(t, verifyRotationTicket(keyringPath, oldTicket))
	require.NoError(t, verifyRotationTicket(keyringPath, newTicket))
}

func TestFileIssuerRejectsMissingOrMalformedFiles(t *testing.T) {
	_, err := NewFileIssuer(FileIssuerConfig{Issuer: "account", TTL: time.Minute, SigningKeyFile: filepath.Join(t.TempDir(), "missing")})
	require.ErrorIs(t, err, ErrInvalidKeyFile)

	path := filepath.Join(t.TempDir(), "invalid.json")
	require.NoError(t, os.WriteFile(path, []byte(`{"kid":"bad"}`), 0o600))
	_, err = NewFileIssuer(FileIssuerConfig{Issuer: "account", TTL: time.Minute, SigningKeyFile: path})
	require.ErrorIs(t, err, ErrInvalidKeyFile)
}

func issueRotationTicket(t *testing.T, issuer TicketIssuer) string {
	t.Helper()
	token, _, err := issuer.Issue(TypeControl, "system:account-center", "platform", Claims{BindingRef: "binding"})
	require.NoError(t, err)
	return token
}

func verifyRotationTicket(path, raw string) error {
	_, err := jwt.Parse(raw, func(token *jwt.Token) (any, error) {
		keyID, _ := token.Header["kid"].(string)
		return ResolvePublicKeyFile(context.Background(), path, keyID)
	}, jwt.WithValidMethods([]string{SigningMethodEd25519.Alg()}))
	return err
}

func writeKeyJSON(t *testing.T, path string, value any) {
	t.Helper()
	raw, err := json.Marshal(value)
	require.NoError(t, err)
	temporary := path + ".new"
	require.NoError(t, os.WriteFile(temporary, raw, 0o600))
	require.NoError(t, os.Rename(temporary, path))
}

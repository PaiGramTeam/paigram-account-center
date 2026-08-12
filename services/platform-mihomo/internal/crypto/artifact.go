package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"io"
	"strings"
)

const artifactCipherVersion = "v2"

var ErrInvalidArtifactCiphertext = errors.New("invalid artifact ciphertext")

func EncryptArtifact(keySource KeyProvider, plaintext, bindingRef, accountKey, artifactType, scopeKey string) (string, error) {
	if keySource == nil {
		return "", ErrInvalidKeyring
	}
	keyID, masterKey, err := keySource.ActiveKey()
	if err != nil {
		return "", err
	}
	gcm, err := artifactGCM(masterKey)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	ciphertext := gcm.Seal(nonce, nonce, []byte(plaintext), artifactAAD(artifactCipherVersion, keyID, bindingRef, accountKey, artifactType, scopeKey))
	return artifactCipherVersion + "." + keyID + "." + base64.RawURLEncoding.EncodeToString(ciphertext), nil
}

func DecryptArtifact(keySource KeyProvider, encoded, bindingRef, accountKey, artifactType, scopeKey string) (string, error) {
	version, remainder, ok := strings.Cut(encoded, ".")
	if !ok || version != artifactCipherVersion {
		return "", ErrInvalidArtifactCiphertext
	}
	keyID, payload, ok := strings.Cut(remainder, ".")
	if !ok || keyID == "" {
		return "", ErrInvalidArtifactCiphertext
	}
	if keySource == nil {
		return "", ErrInvalidKeyring
	}
	masterKey, err := keySource.ResolveKey(keyID)
	if err != nil {
		return "", errors.Join(ErrInvalidArtifactCiphertext, err)
	}
	ciphertext, err := base64.RawURLEncoding.DecodeString(payload)
	if err != nil {
		return "", errors.Join(ErrInvalidArtifactCiphertext, err)
	}
	gcm, err := artifactGCM(masterKey)
	if err != nil {
		return "", err
	}
	if len(ciphertext) < gcm.NonceSize() {
		return "", ErrInvalidArtifactCiphertext
	}
	plaintext, err := gcm.Open(nil, ciphertext[:gcm.NonceSize()], ciphertext[gcm.NonceSize():], artifactAAD(version, keyID, bindingRef, accountKey, artifactType, scopeKey))
	if err != nil {
		return "", errors.Join(ErrInvalidArtifactCiphertext, err)
	}
	return string(plaintext), nil
}

func artifactGCM(masterKey []byte) (cipher.AEAD, error) {
	mac := hmac.New(sha256.New, masterKey)
	_, _ = mac.Write([]byte("paigram-mihomo-artifact-key-v1"))
	block, err := aes.NewCipher(mac.Sum(nil))
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}

func artifactAAD(parts ...string) []byte {
	digest := sha256.New()
	for _, part := range parts {
		_, _ = digest.Write([]byte{byte(len(part) >> 24), byte(len(part) >> 16), byte(len(part) >> 8), byte(len(part))})
		_, _ = digest.Write([]byte(part))
	}
	return digest.Sum(nil)
}

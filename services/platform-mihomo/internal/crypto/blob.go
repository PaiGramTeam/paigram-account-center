package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"io"
	"strings"
)

const credentialCipherVersion = "v2"

func EncryptString(keySource KeyProvider, plaintext string) (string, error) {
	if keySource == nil {
		return "", ErrInvalidKeyring
	}
	keyID, key, err := keySource.ActiveKey()
	if err != nil {
		return "", err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}

	aad := []byte(credentialCipherVersion + ":" + keyID)
	ciphertext := gcm.Seal(nonce, nonce, []byte(plaintext), aad)
	return credentialCipherVersion + "." + keyID + "." + base64.RawURLEncoding.EncodeToString(ciphertext), nil
}

func DecryptString(keySource KeyProvider, encoded string) (string, error) {
	version, remainder, ok := strings.Cut(encoded, ".")
	if !ok || version != credentialCipherVersion {
		return "", ErrInvalidKeyring
	}
	keyID, payload, ok := strings.Cut(remainder, ".")
	if !ok || keyID == "" {
		return "", ErrInvalidKeyring
	}
	if keySource == nil {
		return "", ErrInvalidKeyring
	}
	key, err := keySource.ResolveKey(keyID)
	if err != nil {
		return "", err
	}
	data, err := base64.RawURLEncoding.DecodeString(payload)
	if err != nil {
		return "", err
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}

	if len(data) < gcm.NonceSize() {
		return "", fmt.Errorf("ciphertext too short")
	}

	nonce := data[:gcm.NonceSize()]
	plaintext, err := gcm.Open(nil, nonce, data[gcm.NonceSize():], []byte(version+":"+keyID))
	if err != nil {
		return "", err
	}

	return string(plaintext), nil
}

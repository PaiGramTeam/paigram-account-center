package credentials

import (
	"crypto/rand"
	"encoding/base64"

	"golang.org/x/crypto/bcrypt"
)

// Client-secret hashing parameters. Bcrypt cost 12 matches the OWASP
// minimum and what the pre-Path-D machineidentity package used; tests run
// with this same cost rather than the bcrypt library's default of 10.
const (
	clientSecretBytes      = 48
	clientSecretBcryptCost = 12
)

// dummyClientSecretHash is a constant-cost bcrypt hash used to make
// Service.VerifySecret time-equivalent between the "credential found,
// wrong secret" and "credential not found / disabled" paths. Without
// this, an attacker can enumerate valid client_ids by measuring the
// ~250ms bcrypt cost-12 delta vs the ~1ms DB miss.
//
// Generation:
//
//	out, _ := bcrypt.GenerateFromPassword(
//	    []byte("path-d-dummy-secret-not-real-do-not-use"),
//	    clientSecretBcryptCost, // 12
//	)
//	// out → the literal string below.
//
// The plaintext is irrelevant; this hash is never expected to match.
// Rotate by re-running the generator if the bcrypt format ever needs
// upgrading.
const dummyClientSecretHash = "$2a$12$r4yE1DZhgNY0y5sB0Pk5XOweX7vHLqOtTPUz3mfmpWnTxUw30EX2a"

// GenerateClientSecret returns a URL-safe base64-encoded 384-bit random
// secret. It is the plaintext value handed to the operator at credential
// creation time; we never store it.
func GenerateClientSecret() (string, error) {
	raw := make([]byte, clientSecretBytes)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}

// HashClientSecret returns the bcrypt hash of the given plaintext secret.
func HashClientSecret(secret string) (string, error) {
	hashed, err := bcrypt.GenerateFromPassword([]byte(secret), clientSecretBcryptCost)
	if err != nil {
		return "", err
	}
	return string(hashed), nil
}

// VerifyClientSecret compares a plaintext client_secret against its bcrypt
// hash. Returns nil on match, bcrypt.ErrMismatchedHashAndPassword (or a
// related bcrypt error) on mismatch.
func VerifyClientSecret(hash string, secret string) error {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(secret))
}

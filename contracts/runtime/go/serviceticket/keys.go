package serviceticket

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"strings"
)

func GenerateKeyPairPEM() (string, string, error) {
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return "", "", fmt.Errorf("generate Ed25519 key pair: %w", err)
	}
	privateDER, err := x509.MarshalPKCS8PrivateKey(privateKey)
	if err != nil {
		return "", "", fmt.Errorf("marshal Ed25519 private key: %w", err)
	}
	publicDER, err := x509.MarshalPKIXPublicKey(publicKey)
	if err != nil {
		return "", "", fmt.Errorf("marshal Ed25519 public key: %w", err)
	}
	privatePEM := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: privateDER})
	publicPEM := pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: publicDER})
	return string(privatePEM), string(publicPEM), nil
}

var (
	ErrInvalidPrivateKey = errors.New("invalid Ed25519 private key")
	ErrInvalidPublicKey  = errors.New("invalid Ed25519 public key")
)

func ParsePrivateKeyPEM(raw string) (ed25519.PrivateKey, error) {
	block, rest := pem.Decode([]byte(normalizePEM(raw)))
	if block == nil || len(rest) != 0 {
		return nil, ErrInvalidPrivateKey
	}

	parsed, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidPrivateKey, err)
	}
	privateKey, ok := parsed.(ed25519.PrivateKey)
	if !ok || len(privateKey) != ed25519.PrivateKeySize {
		return nil, ErrInvalidPrivateKey
	}

	return privateKey, nil
}

func ParsePublicKeyPEM(raw string) (ed25519.PublicKey, error) {
	block, rest := pem.Decode([]byte(normalizePEM(raw)))
	if block == nil || len(rest) != 0 {
		return nil, ErrInvalidPublicKey
	}

	parsed, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidPublicKey, err)
	}
	publicKey, ok := parsed.(ed25519.PublicKey)
	if !ok || len(publicKey) != ed25519.PublicKeySize {
		return nil, ErrInvalidPublicKey
	}

	return publicKey, nil
}

func normalizePEM(raw string) string {
	return strings.ReplaceAll(strings.TrimSpace(raw), `\n`, "\n")
}

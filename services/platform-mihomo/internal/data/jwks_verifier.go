package data

import (
	"context"
	"crypto/ed25519"
	"crypto/x509"
	"encoding/pem"
	"fmt"
)

type PublicKeyResolver interface {
	Resolve(ctx context.Context, kid string) (ed25519.PublicKey, error)
}

type StaticPublicKeyResolver struct {
	keys map[string]ed25519.PublicKey
}

func NewStaticPublicKeyResolver(kid string, key ed25519.PublicKey) *StaticPublicKeyResolver {
	keys := map[string]ed25519.PublicKey{}
	if kid != "" && len(key) > 0 {
		keys[kid] = append(ed25519.PublicKey(nil), key...)
	}
	return &StaticPublicKeyResolver{keys: keys}
}

func (r *StaticPublicKeyResolver) Resolve(_ context.Context, kid string) (ed25519.PublicKey, error) {
	if r == nil || kid == "" {
		return nil, fmt.Errorf("service ticket public key not found for kid %q", kid)
	}
	key, ok := r.keys[kid]
	if !ok {
		return nil, fmt.Errorf("service ticket public key not found for kid %q", kid)
	}
	return append(ed25519.PublicKey(nil), key...), nil
}

func ParseEd25519PublicKeyPEM(publicKeyPEM string) (ed25519.PublicKey, error) {
	block, _ := pem.Decode([]byte(publicKeyPEM))
	if block == nil {
		return nil, fmt.Errorf("service ticket public key PEM is invalid")
	}
	parsed, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("parse service ticket public key PEM: %w", err)
	}
	key, ok := parsed.(ed25519.PublicKey)
	if !ok {
		return nil, fmt.Errorf("service ticket public key PEM must contain an Ed25519 public key")
	}
	if len(key) != ed25519.PublicKeySize {
		return nil, fmt.Errorf("service ticket public key PEM has invalid Ed25519 key size")
	}
	return append(ed25519.PublicKey(nil), key...), nil
}

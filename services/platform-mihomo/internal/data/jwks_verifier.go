package data

import (
	"context"
	"crypto/ed25519"
	"fmt"

	contractticket "github.com/PaiGramTeam/paigram-account-center/contracts/runtime/go/serviceticket"
)

type PublicKeyResolver interface {
	Resolve(ctx context.Context, kid string) (ed25519.PublicKey, error)
}

type StaticPublicKeyResolver struct {
	keys map[string]ed25519.PublicKey
}

type FilePublicKeyResolver struct {
	path string
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

func NewFilePublicKeyResolver(path string) (*FilePublicKeyResolver, error) {
	if _, err := contractticket.LoadPublicKeyring(path); err != nil {
		return nil, fmt.Errorf("load service ticket public keyring: %w", err)
	}
	return &FilePublicKeyResolver{path: path}, nil
}

func (r *FilePublicKeyResolver) Resolve(ctx context.Context, kid string) (ed25519.PublicKey, error) {
	if r == nil {
		return nil, fmt.Errorf("service ticket public key resolver is nil")
	}
	key, err := contractticket.ResolvePublicKeyFile(ctx, r.path, kid)
	if err != nil {
		return nil, fmt.Errorf("resolve service ticket public key: %w", err)
	}
	return key, nil
}

func ParseEd25519PublicKeyPEM(publicKeyPEM string) (ed25519.PublicKey, error) {
	key, err := contractticket.ParsePublicKeyPEM(publicKeyPEM)
	if err != nil {
		return nil, fmt.Errorf("service ticket public key PEM is invalid: %w", err)
	}
	return key, nil
}

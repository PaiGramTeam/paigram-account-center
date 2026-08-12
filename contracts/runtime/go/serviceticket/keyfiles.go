package serviceticket

import (
	"context"
	"crypto/ed25519"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"time"
)

const maximumKeyFileSize = 1 << 20

var ErrInvalidKeyFile = errors.New("invalid service ticket key file")

type TicketIssuer interface {
	Issue(ticketType, subject, audience string, claims Claims) (string, time.Time, error)
}

type SigningKeyFile struct {
	KeyID         string `json:"kid"`
	PrivateKeyPEM string `json:"private_key_pem"`
}

type PublicKeyEntry struct {
	KeyID        string `json:"kid"`
	PublicKeyPEM string `json:"public_key_pem"`
}

type PublicKeyringFile struct {
	Keys []PublicKeyEntry `json:"keys"`
}

type FileIssuerConfig struct {
	Issuer         string
	TTL            time.Duration
	SigningKeyFile string
}

type FileIssuer struct {
	config FileIssuerConfig
}

func NewFileIssuer(config FileIssuerConfig) (*FileIssuer, error) {
	issuer := &FileIssuer{config: config}
	if _, err := issuer.load(); err != nil {
		return nil, err
	}
	return issuer, nil
}

func (s *FileIssuer) Issue(ticketType, subject, audience string, claims Claims) (string, time.Time, error) {
	issuer, err := s.load()
	if err != nil {
		return "", time.Time{}, err
	}
	return issuer.Issue(ticketType, subject, audience, claims)
}

func (s *FileIssuer) load() (*Issuer, error) {
	if s == nil || s.config.SigningKeyFile == "" {
		return nil, ErrInvalidKeyFile
	}
	var key SigningKeyFile
	if err := readJSONKeyFile(s.config.SigningKeyFile, &key); err != nil {
		return nil, err
	}
	issuer, err := NewIssuer(IssuerConfig{
		Issuer:        s.config.Issuer,
		KeyID:         key.KeyID,
		TTL:           s.config.TTL,
		PrivateKeyPEM: key.PrivateKeyPEM,
	})
	if err != nil {
		return nil, errors.Join(ErrInvalidKeyFile, err)
	}
	return issuer, nil
}

func LoadPublicKeyring(path string) (map[string]ed25519.PublicKey, error) {
	var keyring PublicKeyringFile
	if err := readJSONKeyFile(path, &keyring); err != nil {
		return nil, err
	}
	if len(keyring.Keys) == 0 {
		return nil, ErrInvalidKeyFile
	}
	keys := make(map[string]ed25519.PublicKey, len(keyring.Keys))
	for _, entry := range keyring.Keys {
		if entry.KeyID == "" {
			return nil, ErrInvalidKeyFile
		}
		if _, exists := keys[entry.KeyID]; exists {
			return nil, fmt.Errorf("%w: duplicate kid %q", ErrInvalidKeyFile, entry.KeyID)
		}
		key, err := ParsePublicKeyPEM(entry.PublicKeyPEM)
		if err != nil {
			return nil, errors.Join(ErrInvalidKeyFile, err)
		}
		keys[entry.KeyID] = key
	}
	return keys, nil
}

func ResolvePublicKeyFile(_ context.Context, path, keyID string) (ed25519.PublicKey, error) {
	keys, err := LoadPublicKeyring(path)
	if err != nil {
		return nil, err
	}
	key, ok := keys[keyID]
	if !ok {
		return nil, fmt.Errorf("%w: kid %q not found", ErrInvalidKeyFile, keyID)
	}
	return append(ed25519.PublicKey(nil), key...), nil
}

func readJSONKeyFile(path string, target any) error {
	info, err := os.Stat(path)
	if err != nil {
		return errors.Join(ErrInvalidKeyFile, err)
	}
	if !info.Mode().IsRegular() || info.Size() <= 0 || info.Size() > maximumKeyFileSize {
		return ErrInvalidKeyFile
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return errors.Join(ErrInvalidKeyFile, err)
	}
	if err := json.Unmarshal(raw, target); err != nil {
		return errors.Join(ErrInvalidKeyFile, err)
	}
	return nil
}

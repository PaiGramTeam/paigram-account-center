package crypto

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"regexp"
	"strings"
)

const maximumKeyringFileSize = 1 << 20

var ErrInvalidKeyring = errors.New("invalid encryption keyring")

var encryptionKeyIDPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)

type KeyProvider interface {
	ActiveKey() (string, []byte, error)
	ResolveKey(keyID string) ([]byte, error)
}

type KeyringFile struct {
	ActiveKeyID string         `json:"active_kid"`
	Keys        []KeyringEntry `json:"keys"`
}

type KeyringEntry struct {
	KeyID     string `json:"kid"`
	KeyBase64 string `json:"key_base64"`
}

type FileKeyring struct {
	path string
}

type staticKeyProvider struct {
	key []byte
}

func NewFileKeyring(path string) (*FileKeyring, error) {
	keyring := &FileKeyring{path: path}
	if _, _, err := keyring.load(); err != nil {
		return nil, err
	}
	return keyring, nil
}

func NewStaticKeyProvider(key []byte) KeyProvider {
	return &staticKeyProvider{key: append([]byte(nil), key...)}
}

func EnvelopeNeedsReencryption(keySource KeyProvider, encoded string) (bool, error) {
	_, remainder, ok := strings.Cut(encoded, ".")
	if !ok {
		return false, ErrInvalidKeyring
	}
	keyID, _, ok := strings.Cut(remainder, ".")
	if !ok || keyID == "" {
		return false, ErrInvalidKeyring
	}
	if keySource == nil {
		return false, ErrInvalidKeyring
	}
	activeKeyID, _, err := keySource.ActiveKey()
	if err != nil {
		return false, err
	}
	return keyID != activeKeyID, nil
}

func (r *FileKeyring) ActiveKey() (string, []byte, error) {
	activeKeyID, keys, err := r.load()
	if err != nil {
		return "", nil, err
	}
	return activeKeyID, append([]byte(nil), keys[activeKeyID]...), nil
}

func (r *FileKeyring) ResolveKey(keyID string) ([]byte, error) {
	_, keys, err := r.load()
	if err != nil {
		return nil, err
	}
	key, ok := keys[keyID]
	if !ok {
		return nil, fmt.Errorf("%w: kid %q not found", ErrInvalidKeyring, keyID)
	}
	return append([]byte(nil), key...), nil
}

func (r *FileKeyring) load() (string, map[string][]byte, error) {
	if r == nil || r.path == "" {
		return "", nil, ErrInvalidKeyring
	}
	info, err := os.Stat(r.path)
	if err != nil {
		return "", nil, errors.Join(ErrInvalidKeyring, err)
	}
	if !info.Mode().IsRegular() || info.Size() <= 0 || info.Size() > maximumKeyringFileSize {
		return "", nil, ErrInvalidKeyring
	}
	raw, err := os.ReadFile(r.path)
	if err != nil {
		return "", nil, errors.Join(ErrInvalidKeyring, err)
	}
	var keyring KeyringFile
	if err := json.Unmarshal(raw, &keyring); err != nil {
		return "", nil, errors.Join(ErrInvalidKeyring, err)
	}
	if !validEncryptionKeyID(keyring.ActiveKeyID) || len(keyring.Keys) == 0 {
		return "", nil, ErrInvalidKeyring
	}
	keys := make(map[string][]byte, len(keyring.Keys))
	for _, entry := range keyring.Keys {
		if !validEncryptionKeyID(entry.KeyID) {
			return "", nil, ErrInvalidKeyring
		}
		if _, exists := keys[entry.KeyID]; exists {
			return "", nil, fmt.Errorf("%w: duplicate kid %q", ErrInvalidKeyring, entry.KeyID)
		}
		key, err := base64.RawStdEncoding.DecodeString(entry.KeyBase64)
		if err != nil || len(key) != 32 {
			return "", nil, fmt.Errorf("%w: kid %q must contain a 32-byte key", ErrInvalidKeyring, entry.KeyID)
		}
		keys[entry.KeyID] = key
	}
	if _, exists := keys[keyring.ActiveKeyID]; !exists {
		return "", nil, fmt.Errorf("%w: active kid %q not found", ErrInvalidKeyring, keyring.ActiveKeyID)
	}
	return keyring.ActiveKeyID, keys, nil
}

func validEncryptionKeyID(keyID string) bool {
	return encryptionKeyIDPattern.MatchString(keyID)
}

func (s *staticKeyProvider) ActiveKey() (string, []byte, error) {
	if len(s.key) != 32 {
		return "", nil, ErrInvalidKeyring
	}
	return "static", append([]byte(nil), s.key...), nil
}

func (s *staticKeyProvider) ResolveKey(keyID string) ([]byte, error) {
	if keyID != "static" || len(s.key) != 32 {
		return nil, ErrInvalidKeyring
	}
	return append([]byte(nil), s.key...), nil
}

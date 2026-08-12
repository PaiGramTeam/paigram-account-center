package secretfile

import (
	"errors"
	"os"
	"strings"
)

const maximumSecretSize = 1 << 20

var ErrInvalidSecretFile = errors.New("invalid secret file")

func Read(path string) (string, error) {
	if path == "" {
		return "", ErrInvalidSecretFile
	}
	info, err := os.Stat(path)
	if err != nil {
		return "", errors.Join(ErrInvalidSecretFile, err)
	}
	if !info.Mode().IsRegular() || info.Size() <= 0 || info.Size() > maximumSecretSize {
		return "", ErrInvalidSecretFile
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return "", errors.Join(ErrInvalidSecretFile, err)
	}
	value := strings.TrimSuffix(strings.TrimSuffix(string(raw), "\n"), "\r")
	if value == "" || strings.ContainsRune(value, '\x00') {
		return "", ErrInvalidSecretFile
	}
	return value, nil
}

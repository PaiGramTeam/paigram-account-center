package secretfile

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestReadRemovesOnlyOneLineEnding(t *testing.T) {
	path := filepath.Join(t.TempDir(), "secret")
	require.NoError(t, os.WriteFile(path, []byte(" value with spaces \r\n"), 0o600))
	value, err := Read(path)
	require.NoError(t, err)
	require.Equal(t, " value with spaces ", value)
}

func TestReadRejectsMissingEmptyAndNonRegularFiles(t *testing.T) {
	_, err := Read(filepath.Join(t.TempDir(), "missing"))
	require.ErrorIs(t, err, ErrInvalidSecretFile)
	path := filepath.Join(t.TempDir(), "empty")
	require.NoError(t, os.WriteFile(path, nil, 0o600))
	_, err = Read(path)
	require.ErrorIs(t, err, ErrInvalidSecretFile)
	_, err = Read(t.TempDir())
	require.ErrorIs(t, err, ErrInvalidSecretFile)
}
